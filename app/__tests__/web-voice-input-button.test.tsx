import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import {VoiceInputButton} from '../web/src/features/speech/VoiceInputButton';

function deferred<T = void>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>(next => {
    resolve = next;
  });
  return {promise, resolve};
}

function pointerEvent(input: {
  pointerId?: number;
  clientY?: number;
  timeStamp?: number;
} = {}) {
  return {
    pointerId: input.pointerId ?? 1,
    clientY: input.clientY ?? 200,
    timeStamp: input.timeStamp ?? 0,
    preventDefault: jest.fn(),
    currentTarget: {
      setPointerCapture: jest.fn(),
      releasePointerCapture: jest.fn(),
    },
  };
}

describe('VoiceInputButton', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  test('sends on short press when the composer has sendable content', async () => {
    const onSend = jest.fn();
    const onStart = jest.fn();
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    const onLog = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onSend={onSend}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
          onLog={onLog}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      button.props.onPointerUp(pointerEvent({timeStamp: 80}));
    });

    expect(onSend).toHaveBeenCalledTimes(1);
    expect(onStart).not.toHaveBeenCalled();
    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
  });

  test('starts hold voice input on long press when the composer has sendable content', async () => {
    const onSend = jest.fn();
    const onStart = jest.fn();
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onSend={onSend}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(async () => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      jest.advanceTimersByTime(260);
      await Promise.resolve();
    });

    expect(onStart).toHaveBeenCalledWith('hold');
    expect(onSend).not.toHaveBeenCalled();

    await ReactTestRenderer.act(() => {
      button.props.onPointerUp(pointerEvent({timeStamp: 420}));
    });

    expect(onFinish).toHaveBeenCalledTimes(1);
    expect(onCancel).not.toHaveBeenCalled();
  });

  test('cancels hold voice input on upward release', async () => {
    const onSend = jest.fn();
    const onStart = jest.fn();
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    const onCancelIntentChange = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onSend={onSend}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
          onCancelIntentChange={onCancelIntentChange}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(async () => {
      button.props.onPointerDown(pointerEvent({clientY: 200, timeStamp: 0}));
      jest.advanceTimersByTime(260);
      await Promise.resolve();
      button.props.onPointerMove(pointerEvent({clientY: 120, timeStamp: 300}));
      button.props.onPointerUp(pointerEvent({clientY: 120, timeStamp: 360}));
    });

    expect(onStart).toHaveBeenCalledWith('hold');
    expect(onCancelIntentChange).toHaveBeenCalledWith(true);
    expect(onCancelIntentChange).toHaveBeenLastCalledWith(false);
    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onSend).not.toHaveBeenCalled();
  });

  test('starts locked recording on short press when the composer is empty', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={false}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      button.props.onPointerUp(pointerEvent({timeStamp: 80}));
    });

    expect(onStart).toHaveBeenCalledWith('locked');
    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
  });

  test('promotes empty composer long press to hold and finishes on release', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    const onModeChange = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={false}
          onModeChange={onModeChange}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(async () => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      jest.advanceTimersByTime(260);
      await Promise.resolve();
      button.props.onPointerUp(pointerEvent({timeStamp: 520}));
    });

    expect(onStart).toHaveBeenCalledWith('locked');
    expect(onModeChange).toHaveBeenCalledWith('hold');
    expect(onFinish).toHaveBeenCalledTimes(1);
    expect(onCancel).not.toHaveBeenCalled();
  });

  test('finishes locked recording on the next pointer down', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={true}
          recordingMode="locked"
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({timeStamp: 300}));
    });

    expect(onFinish).toHaveBeenCalledTimes(1);
    expect(onStart).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
  });

  test('downgrades async hold start to locked recording when permission UI interrupts the gesture', async () => {
    const start = deferred();
    const onStart = jest.fn(() => start.promise);
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    const onModeChange = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onModeChange={onModeChange}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({clientY: 200, timeStamp: 0}));
      jest.advanceTimersByTime(260);
      button.props.onPointerUp(pointerEvent({clientY: 200, timeStamp: 320}));
    });

    expect(onStart).toHaveBeenCalledWith('hold');
    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();

    await ReactTestRenderer.act(async () => {
      start.resolve();
      await start.promise;
    });

    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
    expect(onModeChange).toHaveBeenCalledWith('locked');
  });

  test('keeps upward cancel when hold start settles after the gesture ends', async () => {
    const start = deferred();
    const onStart = jest.fn(() => start.promise);
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    const onModeChange = jest.fn();
    const onCancelIntentChange = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onModeChange={onModeChange}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
          onCancelIntentChange={onCancelIntentChange}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({clientY: 200, timeStamp: 0}));
      jest.advanceTimersByTime(260);
      button.props.onPointerMove(pointerEvent({clientY: 120, timeStamp: 300}));
      button.props.onPointerUp(pointerEvent({clientY: 120, timeStamp: 320}));
    });

    expect(onStart).toHaveBeenCalledWith('hold');
    expect(onCancelIntentChange).toHaveBeenCalledWith(true);
    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();

    await ReactTestRenderer.act(async () => {
      start.resolve();
      await start.promise;
    });

    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onModeChange).not.toHaveBeenCalledWith('locked');
    expect(onCancelIntentChange).toHaveBeenLastCalledWith(false);
  });

  test('finishes settled hold recording on browser pointer cancel without cancel intent', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    const onModeChange = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onModeChange={onModeChange}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(async () => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      jest.advanceTimersByTime(260);
      await Promise.resolve();
      button.props.onPointerCancel(pointerEvent({timeStamp: 320}));
    });

    expect(onStart).toHaveBeenCalledWith('hold');
    expect(onFinish).toHaveBeenCalledTimes(1);
    expect(onCancel).not.toHaveBeenCalled();
    expect(onModeChange).not.toHaveBeenCalled();
  });

  test('does not start hold recording when the browser cancels before the timer callback runs', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    const onSend = jest.fn();
    const onModeChange = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onSend={onSend}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
          onModeChange={onModeChange}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      button.props.onPointerCancel(pointerEvent({timeStamp: 320}));
    });

    expect(onStart).not.toHaveBeenCalled();
    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
    expect(onSend).not.toHaveBeenCalled();
    expect(onModeChange).not.toHaveBeenCalled();
  });
});
