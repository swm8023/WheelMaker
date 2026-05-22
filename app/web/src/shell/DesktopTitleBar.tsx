import React, { useEffect, useRef, useState } from 'react';
import { getDesktopWindowBridge, type DesktopWebSourceState } from './desktopRuntime';
import {
  readDesktopWebSourceState,
  setDesktopWebSourcePreference,
} from './desktop/webSource';

type DesktopTitleBarProps = {
  title: string;
};

function invokeDesktopAction(action: (() => Promise<void> | void) | undefined) {
  void action?.();
}

function isDesktopTitleBarInteractiveTarget(target: EventTarget | null) {
  const targetElement = target as { closest?: (selector: string) => Element | null } | null;
  return typeof targetElement?.closest === 'function'
    && Boolean(targetElement.closest('button, select, [data-desktop-titlebar-interactive]'));
}

export function DesktopTitleBar({ title }: DesktopTitleBarProps) {
  const bridge = getDesktopWindowBridge();
  const suppressNextDoubleClickRef = useRef(false);
  const [webSourceState, setWebSourceState] = useState<DesktopWebSourceState | null>(null);
  useEffect(() => {
    let cancelled = false;
    readDesktopWebSourceState().then(state => {
      if (!cancelled) {
        setWebSourceState(state);
      }
    });
    return () => {
      cancelled = true;
    };
  }, []);
  if (!bridge) {
    return null;
  }

  const handleDragMouseDown = (event: React.MouseEvent<HTMLDivElement>) => {
    if (event.button !== 0) {
      return;
    }
    if (isDesktopTitleBarInteractiveTarget(event.target)) {
      return;
    }
    event.preventDefault();
    if (event.detail >= 2) {
      suppressNextDoubleClickRef.current = true;
      invokeDesktopAction(bridge.toggleMaximize);
      return;
    }
    invokeDesktopAction(bridge.startDrag);
  };

  const handleDragDoubleClick = (event: React.MouseEvent<HTMLDivElement>) => {
    if (isDesktopTitleBarInteractiveTarget(event.target)) {
      suppressNextDoubleClickRef.current = false;
      return;
    }
    if (suppressNextDoubleClickRef.current) {
      suppressNextDoubleClickRef.current = false;
      return;
    }
    invokeDesktopAction(bridge.toggleMaximize);
  };
  const handleWebSourcePreferenceChange = async (event: React.ChangeEvent<HTMLSelectElement>) => {
    const preference = event.target.value === 'embedded' ? 'embedded' : 'auto';
    const nextState = await setDesktopWebSourcePreference(preference);
    if (nextState) {
      setWebSourceState(nextState);
    }
    window.location.reload();
  };
  const displayTitle = webSourceState?.displayTitle || title;
  const hasRemoteWebSource = Boolean(webSourceState?.remoteUrl && webSourceState.remoteHost && bridge.setWebSourcePreference);
  const remoteSourceLabel = webSourceState?.remoteHost || webSourceState?.displaySource || 'Auto';

  return (
    <div className="desktop-titlebar" data-desktop-titlebar={true}>
      <div
        className="desktop-titlebar-drag-region"
        data-desktop-titlebar-drag-region={true}
        onDoubleClick={handleDragDoubleClick}
        onMouseDown={handleDragMouseDown}
      >
        <img className="desktop-titlebar-icon" src="/icons/icon.svg" alt="" draggable={false} />
        {hasRemoteWebSource ? (
          <select
            className="desktop-titlebar-source-select"
            aria-label="Desktop web source"
            data-desktop-titlebar-interactive={true}
            title={webSourceState?.remoteUrl || displayTitle}
            value="current"
            onChange={handleWebSourcePreferenceChange}
          >
            <option value="current" hidden>{displayTitle}</option>
            <option value="auto">{remoteSourceLabel}</option>
            <option value="embedded">Embedded</option>
          </select>
        ) : (
          <span className="desktop-titlebar-title" title={displayTitle}>{displayTitle}</span>
        )}
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
