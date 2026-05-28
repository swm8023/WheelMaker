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
  });

  test('starts locked voice input on long press when the composer has sendable content', async () => {
    const onSend = jest.fn();
    const onStart = jest.fn();
    const onFinish = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onSend={onSend}
          onStart={onStart}
          onFinish={onFinish}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(async () => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      jest.advanceTimersByTime(260);
      await Promise.resolve();
    });

    expect(onStart).toHaveBeenCalledWith('locked');
    expect(onSend).not.toHaveBeenCalled();

    await ReactTestRenderer.act(() => {
      button.props.onPointerUp(pointerEvent({timeStamp: 420}));
    });

    expect(onFinish).not.toHaveBeenCalled();
  });

  test('starts locked recording on short press when the composer is empty', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={false}
          onStart={onStart}
          onFinish={onFinish}
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
  });

  test('keeps empty composer long press locked until the next recording click', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={false}
          onStart={onStart}
          onFinish={onFinish}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(async () => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      jest.advanceTimersByTime(260);
      await Promise.resolve();
    });

    expect(onStart).toHaveBeenCalledWith('locked');

    await ReactTestRenderer.act(() => {
      button.props.onPointerUp(pointerEvent({timeStamp: 520}));
    });

    expect(onFinish).not.toHaveBeenCalled();
  });

  test('finishes locked recording on the next pointer down', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={true}
          recordingMode="locked"
          onStart={onStart}
          onFinish={onFinish}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({timeStamp: 300}));
    });

    expect(onFinish).toHaveBeenCalledTimes(1);
    expect(onStart).not.toHaveBeenCalled();
  });

  test('keeps async voice start alive when permission UI interrupts the gesture', async () => {
    const start = deferred();
    const onStart = jest.fn(() => start.promise);
    const onFinish = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onStart={onStart}
          onFinish={onFinish}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({clientY: 200, timeStamp: 0}));
      jest.advanceTimersByTime(260);
      button.props.onPointerUp(pointerEvent({clientY: 200, timeStamp: 320}));
    });

    expect(onStart).toHaveBeenCalledWith('locked');
    expect(onFinish).not.toHaveBeenCalled();

    await ReactTestRenderer.act(async () => {
      start.resolve();
      await start.promise;
    });

    expect(onFinish).not.toHaveBeenCalled();
  });

  test('ignores upward movement during a recording start because cancellation is click-only', async () => {
    const start = deferred();
    const onStart = jest.fn(() => start.promise);
    const onFinish = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onStart={onStart}
          onFinish={onFinish}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({clientY: 200, timeStamp: 0}));
      jest.advanceTimersByTime(260);
      button.props.onPointerMove(pointerEvent({clientY: 120, timeStamp: 280}));
      button.props.onPointerUp(pointerEvent({clientY: 120, timeStamp: 320}));
    });

    expect(onFinish).not.toHaveBeenCalled();

    await ReactTestRenderer.act(async () => {
      start.resolve();
      await start.promise;
    });

    expect(onFinish).not.toHaveBeenCalled();
  });

  test('leaves long-press voice recording active on browser pointer cancel', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          hasSendableContent={true}
          onStart={onStart}
          onFinish={onFinish}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(async () => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      jest.advanceTimersByTime(260);
      await Promise.resolve();
      button.props.onPointerCancel(pointerEvent({timeStamp: 120}));
    });

    expect(onStart).toHaveBeenCalledWith('locked');
    expect(onFinish).not.toHaveBeenCalled();
  });
});
