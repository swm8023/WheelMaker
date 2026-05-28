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
  test('locks recording when a short press ends before async start settles', async () => {
    const start = deferred();
    const onStart = jest.fn(() => start.promise);
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
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

    expect(onStart).toHaveBeenCalledTimes(1);
    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();

    await ReactTestRenderer.act(async () => {
      start.resolve();
      await start.promise;
    });

    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();

    await ReactTestRenderer.act(() => {
      renderer!.update(
        <VoiceInputButton
          recording={true}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
        />,
      );
    });
    const recordingButton = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      recordingButton.props.onPointerDown(pointerEvent({timeStamp: 300}));
    });

    expect(onFinish).toHaveBeenCalledTimes(1);
    expect(onStart).toHaveBeenCalledTimes(1);
  });

  test('finishes recording after long press release', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      button.props.onPointerUp(pointerEvent({timeStamp: 520}));
    });

    expect(onStart).toHaveBeenCalledTimes(1);
    expect(onFinish).toHaveBeenCalledTimes(1);
    expect(onCancel).not.toHaveBeenCalled();
  });

  test('defers cancel intent until async start settles', async () => {
    const start = deferred();
    const onStart = jest.fn(() => start.promise);
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({clientY: 200, timeStamp: 0}));
      button.props.onPointerMove(pointerEvent({clientY: 120, timeStamp: 90}));
      button.props.onPointerUp(pointerEvent({clientY: 120, timeStamp: 120}));
    });

    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();

    await ReactTestRenderer.act(async () => {
      start.resolve();
      await start.promise;
    });

    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  test('locks recording instead of cancelling on browser pointer cancel without cancel intent', async () => {
    const onStart = jest.fn();
    const onFinish = jest.fn();
    const onCancel = jest.fn();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceInputButton
          recording={false}
          onStart={onStart}
          onFinish={onFinish}
          onCancel={onCancel}
        />,
      );
    });

    const button = renderer!.root.findByType('button');
    await ReactTestRenderer.act(() => {
      button.props.onPointerDown(pointerEvent({timeStamp: 0}));
      button.props.onPointerCancel(pointerEvent({timeStamp: 120}));
    });

    expect(onStart).toHaveBeenCalledTimes(1);
    expect(onFinish).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
  });
});
