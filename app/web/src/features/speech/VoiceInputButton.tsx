import React, {useEffect, useRef, useState} from 'react';
import {resolveVoiceGestureState} from './useVoiceInputController';
import {formatVoiceInputDiagnosticError, type VoiceInputDiagnosticEntry} from './voiceInputDiagnostics';

const VOICE_LONG_PRESS_MS = 260;

export type VoiceInputButtonProps = {
  disabled?: boolean;
  readOnly?: boolean;
  recording: boolean;
  onStart: () => void | Promise<void>;
  onFinish: () => void | Promise<void>;
  onCancel: () => void | Promise<void>;
  onCancelIntentChange?: (cancelIntent: boolean) => void;
  onLog?: (entry: VoiceInputDiagnosticEntry) => void;
};

type PointerAction = 'finish' | 'cancel' | 'lock';

type ActivePointer = {
  pointerId: number;
  startY: number;
  startTime: number;
  startTimer: number | null;
  startRequested: boolean;
  cancelIntent: boolean;
  startSettled: boolean;
  pendingAction: PointerAction | null;
};

export function VoiceInputButton({
  disabled = false,
  readOnly = false,
  recording,
  onStart,
  onFinish,
  onCancel,
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
    if (action === 'lock') {
      return;
    }
    if (action === 'cancel') {
      void Promise.resolve(onCancel());
      return;
    }
    void Promise.resolve(onFinish());
  };

  const beginPointerStart = (pointer: ActivePointer) => {
    if (pointerRef.current !== pointer || pointer.startRequested) {
      return;
    }
    pointer.startTimer = null;
    pointer.startRequested = true;
    log('debug', 'long_press_start', {pointerId: pointer.pointerId});
    void Promise.resolve(onStart())
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

  const resolvePointerAction = (
    pointer: ActivePointer,
    event: React.PointerEvent<HTMLButtonElement>,
    source: 'up' | 'cancel',
  ): PointerAction => {
    if (pointer.cancelIntent) {
      return 'cancel';
    }
    if (source === 'cancel') {
      return 'lock';
    }
    const elapsedMs = Math.max(0, event.timeStamp - pointer.startTime);
    return elapsedMs < VOICE_LONG_PRESS_MS ? 'lock' : 'finish';
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
    if (!pointer.startRequested) {
      clearStartTimer(pointer);
      pointerRef.current = null;
      setNextCancelIntent(false);
      log('debug', source === 'cancel' ? 'pointer_cancel_before_start' : 'short_press_ignored', {
        pointerId: pointer.pointerId,
        elapsedMs,
        cancelIntent: pointer.cancelIntent,
      });
      return;
    }
    const action = resolvePointerAction(pointer, event, source);
    setNextCancelIntent(false);
    if (!pointer.startSettled) {
      pointer.pendingAction = action;
      return;
    }
    pointerRef.current = null;
    runTerminalAction(action);
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
      className={`voice-input-button${recording ? ' recording' : ''}${cancelIntent ? ' cancel-intent' : ''}`}
      disabled={disabled}
      aria-label={recording ? 'Finish voice input' : 'Start voice input'}
      title={recording ? 'Finish voice input' : 'Voice input'}
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
          log('debug', 'recording_pointer_down_finish', {pointerId: event.pointerId});
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
          startRequested: false,
          cancelIntent: false,
          startSettled: false,
          pendingAction: null,
        };
        pointer.startTimer = window.setTimeout(() => beginPointerStart(pointer), VOICE_LONG_PRESS_MS);
        pointerRef.current = pointer;
        setNextCancelIntent(false);
      }}
      onPointerMove={event => {
        const pointer = pointerRef.current;
        if (!pointer || pointer.pointerId !== event.pointerId) {
          return;
        }
        const next = resolveVoiceGestureState(pointer.startY, event.clientY) === 'cancel';
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
        const next = resolveVoiceGestureState(pointer.startY, event.clientY) === 'cancel';
        if (next !== pointer.cancelIntent) {
          setNextCancelIntent(next);
        }
      }}
    >
      <span className="codicon codicon-mic" aria-hidden="true" />
    </button>
  );
}
