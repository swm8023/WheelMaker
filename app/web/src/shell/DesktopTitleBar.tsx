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
  const [sourceMenuOpen, setSourceMenuOpen] = useState(false);
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
  useEffect(() => {
    if (!sourceMenuOpen || typeof window === 'undefined' || typeof window.addEventListener !== 'function') {
      return undefined;
    }
    const closeSourceMenu = (event: PointerEvent) => {
      const target = event.target as { closest?: (selector: string) => Element | null } | null;
      if (typeof target?.closest === 'function' && target.closest('[data-desktop-titlebar-source-root]')) {
        return;
      }
      setSourceMenuOpen(false);
    };
    window.addEventListener('pointerdown', closeSourceMenu);
    return () => window.removeEventListener('pointerdown', closeSourceMenu);
  }, [sourceMenuOpen]);
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
  const handleWebSourcePreferenceSelect = async (preference: 'auto' | 'embedded') => {
    setSourceMenuOpen(false);
    const nextState = await setDesktopWebSourcePreference(preference);
    if (nextState) {
      setWebSourceState(nextState);
    }
    window.location.reload();
  };
  const displayTitle = webSourceState?.displayTitle || title;
  const titlePrefix = `${title} - `;
  const actualSourceLabel = webSourceState?.displaySource || '';
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
        {webSourceState ? (
          <span className="desktop-titlebar-title-group">
            <span className="desktop-titlebar-app-title" title={displayTitle}>{titlePrefix}</span>
            {hasRemoteWebSource ? (
              <span className="desktop-titlebar-source-control" data-desktop-titlebar-source-root={true}>
                <button
                  type="button"
                  className="desktop-titlebar-source-button"
                  aria-expanded={sourceMenuOpen}
                  aria-haspopup="menu"
                  aria-label="Desktop web source"
                  data-desktop-titlebar-interactive={true}
                  title={webSourceState.remoteUrl || actualSourceLabel}
                  onClick={() => setSourceMenuOpen(open => !open)}
                >
                  {actualSourceLabel}
                </button>
                {sourceMenuOpen ? (
                  <div
                    className="desktop-titlebar-source-menu"
                    role="menu"
                    data-desktop-titlebar-interactive={true}
                  >
                    <button
                      type="button"
                      className="desktop-titlebar-source-menu-item"
                      role="menuitemradio"
                      aria-checked={webSourceState.preference === 'auto'}
                      title={webSourceState.remoteUrl}
                      onClick={() => void handleWebSourcePreferenceSelect('auto')}
                    >
                      {remoteSourceLabel}
                    </button>
                    <button
                      type="button"
                      className="desktop-titlebar-source-menu-item"
                      role="menuitemradio"
                      aria-checked={webSourceState.preference === 'embedded'}
                      onClick={() => void handleWebSourcePreferenceSelect('embedded')}
                    >
                      Embedded
                    </button>
                  </div>
                ) : null}
              </span>
            ) : (
              <span className="desktop-titlebar-source-label" title={actualSourceLabel}>{actualSourceLabel}</span>
            )}
          </span>
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
