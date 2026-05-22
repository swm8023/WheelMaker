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
    const target = event.target as { closest?: (selector: string) => Element | null } | null;
    if (typeof target?.closest === 'function' && target.closest('button')) {
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

  const handleDragDoubleClick = () => {
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

  return (
    <div className="desktop-titlebar" data-desktop-titlebar={true}>
      <div
        className="desktop-titlebar-drag-region"
        data-desktop-titlebar-drag-region={true}
        onDoubleClick={handleDragDoubleClick}
        onMouseDown={handleDragMouseDown}
      >
        <img className="desktop-titlebar-icon" src="/icons/icon.svg" alt="" draggable={false} />
        <span className="desktop-titlebar-title" title={webSourceState?.remoteUrl || displayTitle}>{displayTitle}</span>
      </div>
      <div className="desktop-titlebar-controls">
        {webSourceState && bridge.setWebSourcePreference ? (
          <select
            className="desktop-titlebar-source-select"
            aria-label="Desktop web source"
            title={webSourceState.remoteUrl || webSourceState.displaySource}
            value={webSourceState.preference}
            onChange={handleWebSourcePreferenceChange}
          >
            <option value="auto">Auto</option>
            <option value="embedded">Embedded</option>
          </select>
        ) : null}
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
