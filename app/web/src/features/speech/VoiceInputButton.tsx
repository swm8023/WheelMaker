import React, {useEffect, useRef} from 'react';
import {formatVoiceInputDiagnosticError, type VoiceInputDiagnosticEntry} from './voiceInputDiagnostics';

const VOICE_LONG_PRESS_MS = 260;

export type VoiceInputInteractionMode = 'locked' | 'hold';

export type VoiceInputButtonProps = {
  disabled?: boolean;
  readOnly?: boolean;
  hasSendableContent?: boolean;
  recording: boolean;
  recordingMode?: VoiceInputInteractionMode | null;
  onSend?: () => void | Promise<void>;
  onStart: (mode: VoiceInputInteractionMode) => void | Promise<void>;
  onFinish: () => void | Promise<void>;
  onLog?: (entry: VoiceInputDiagnosticEntry) => void;
};

type ActivePointer = {
  pointerId: number;
  startTime: number;
  startTimer: number | null;
  voiceStarted: boolean;
  voiceMode: VoiceInputInteractionMode | null;
  startSettled: boolean;
};

export function VoiceInputButton({
  disabled = false,
  readOnly = false,
  hasSendableContent = false,
  recording,
  recordingMode = null,
  onSend,
  onStart,
  onFinish,
  onLog,
}: VoiceInputButtonProps) {
  const pointerRef = useRef<ActivePointer | null>(null);

  const log = (level: VoiceInputDiagnosticEntry['level'], event: string, details?: Record<string, unknown>) => {
    onLog?.({level, event, details});
  };

  const clearStartTimer = (pointer: ActivePointer) => {
    if (pointer.startTimer === null) {
      return;
    }
    window.clearTimeout(pointer.startTimer);
    pointer.startTimer = null;
  };

  const beginVoiceStart = (pointer: ActivePointer, mode: VoiceInputInteractionMode) => {
    if (pointerRef.current !== pointer || pointer.voiceStarted) {
      return;
    }
    pointer.startTimer = null;
    pointer.voiceStarted = true;
    pointer.voiceMode = mode;
    log('debug', mode === 'hold' ? 'long_press_start' : 'locked_press_start', {pointerId: pointer.pointerId});
    void Promise.resolve(onStart(mode))
      .then(() => {
        if (pointerRef.current !== pointer) {
          log('debug', 'start_settled_after_pointer_cleared', {pointerId: pointer.pointerId});
          return;
        }
        pointer.startSettled = true;
      })
      .catch(error => {
        log('error', 'start_failed', {
          pointerId: pointer.pointerId,
          error: formatVoiceInputDiagnosticError(error),
        });
        if (pointerRef.current === pointer) {
          pointerRef.current = null;
        }
      });
  };

  const startPointerVoice = (pointer: ActivePointer) => {
    if (pointerRef.current !== pointer) {
      return;
    }
    pointer.startTimer = null;
    if (!pointer.voiceStarted) {
      beginVoiceStart(pointer, 'locked');
    }
  };

  const finishPointer = (
    event: React.PointerEvent<HTMLButtonElement>,
    source: 'up' | 'cancel',
  ) => {
    const pointer = pointerRef.current;
    if (!pointer || pointer.pointerId !== event.pointerId) {
      return;
    }
    event.currentTarget.releasePointerCapture?.(event.pointerId);
    const elapsedMs = Math.max(0, event.timeStamp - pointer.startTime);
    clearStartTimer(pointer);
    pointerRef.current = null;
    if (!pointer.voiceStarted) {
      if (source === 'up' && hasSendableContent) {
        void Promise.resolve(onSend?.());
      }
      log('debug', source === 'cancel' ? 'pointer_cancel_before_start' : 'short_press_send', {
        pointerId: pointer.pointerId,
        elapsedMs,
      });
      return;
    }
    log('debug', source === 'cancel' ? 'pointer_cancel_recording_kept' : 'pointer_up_recording_kept', {
      pointerId: pointer.pointerId,
      elapsedMs,
      mode: pointer.voiceMode,
      startSettled: pointer.startSettled,
    });
  };

  useEffect(() => () => {
    const pointer = pointerRef.current;
    if (pointer) {
      clearStartTimer(pointer);
    }
  }, []);

  return (
    <button
      type="button"
      className={[
        'voice-input-button',
        hasSendableContent && !recording ? 'send-with-voice' : '',
        recording ? 'recording' : '',
        recording && recordingMode === 'locked' ? 'locked-recording' : '',
        recording && recordingMode === 'hold' ? 'hold-recording' : '',
      ].filter(Boolean).join(' ')}
      disabled={disabled}
      aria-label={
        recording
          ? 'Finish voice input'
          : hasSendableContent
            ? 'Send message, hold for voice input'
            : 'Start voice input'
      }
      title={recording ? 'Finish voice input' : hasSendableContent ? 'Send' : 'Voice input'}
      onContextMenu={event => event.preventDefault()}
      onPointerDown={event => {
        if (disabled || pointerRef.current) {
          log('warn', 'pointer_down_ignored', {
            reason: disabled ? 'disabled' : 'active_pointer',
            pointerId: event.pointerId,
          });
          return;
        }
        event.preventDefault();
        if (recording) {
          log('debug', `${recordingMode ?? 'recording'}_pointer_down_finish`, {pointerId: event.pointerId});
          void Promise.resolve(onFinish());
          return;
        }
        if (readOnly) {
          log('warn', 'pointer_down_ignored', {
            reason: 'read_only',
            pointerId: event.pointerId,
          });
          return;
        }
        event.currentTarget.setPointerCapture?.(event.pointerId);
        const pointer: ActivePointer = {
          pointerId: event.pointerId,
          startTime: event.timeStamp,
          startTimer: null,
          voiceStarted: false,
          voiceMode: null,
          startSettled: false,
        };
        if (hasSendableContent) {
          pointer.startTimer = window.setTimeout(() => startPointerVoice(pointer), VOICE_LONG_PRESS_MS);
        }
        pointerRef.current = pointer;
        if (!hasSendableContent) {
          beginVoiceStart(pointer, 'locked');
        }
      }}
      onPointerMove={event => {
        const pointer = pointerRef.current;
        if (!pointer || pointer.pointerId !== event.pointerId) {
          return;
        }
        event.preventDefault();
      }}
      onPointerUp={event => finishPointer(event, 'up')}
      onPointerCancel={event => finishPointer(event, 'cancel')}
      onPointerLeave={event => {
        const pointer = pointerRef.current;
        if (!pointer || pointer.pointerId !== event.pointerId) {
          return;
        }
        event.preventDefault();
      }}
    >
      <span className={`codicon ${hasSendableContent && !recording ? 'codicon-send' : 'codicon-mic'}`} aria-hidden="true" />
      {hasSendableContent && !recording ? (
        <span className="voice-input-badge" aria-hidden="true">
          <span className="codicon codicon-mic" />
        </span>
      ) : null}
    </button>
  );
}
