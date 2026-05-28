import React, {useRef, useState} from 'react';
import {resolveVoiceGestureState} from './useVoiceInputController';

export type VoiceInputButtonProps = {
  disabled?: boolean;
  readOnly?: boolean;
  recording: boolean;
  onStart: () => void | Promise<void>;
  onFinish: () => void | Promise<void>;
  onCancel: () => void | Promise<void>;
  onCancelIntentChange?: (cancelIntent: boolean) => void;
};

type ActivePointer = {
  pointerId: number;
  startY: number;
  cancelIntent: boolean;
  startSettled: boolean;
  pendingAction: 'finish' | 'cancel' | null;
};

export function VoiceInputButton({
  disabled = false,
  readOnly = false,
  recording,
  onStart,
  onFinish,
  onCancel,
  onCancelIntentChange,
}: VoiceInputButtonProps) {
  const pointerRef = useRef<ActivePointer | null>(null);
  const [cancelIntent, setCancelIntent] = useState(false);

  const setNextCancelIntent = (next: boolean) => {
    setCancelIntent(next);
    onCancelIntentChange?.(next);
    if (pointerRef.current) {
      pointerRef.current.cancelIntent = next;
    }
  };

  const runTerminalAction = (action: 'finish' | 'cancel') => {
    if (action === 'cancel') {
      void Promise.resolve(onCancel());
      return;
    }
    void Promise.resolve(onFinish());
  };

  const finishPointer = (event: React.PointerEvent<HTMLButtonElement>, forceCancel: boolean) => {
    const pointer = pointerRef.current;
    if (!pointer || pointer.pointerId !== event.pointerId) {
      return;
    }
    event.currentTarget.releasePointerCapture?.(event.pointerId);
    const action = forceCancel || pointer.cancelIntent ? 'cancel' : 'finish';
    setNextCancelIntent(false);
    if (!pointer.startSettled) {
      pointer.pendingAction = action;
      return;
    }
    pointerRef.current = null;
    runTerminalAction(action);
  };

  return (
    <button
      type="button"
      className={`voice-input-button${recording ? ' recording' : ''}${cancelIntent ? ' cancel-intent' : ''}`}
      disabled={disabled}
      aria-label={recording ? 'Finish voice input' : 'Start voice input'}
      title={recording ? 'Finish voice input' : 'Voice input'}
      onContextMenu={event => event.preventDefault()}
      onPointerDown={event => {
        if (disabled || readOnly || pointerRef.current) {
          return;
        }
        event.preventDefault();
        event.currentTarget.setPointerCapture?.(event.pointerId);
        pointerRef.current = {
          pointerId: event.pointerId,
          startY: event.clientY,
          cancelIntent: false,
          startSettled: false,
          pendingAction: null,
        };
        setNextCancelIntent(false);
        const pointer = pointerRef.current;
        void Promise.resolve(onStart())
          .then(() => {
            if (pointerRef.current !== pointer) {
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
          .catch(() => {
            if (pointerRef.current === pointer) {
              pointerRef.current = null;
              setNextCancelIntent(false);
            }
          });
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
      onPointerUp={event => finishPointer(event, false)}
      onPointerCancel={event => finishPointer(event, true)}
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
