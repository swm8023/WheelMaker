import {
  resolveDesktopChatQuickSwitchContextMenu,
} from '../web/src/shell/desktop/chatQuickSwitchContextMenu';

function target(interactive = false): EventTarget {
  return {
    closest: jest.fn(() => interactive ? { nodeName: 'A' } : null),
  } as unknown as EventTarget;
}

describe('desktop chat quick switch context menu', () => {
  test('opens only for the desktop shell when no text is selected and the target is not interactive', () => {
    const result = resolveDesktopChatQuickSwitchContextMenu({
      desktopShell: true,
      target: target(),
      selectedText: '',
      clientX: 280,
      clientY: 180,
      viewportWidth: 900,
      viewportHeight: 600,
    });

    expect(result).toEqual({
      open: true,
      style: {
        position: 'fixed',
        left: 280,
        top: 180,
        width: 320,
        maxHeight: 280,
      },
    });
  });

  test('keeps native context menu outside the desktop shell', () => {
    const result = resolveDesktopChatQuickSwitchContextMenu({
      desktopShell: false,
      target: target(),
      selectedText: '',
      clientX: 120,
      clientY: 80,
      viewportWidth: 900,
      viewportHeight: 600,
    });

    expect(result).toEqual({ open: false });
  });

  test('uses the desktop bridge when an explicit shell override is not supplied', () => {
    const globalWithWindow = globalThis as unknown as { window?: unknown };
    const previousWindow = globalWithWindow.window;
    globalWithWindow.window = { WheelMakerDesktop: { enabled: true } };
    try {
      const result = resolveDesktopChatQuickSwitchContextMenu({
        desktopLayout: true,
        target: target(),
        selectedText: '',
        clientX: 120,
        clientY: 80,
        viewportWidth: 900,
        viewportHeight: 600,
      });

      expect(result.open).toBe(true);
    } finally {
      if (previousWindow === undefined) {
        delete globalWithWindow.window;
      } else {
        globalWithWindow.window = previousWindow;
      }
    }
  });

  test('keeps native context menu when text is selected', () => {
    const result = resolveDesktopChatQuickSwitchContextMenu({
      desktopShell: true,
      target: target(),
      selectedText: 'selected text',
      clientX: 120,
      clientY: 80,
      viewportWidth: 900,
      viewportHeight: 600,
    });

    expect(result).toEqual({ open: false });
  });

  test('keeps native context menu for interactive chat targets', () => {
    const result = resolveDesktopChatQuickSwitchContextMenu({
      desktopShell: true,
      target: target(true),
      selectedText: '',
      clientX: 120,
      clientY: 80,
      viewportWidth: 900,
      viewportHeight: 600,
    });

    expect(result).toEqual({ open: false });
  });

  test('clamps the menu inside the viewport when opened near the bottom right edge', () => {
    const result = resolveDesktopChatQuickSwitchContextMenu({
      desktopShell: true,
      target: target(),
      selectedText: '',
      clientX: 780,
      clientY: 560,
      viewportWidth: 900,
      viewportHeight: 600,
    });

    expect(result).toEqual({
      open: true,
      style: {
        position: 'fixed',
        left: 572,
        top: 312,
        width: 320,
        maxHeight: 280,
      },
    });
  });
});
