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

  test('renders source selector in the title text position when a remote URL is available', async () => {
    const startDrag = jest.fn();
    const minimize = jest.fn();
    const toggleMaximize = jest.fn();
    const close = jest.fn();
    const getWebSourceState = jest.fn(async () => ({
      preference: 'auto',
      actualSource: 'remote',
      displayTitle: 'WheelMaker - example.com',
      displaySource: 'example.com',
      remoteUrl: 'https://example.com/',
      remoteHost: 'example.com',
    }));
    const setWebSourcePreference = jest.fn(async () => ({
      preference: 'embedded',
      actualSource: 'embedded',
      displayTitle: 'WheelMaker - Embedded',
      displaySource: 'Embedded',
      remoteUrl: '',
      remoteHost: '',
    }));
    const reload = jest.fn();
    (global as typeof globalThis & { window?: unknown }).window = {
      location: {reload},
      WheelMakerDesktop: {
        enabled: true,
        startDrag,
        minimize,
        toggleMaximize,
        close,
        getWebSourceState,
        setWebSourcePreference,
      },
    };

    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
    await ReactTestRenderer.act(async () => {
      renderer = ReactTestRenderer.create(<DesktopTitleBar title="WheelMaker" />);
    });

    const root = renderer!.root;
    expect(root.findByProps({'data-desktop-titlebar': true})).toBeDefined();
    expect(root.findByType('img').props.src).toBe('/icons/icon.svg');
    expect(root.findAllByProps({className: 'desktop-titlebar-title'})).toHaveLength(0);
    expect(root.findByProps({className: 'desktop-titlebar-app-title'}).props.children).toBe('WheelMaker - ');
    const select = root.findByProps({className: 'desktop-titlebar-source-select'});
    expect(select.props['data-desktop-titlebar-interactive']).toBe(true);
    expect(select.props.title).toBe('https://example.com/');
    expect(select.findAllByType('option').find(option => option.props.value === 'current')?.props.children).toBe('example.com');
    expect(select.findAllByType('option').filter(option => !option.props.hidden).map(option => option.props.children)).toEqual([
      'example.com',
      'Embedded',
    ]);
    expect(select.props.value).toBe('current');
    await ReactTestRenderer.act(async () => {
      await select.props.onChange({target: {value: 'embedded'}});
    });
    expect(setWebSourcePreference).toHaveBeenCalledWith('embedded');
    expect(reload).toHaveBeenCalled();

    const dragRegion = root.findByProps({'data-desktop-titlebar-drag-region': true});
    expect(root.findByProps({className: 'desktop-titlebar-controls'}).findAllByType('select')).toHaveLength(0);
    expect(dragRegion.props.onPointerDown).toBeUndefined();
    const preventDefault = jest.fn();
    dragRegion.props.onMouseDown({
      button: 0,
      detail: 1,
      target: { closest: () => null },
      preventDefault,
    });
    expect(preventDefault).toHaveBeenCalled();
    expect(startDrag).toHaveBeenCalled();
    const selectMouseDownPreventDefault = jest.fn();
    dragRegion.props.onMouseDown({
      button: 0,
      detail: 1,
      target: { closest: (selector: string) => selector.includes('[data-desktop-titlebar-interactive]') ? {} : null },
      preventDefault: selectMouseDownPreventDefault,
    });
    expect(selectMouseDownPreventDefault).not.toHaveBeenCalled();
    expect(startDrag).toHaveBeenCalledTimes(1);
    dragRegion.props.onDoubleClick({
      target: { closest: (selector: string) => selector.includes('[data-desktop-titlebar-interactive]') ? {} : null },
    });
    expect(toggleMaximize).not.toHaveBeenCalled();
    dragRegion.props.onDoubleClick({ target: { closest: () => null } });
    expect(toggleMaximize).toHaveBeenCalledTimes(1);
    const doubleClickPreventDefault = jest.fn();
    dragRegion.props.onMouseDown({
      button: 0,
      detail: 2,
      target: { closest: () => null },
      preventDefault: doubleClickPreventDefault,
    });
    expect(doubleClickPreventDefault).toHaveBeenCalled();
    expect(startDrag).toHaveBeenCalledTimes(1);
    expect(toggleMaximize).toHaveBeenCalledTimes(2);
    dragRegion.props.onDoubleClick({ target: { closest: () => null } });
    expect(toggleMaximize).toHaveBeenCalledTimes(2);

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
    expect(toggleMaximize).toHaveBeenCalledTimes(3);
    expect(close).toHaveBeenCalled();
  });

  test('renders plain embedded title text when no remote URL is available', async () => {
    const getWebSourceState = jest.fn(async () => ({
      preference: 'auto',
      actualSource: 'embedded',
      displayTitle: 'WheelMaker - Embedded',
      displaySource: 'Embedded',
      remoteUrl: '',
      remoteHost: '',
    }));
    (global as typeof globalThis & { window?: unknown }).window = {
      WheelMakerDesktop: {
        enabled: true,
        getWebSourceState,
      },
    };

    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
    await ReactTestRenderer.act(async () => {
      renderer = ReactTestRenderer.create(<DesktopTitleBar title="WheelMaker" />);
    });

    const root = renderer!.root;
    expect(root.findByProps({className: 'desktop-titlebar-app-title'}).props.children).toBe('WheelMaker - ');
    expect(root.findByProps({className: 'desktop-titlebar-source-label'}).props.children).toBe('Embedded');
    expect(root.findAllByProps({className: 'desktop-titlebar-source-select'})).toHaveLength(0);
  });
});
