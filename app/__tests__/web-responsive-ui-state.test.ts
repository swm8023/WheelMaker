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
      collapsedProjectIds: ['project-a', 'project-b', 'project-a'],
      pinnedProjectIds: ['project-c', 'project-a', 'project-c'],
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
      collapsedProjectIds: ['project-a', 'project-b'],
      pinnedProjectIds: ['project-c', 'project-a'],
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
    expect(state.shared.collapsedProjectIds).toEqual(['project-a', 'project-b']);
    expect(state.shared.pinnedProjectIds).toEqual(['project-c', 'project-a']);
    expect(state.desktop.sidebarCollapsed).toBe(true);
    expect(state.mobile.floatingControlSlot).toBe('center');
    expect(state.mobile.drawerOpen).toBe(false);
    expect(state.mobile.chatConfigOverflowOpen).toBe(false);
    expect(state.transient.chatKeyboardInset).toBe(0);
    expect(state.transient.floatingKeyboardOffset).toBe(0);
    expect(state.transient.floatingDragState).toBeNull();

    state = workspaceUiReducer(state, {
      type: 'shared/setPinnedProjectIds',
      next: ['project-b', 'project-b', 'project-c'],
    });

    expect(state.shared.pinnedProjectIds).toEqual(['project-b', 'project-c']);
  });

  test('sorts pinned projects above unpinned projects while preserving registry order', () => {
    const projectRoot = path.join(__dirname, '..');
    const modulePath = path.join(projectRoot, 'web', 'src', 'services', 'projectNavigation.ts');

    expect(fs.existsSync(modulePath)).toBe(true);

    const {
      sortProjectsByPin,
      togglePinnedProjectId,
    } = require(modulePath);

    const projects = [
      { projectId: 'project-a', name: 'A' },
      { projectId: 'project-b', name: 'B' },
      { projectId: 'project-c', name: 'C' },
      { projectId: 'project-d', name: 'D' },
    ];

    expect(sortProjectsByPin(projects, ['project-c', 'missing', 'project-a']).map(item => item.projectId)).toEqual([
      'project-a',
      'project-c',
      'project-b',
      'project-d',
    ]);
    expect(togglePinnedProjectId(['project-a'], 'project-c')).toEqual(['project-a', 'project-c']);
    expect(togglePinnedProjectId(['project-a', 'project-c'], 'project-a')).toEqual(['project-c']);
  });

  test('main web app uses the responsive layout and workspace ui state modules', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("from './services/responsiveLayout'");
    expect(mainTsx).toContain("from './services/workspaceUiState'");
    expect(mainTsx).toContain('const layoutMode = resolveLayoutMode(windowWidth);');
    expect(mainTsx).toContain("const isWide = layoutMode === 'desktop';");
    expect(mainTsx).toContain('const [workspaceUiState, dispatchWorkspaceUi] = useReducer(');
    expect(mainTsx).toContain('collapsedProjectIds: globalState.collapsedProjectIds ?? globalState.desktopCollapsedProjectIds ?? []');
    expect(mainTsx).toContain('pinnedProjectIds: globalState.pinnedProjectIds ?? []');
    expect(mainTsx).toContain('const collapsedProjectIds = workspaceUiState.shared.collapsedProjectIds;');
    expect(mainTsx).toContain('const pinnedProjectIds = workspaceUiState.shared.pinnedProjectIds;');
    expect(mainTsx).toContain("dispatchWorkspaceUi({ type: 'shared/setCollapsedProjectIds', next });");
    expect(mainTsx).toContain("dispatchWorkspaceUi({ type: 'shared/setPinnedProjectIds', next });");
  });
});
