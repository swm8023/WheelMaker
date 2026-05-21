import React from 'react';
import { getDesktopWindowBridge } from './desktopRuntime';

type DesktopTitleBarProps = {
  title: string;
};

function invokeDesktopAction(action: (() => Promise<void> | void) | undefined) {
  void action?.();
}

export function DesktopTitleBar({ title }: DesktopTitleBarProps) {
  const bridge = getDesktopWindowBridge();
  if (!bridge) {
    return null;
  }

  const handleDragPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) {
      return;
    }
    const target = event.target as { closest?: (selector: string) => Element | null } | null;
    if (typeof target?.closest === 'function' && target.closest('button')) {
      return;
    }
    event.preventDefault();
    if (event.detail >= 2) {
      return;
    }
    invokeDesktopAction(bridge.startDrag);
  };

  return (
    <div className="desktop-titlebar" data-desktop-titlebar={true}>
      <div
        className="desktop-titlebar-drag-region"
        data-desktop-titlebar-drag-region={true}
        onDoubleClick={() => invokeDesktopAction(bridge.toggleMaximize)}
        onPointerDown={handleDragPointerDown}
      >
        <img className="desktop-titlebar-icon" src="/icons/icon.svg" alt="" draggable={false} />
        <span className="desktop-titlebar-title">{title}</span>
      </div>
      <div className="desktop-titlebar-controls">
        <button
          type="button"
          className="desktop-titlebar-button"
          aria-label="Minimize"
          title="Minimize"
          onClick={() => invokeDesktopAction(bridge.minimize)}
        >
          <span className="codicon codicon-chrome-minimize" aria-hidden="true" />
        </button>
        <button
          type="button"
          className="desktop-titlebar-button"
          aria-label="Maximize or restore"
          title="Maximize or restore"
          onClick={() => invokeDesktopAction(bridge.toggleMaximize)}
        >
          <span className="codicon codicon-chrome-maximize" aria-hidden="true" />
        </button>
        <button
          type="button"
          className="desktop-titlebar-button desktop-titlebar-close"
          aria-label="Close"
          title="Close"
          onClick={() => invokeDesktopAction(bridge.close)}
        >
          <span className="codicon codicon-chrome-close" aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}
