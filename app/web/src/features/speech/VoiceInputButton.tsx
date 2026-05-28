import React, {useEffect, useRef, useState} from 'react';
import {resolveVoiceGestureState} from './useVoiceInputController';
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
  onCancel: () => void | Promise<void>;
  onModeChange?: (mode: VoiceInputInteractionMode) => void;
  onCancelIntentChange?: (cancelIntent: boolean) => void;
  onLog?: (entry: VoiceInputDiagnosticEntry) => void;
};

type PointerAction = 'send' | 'finish' | 'cancel' | 'lock' | 'none';

type ActivePointer = {
  pointerId: number;
  startY: number;
  startTime: number;
  startTimer: number | null;
  voiceStarted: boolean;
  voiceMode: VoiceInputInteractionMode | null;
  startSettled: boolean;
  cancelIntent: boolean;
  pendingAction: PointerAction | null;
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
  onCancel,
  onModeChange,
  onCancelIntentChange,
  onLog,
}: VoiceInputButtonProps) {
  const pointerRef = useRef<ActivePointer | null>(null);
  const [cancelIntent, setCancelIntent] = useState(false);

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

  const setNextCancelIntent = (next: boolean) => {
    setCancelIntent(next);
    onCancelIntentChange?.(next);
    if (pointerRef.current) {
      pointerRef.current.cancelIntent = next;
    }
  };

  const runTerminalAction = (action: PointerAction) => {
    if (action === 'none') {
      return;
    }
    if (action === 'send') {
      void Promise.resolve(onSend?.());
      return;
    }
    if (action === 'lock') {
      onModeChange?.('locked');
      return;
    }
    if (action === 'cancel') {
      void Promise.resolve(onCancel());
      return;
    }
    void Promise.resolve(onFinish());
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
        if (!pointer.pendingAction) {
          return;
        }
        const action = pointer.pendingAction;
        pointerRef.current = null;
        setNextCancelIntent(false);
        runTerminalAction(action);
      })
      .catch(error => {
        log('error', 'start_failed', {
          pointerId: pointer.pointerId,
          error: formatVoiceInputDiagnosticError(error),
        });
        if (pointerRef.current === pointer) {
          pointerRef.current = null;
          setNextCancelIntent(false);
        }
      });
  };

  const promotePointerToHold = (pointer: ActivePointer) => {
    if (pointerRef.current !== pointer) {
      return;
    }
    pointer.startTimer = null;
    if (!pointer.voiceStarted) {
      beginVoiceStart(pointer, 'hold');
      return;
    }
    if (pointer.voiceMode !== 'hold') {
      pointer.voiceMode = 'hold';
      onModeChange?.('hold');
      log('debug', 'locked_press_promoted_to_hold', {pointerId: pointer.pointerId});
    }
  };

  const resolvePointerAction = (pointer: ActivePointer, source: 'up' | 'cancel'): PointerAction => {
    if (!pointer.voiceStarted) {
      return source === 'up' && hasSendableContent ? 'send' : 'none';
    }
    if (pointer.voiceMode === 'hold') {
      return pointer.cancelIntent ? 'cancel' : 'finish';
    }
    return 'lock';
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
    if (!pointer.voiceStarted) {
      if (source === 'up' && elapsedMs >= VOICE_LONG_PRESS_MS) {
        clearStartTimer(pointer);
        beginVoiceStart(pointer, 'hold');
        pointer.pendingAction = 'lock';
        return;
      }
      clearStartTimer(pointer);
      pointerRef.current = null;
      setNextCancelIntent(false);
      runTerminalAction(source === 'up' && hasSendableContent ? 'send' : 'none');
      log('debug', source === 'cancel' ? 'pointer_cancel_before_start' : 'short_press_send', {
        pointerId: pointer.pointerId,
        elapsedMs,
      });
      return;
    }
    clearStartTimer(pointer);
    const action = resolvePointerAction(pointer, source);
    setNextCancelIntent(false);
    if (!pointer.startSettled) {
      pointer.pendingAction = 'lock';
      return;
    }
    pointerRef.current = null;
    runTerminalAction(action);
    log('debug', source === 'cancel' ? 'pointer_cancel_recording_resolved' : 'pointer_up_recording_resolved', {
      pointerId: pointer.pointerId,
      elapsedMs,
      mode: pointer.voiceMode,
      startSettled: pointer.startSettled,
      action,
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
        cancelIntent ? 'cancel-intent' : '',
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
          startY: event.clientY,
          startTime: event.timeStamp,
          startTimer: null,
          voiceStarted: false,
          voiceMode: null,
          startSettled: false,
          cancelIntent: false,
          pendingAction: null,
        };
        pointer.startTimer = window.setTimeout(() => promotePointerToHold(pointer), VOICE_LONG_PRESS_MS);
        pointerRef.current = pointer;
        setNextCancelIntent(false);
        if (!hasSendableContent) {
          beginVoiceStart(pointer, 'locked');
        }
      }}
      onPointerMove={event => {
        const pointer = pointerRef.current;
        if (!pointer || pointer.pointerId !== event.pointerId) {
          return;
        }
        const next = pointer.voiceMode === 'hold' &&
          resolveVoiceGestureState(pointer.startY, event.clientY) === 'cancel';
        if (next !== pointer.cancelIntent) {
          setNextCancelIntent(next);
        }
      }}
      onPointerUp={event => finishPointer(event, 'up')}
      onPointerCancel={event => finishPointer(event, 'cancel')}
      onPointerLeave={event => {
        const pointer = pointerRef.current;
        if (!pointer || pointer.pointerId !== event.pointerId) {
          return;
        }
        const next = pointer.voiceMode === 'hold' &&
          resolveVoiceGestureState(pointer.startY, event.clientY) === 'cancel';
        if (next !== pointer.cancelIntent) {
          setNextCancelIntent(next);
        }
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
