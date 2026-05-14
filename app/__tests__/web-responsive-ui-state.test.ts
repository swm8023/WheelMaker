import fs from 'fs';
import path from 'path';

describe('web responsive ui state', () => {
  test('centralizes viewport layout mode resolution at the 900px shell breakpoint', () => {
    const projectRoot = path.join(__dirname, '..');
    const modulePath = path.join(projectRoot, 'web', 'src', 'services', 'responsiveLayout.ts');

    expect(fs.existsSync(modulePath)).toBe(true);

    const {
      LAYOUT_MODE_BREAKPOINT_PX,
      resolveLayoutMode,
    } = require(modulePath);

    expect(LAYOUT_MODE_BREAKPOINT_PX).toBe(900);
    expect(resolveLayoutMode(899)).toBe('mobile');
    expect(resolveLayoutMode(900)).toBe('desktop');
    expect(resolveLayoutMode(1200)).toBe('desktop');
  });

  test('keeps shared, desktop, mobile, and transient ui state under one reducer', () => {
    const projectRoot = path.join(__dirname, '..');
    const modulePath = path.join(projectRoot, 'web', 'src', 'services', 'workspaceUiState.ts');

    expect(fs.existsSync(modulePath)).toBe(true);

    const {
      createWorkspaceUiState,
      workspaceUiReducer,
    } = require(modulePath);

    let state = createWorkspaceUiState({
      tab: 'git',
      settingsOpen: true,
      sidebarCollapsed: true,
      drawerOpen: true,
      floatingControlSlot: 'center',
      chatConfigOverflowOpen: true,
      chatKeyboardInset: 120,
      floatingKeyboardOffset: 120,
      floatingDragState: {
        active: true,
        pressing: false,
        pointerId: 1,
        originY: 10,
        startTop: 40,
        currentTop: 80,
        cooldownUntil: 0,
      },
    });

    expect(state.shared).toMatchObject({
      tab: 'git',
      settingsOpen: true,
    });
    expect(state.desktop).toMatchObject({
      sidebarCollapsed: true,
    });
    expect(state.mobile).toMatchObject({
      drawerOpen: true,
      floatingControlSlot: 'center',
      chatConfigOverflowOpen: true,
    });

    state = workspaceUiReducer(state, {
      type: 'layout/modeChanged',
      from: 'mobile',
      to: 'desktop',
    });

    expect(state.shared.settingsOpen).toBe(true);
    expect(state.desktop.sidebarCollapsed).toBe(true);
    expect(state.mobile.floatingControlSlot).toBe('center');
    expect(state.mobile.drawerOpen).toBe(false);
    expect(state.mobile.chatConfigOverflowOpen).toBe(false);
    expect(state.transient.chatKeyboardInset).toBe(0);
    expect(state.transient.floatingKeyboardOffset).toBe(0);
    expect(state.transient.floatingDragState).toBeNull();
  });

  test('main web app uses the responsive layout and workspace ui state modules', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("from './services/responsiveLayout'");
    expect(mainTsx).toContain("from './services/workspaceUiState'");
    expect(mainTsx).toContain('const layoutMode = resolveLayoutMode(windowWidth);');
    expect(mainTsx).toContain("const isWide = layoutMode === 'desktop';");
    expect(mainTsx).toContain('const [workspaceUiState, dispatchWorkspaceUi] = useReducer(');
  });
});
