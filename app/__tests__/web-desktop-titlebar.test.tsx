import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import { DesktopTitleBar } from '../web/src/shell/DesktopTitleBar';

describe('desktop title bar', () => {
  const originalWindow = (global as typeof globalThis & { window?: unknown }).window;

  afterEach(() => {
    (global as typeof globalThis & { window?: unknown }).window = originalWindow;
  });

  test('renders nothing outside the desktop WebView runtime', async () => {
    (global as typeof globalThis & { window?: unknown }).window = {};

    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(<DesktopTitleBar title="WheelMaker" />);
    });

    expect(renderer!.toJSON()).toBeNull();
  });

  test('renders themed native window controls inside the desktop runtime', async () => {
    const startDrag = jest.fn();
    const minimize = jest.fn();
    const toggleMaximize = jest.fn();
    const close = jest.fn();
    (global as typeof globalThis & { window?: unknown }).window = {
      WheelMakerDesktop: {
        enabled: true,
        startDrag,
        minimize,
        toggleMaximize,
        close,
      },
    };

    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(<DesktopTitleBar title="WheelMaker" />);
    });

    const root = renderer!.root;
    expect(root.findByProps({'data-desktop-titlebar': true})).toBeDefined();
    expect(root.findByProps({className: 'desktop-titlebar-title'}).props.children).toBe('WheelMaker');
    expect(root.findByType('img').props.src).toBe('/icons/icon.svg');

    const dragRegion = root.findByProps({'data-desktop-titlebar-drag-region': true});
    const preventDefault = jest.fn();
    dragRegion.props.onPointerDown({
      button: 0,
      target: { closest: () => null },
      preventDefault,
    });
    expect(preventDefault).toHaveBeenCalled();
    expect(startDrag).toHaveBeenCalled();
    dragRegion.props.onDoubleClick();
    expect(toggleMaximize).toHaveBeenCalledTimes(1);
    const doubleClickPreventDefault = jest.fn();
    dragRegion.props.onPointerDown({
      button: 0,
      detail: 2,
      target: { closest: () => null },
      preventDefault: doubleClickPreventDefault,
    });
    expect(doubleClickPreventDefault).toHaveBeenCalled();
    expect(startDrag).toHaveBeenCalledTimes(1);
    expect(toggleMaximize).toHaveBeenCalledTimes(1);

    const buttons = root.findAllByType('button');
    expect(buttons.map(button => button.props['aria-label'])).toEqual([
      'Minimize',
      'Maximize or restore',
      'Close',
    ]);

    buttons[0].props.onClick();
    buttons[1].props.onClick();
    buttons[2].props.onClick();

    expect(minimize).toHaveBeenCalled();
    expect(toggleMaximize).toHaveBeenCalledTimes(2);
    expect(close).toHaveBeenCalled();
  });
});
