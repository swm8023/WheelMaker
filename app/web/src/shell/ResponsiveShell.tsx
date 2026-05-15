import React, { type ReactNode } from 'react';
import type { LayoutMode } from '../services/responsiveLayout';

type ShellThemeMode = 'dark' | 'light';

type ShellContentProps = {
  themeMode: ShellThemeMode;
  setiFontCss: string;
  sidebar: ReactNode;
  main: ReactNode;
};

export type DesktopShellProps = ShellContentProps & {
  desktopActivityBar: ReactNode;
  sidebarCollapsed: boolean;
  desktopSidebarWidth: number;
};

export type MobileShellProps = ShellContentProps & {
  floatingControlStack: ReactNode;
  mobileSettingsScreen: ReactNode;
  drawerOpen: boolean;
  onCloseDrawer: () => void;
};

export type ResponsiveShellProps = DesktopShellProps &
  MobileShellProps & {
    mode: LayoutMode;
  };

export function DesktopShell({
  themeMode,
  setiFontCss,
  desktopActivityBar,
  sidebar,
  main,
  sidebarCollapsed,
  desktopSidebarWidth,
}: DesktopShellProps) {
  return (
    <div className={`workspace theme-${themeMode}`}>
      <style>{setiFontCss}</style>
      <div
        className="desktop-shell"
        style={{ '--desktop-sidebar-width': `${desktopSidebarWidth}px` } as React.CSSProperties}
      >
        {desktopActivityBar}
        <div className="body">
          {!sidebarCollapsed ? (
            <aside className="workspace-left">{sidebar}</aside>
          ) : null}
          <main className="workspace-right">{main}</main>
        </div>
      </div>
    </div>
  );
}

export function MobileShell({
  themeMode,
  setiFontCss,
  floatingControlStack,
  mobileSettingsScreen,
  sidebar,
  main,
  drawerOpen,
  onCloseDrawer,
}: MobileShellProps) {
  return (
    <div className={`workspace theme-${themeMode} narrow-shell`}>
      <style>{setiFontCss}</style>
      {floatingControlStack}
      {mobileSettingsScreen}

      <div className="body">
        <main className="workspace-right">{main}</main>
      </div>

      <div
        className={`drawer-overlay ${drawerOpen ? 'show' : ''}`}
        onClick={onCloseDrawer}
      />
      <aside
        className={`drawer ${drawerOpen ? 'show' : ''}`}
        onClick={event => event.stopPropagation()}
      >
        {sidebar}
      </aside>
    </div>
  );
}

export function ResponsiveShell({ mode, ...props }: ResponsiveShellProps) {
  return mode === 'desktop' ? (
    <DesktopShell {...props} />
  ) : (
    <MobileShell {...props} />
  );
}
