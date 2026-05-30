import fs from 'fs';
import path from 'path';

describe('web responsive ui state', () => {
  test('resolves mobile floating control side with a center hysteresis band', () => {
    const projectRoot = path.join(__dirname, '..');
    const modulePath = path.join(projectRoot, 'web', 'src', 'services', 'mobileFloatingControls.ts');

    expect(fs.existsSync(modulePath)).toBe(true);

    const {
      FLOATING_CONTROL_SIDE_HYSTERESIS_PX,
      resolveFloatingControlDragSide,
    } = require(modulePath);

    expect(FLOATING_CONTROL_SIDE_HYSTERESIS_PX).toBe(24);
    expect(resolveFloatingControlDragSide('right', 377, 800)).toBe('right');
    expect(resolveFloatingControlDragSide('right', 375, 800)).toBe('left');
    expect(resolveFloatingControlDragSide('left', 423, 800)).toBe('left');
    expect(resolveFloatingControlDragSide('left', 425, 800)).toBe('right');
  });

  test('stores mobile floating control height as a continuous clamped ratio', () => {
    const projectRoot = path.join(__dirname, '..');
    const modulePath = path.join(projectRoot, 'web', 'src', 'services', 'mobileFloatingControls.ts');

    expect(fs.existsSync(modulePath)).toBe(true);

    const {
      FLOATING_CONTROL_DEFAULT_Y_RATIO,
      FLOATING_CONTROL_COMPOSER_GAP_PX,
      floatingControlTopFromYRatio,
      floatingControlYRatioFromLegacySlot,
      floatingControlYRatioFromTop,
      resolveFloatingControlAvoidanceBounds,
      resolveFloatingControlDefaultBounds,
      resolveFloatingControlYRatioForBoundsChange,
      resolveFloatingControlYRatioForStableTop,
      sanitizeFloatingControlYRatio,
    } = require(modulePath);

    expect(FLOATING_CONTROL_DEFAULT_Y_RATIO).toBe(0.25);
    expect(FLOATING_CONTROL_COMPOSER_GAP_PX).toBe(12);
    expect(sanitizeFloatingControlYRatio(-0.4)).toBe(0);
    expect(sanitizeFloatingControlYRatio(1.4)).toBe(1);
    expect(sanitizeFloatingControlYRatio(Number.NaN)).toBe(0.25);
    expect(floatingControlTopFromYRatio(0.4, 10, 210)).toBe(90);
    expect(floatingControlYRatioFromTop(90, 10, 210)).toBe(0.4);
    expect(floatingControlYRatioFromTop(999, 10, 210)).toBe(1);
    const stableExpandedRatio = resolveFloatingControlYRatioForStableTop({
      previousTop: 90,
      minTop: 10,
      maxTop: 310,
      fallbackRatio: 0.4,
    });
    expect(floatingControlTopFromYRatio(stableExpandedRatio, 10, 310)).toBe(90);
    expect(resolveFloatingControlYRatioForStableTop({
      previousTop: 999,
      minTop: 10,
      maxTop: 310,
      fallbackRatio: 0.4,
    })).toBe(1);
    expect(resolveFloatingControlYRatioForBoundsChange({
      previousTop: 260,
      previousHadDefaultComposerTop: false,
      nextHasDefaultComposerTop: true,
      minTop: 10,
      maxTop: 310,
      fallbackRatio: 0.4,
    })).toBe(0.4);
    expect(resolveFloatingControlYRatioForBoundsChange({
      previousTop: 90,
      previousHadDefaultComposerTop: true,
      nextHasDefaultComposerTop: true,
      minTop: 10,
      maxTop: 310,
      fallbackRatio: 0.4,
    })).toBeCloseTo(0.2667, 4);
    const defaultBounds = resolveFloatingControlDefaultBounds({
      viewportHeight: 800,
      stackHeight: 184,
      safeAreaTopInset: 0,
      safeAreaBottomInset: 0,
      defaultComposerTop: 620,
    });
    expect(defaultBounds).toEqual({minTop: 6, maxTop: 424});
    expect(resolveFloatingControlAvoidanceBounds({
      defaultBounds,
      viewportHeight: 800,
      keyboardOffset: 240,
      stackHeight: 184,
      safeAreaBottomInset: 0,
      composerTop: null,
    })).toEqual({minTop: 6, maxTop: 370});
    expect(resolveFloatingControlAvoidanceBounds({
      defaultBounds,
      viewportHeight: 800,
      keyboardOffset: 0,
      stackHeight: 184,
      safeAreaBottomInset: 0,
      composerTop: 560,
    })).toEqual({minTop: 6, maxTop: 364});
    expect(resolveFloatingControlAvoidanceBounds({
      defaultBounds,
      viewportHeight: 800,
      keyboardOffset: 0,
      stackHeight: 184,
      safeAreaBottomInset: 0,
      composerTop: null,
    })).toEqual(defaultBounds);
    expect(floatingControlYRatioFromLegacySlot('upper')).toBe(0);
    expect(floatingControlYRatioFromLegacySlot('upper-middle')).toBe(0.25);
    expect(floatingControlYRatioFromLegacySlot('center')).toBe(0.5);
    expect(floatingControlYRatioFromLegacySlot('lower-middle')).toBe(0.75);
    expect(floatingControlYRatioFromLegacySlot('lower')).toBe(1);
    expect(floatingControlYRatioFromLegacySlot('invalid')).toBeNull();
  });

  test('keeps the chat scroll-to-bottom button above the composer and keyboard inset', () => {
    const projectRoot = path.join(__dirname, '..');
    const modulePath = path.join(projectRoot, 'web', 'src', 'services', 'chatScrollBottomButton.ts');

    expect(fs.existsSync(modulePath)).toBe(true);

    const {
      CHAT_SCROLL_BOTTOM_COMPOSER_GAP_PX,
      CHAT_SCROLL_BOTTOM_FALLBACK_OFFSET_PX,
      resolveChatScrollBottomButtonOffset,
    } = require(modulePath);

    expect(CHAT_SCROLL_BOTTOM_COMPOSER_GAP_PX).toBe(10);
    expect(CHAT_SCROLL_BOTTOM_FALLBACK_OFFSET_PX).toBe(92);
    expect(resolveChatScrollBottomButtonOffset({
      composerHeight: 84,
      keyboardInset: 0,
    })).toBe(94);
    expect(resolveChatScrollBottomButtonOffset({
      composerHeight: 128,
      keyboardInset: 240,
    })).toBe(378);
    expect(resolveChatScrollBottomButtonOffset({
      composerHeight: 0,
      keyboardInset: 240,
    })).toBe(332);
    expect(resolveChatScrollBottomButtonOffset({
      composerHeight: Number.NaN,
      keyboardInset: -20,
    })).toBe(92);
  });

  test('uses a best-effort mobile haptic helper around navigator vibration', () => {
    const projectRoot = path.join(__dirname, '..');
    const modulePath = path.join(projectRoot, 'web', 'src', 'services', 'mobileHaptics.ts');

    expect(fs.existsSync(modulePath)).toBe(true);

    const {
      MOBILE_HAPTIC_LIGHT_MS,
      triggerMobileHaptic,
    } = require(modulePath);
    const originalNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator');

    try {
      const vibrate = jest.fn();
      Object.defineProperty(globalThis, 'navigator', {
        configurable: true,
        value: { vibrate },
      });
      triggerMobileHaptic();
      triggerMobileHaptic(8);
      expect(vibrate).toHaveBeenNthCalledWith(1, MOBILE_HAPTIC_LIGHT_MS);
      expect(vibrate).toHaveBeenNthCalledWith(2, 8);

      Object.defineProperty(globalThis, 'navigator', {
        configurable: true,
        value: { vibrate: () => { throw new Error('unsupported'); } },
      });
      expect(() => triggerMobileHaptic()).not.toThrow();

      Object.defineProperty(globalThis, 'navigator', {
        configurable: true,
        value: {},
      });
      expect(() => triggerMobileHaptic()).not.toThrow();
    } finally {
      if (originalNavigator) {
        Object.defineProperty(globalThis, 'navigator', originalNavigator);
      } else {
        delete (globalThis as { navigator?: unknown }).navigator;
      }
    }
  });

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
      DESKTOP_SIDEBAR_WIDTH_DEFAULT,
      DESKTOP_SIDEBAR_WIDTH_MAX,
      DESKTOP_SIDEBAR_WIDTH_MIN,
      createWorkspaceUiState,
      workspaceUiReducer,
    } = require(modulePath);

    expect(DESKTOP_SIDEBAR_WIDTH_DEFAULT).toBe(380);
    expect(DESKTOP_SIDEBAR_WIDTH_MIN).toBe(320);
    expect(DESKTOP_SIDEBAR_WIDTH_MAX).toBe(560);

    let state = createWorkspaceUiState({
      tab: 'git',
      settingsOpen: true,
      sidebarCollapsed: true,
      desktopSidebarWidth: 420,
      drawerOpen: true,
      collapsedProjectIds: ['project-a', 'project-b', 'project-a'],
      pinnedProjectIds: ['project-c', 'project-a', 'project-c'],
      floatingControlYRatio: 0.42,
      floatingControlSide: 'left',
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
      sidebarWidth: 420,
    });
    expect(createWorkspaceUiState({ desktopSidebarWidth: 200 }).desktop.sidebarWidth).toBe(320);
    expect(createWorkspaceUiState({ desktopSidebarWidth: 700 }).desktop.sidebarWidth).toBe(560);
    expect(state.mobile).toMatchObject({
      drawerOpen: true,
      floatingControlYRatio: 0.42,
      floatingControlSide: 'left',
      chatConfigOverflowOpen: true,
    });
    expect(createWorkspaceUiState({ floatingControlSide: 'invalid' }).mobile.floatingControlSide).toBe('right');
    expect(createWorkspaceUiState({ floatingControlYRatio: 3 }).mobile.floatingControlYRatio).toBe(1);

    state = workspaceUiReducer(state, {
      type: 'layout/modeChanged',
      from: 'mobile',
      to: 'desktop',
    });

    expect(state.shared.settingsOpen).toBe(true);
    expect(state.shared.collapsedProjectIds).toEqual(['project-a', 'project-b']);
    expect(state.shared.pinnedProjectIds).toEqual(['project-c', 'project-a']);
    expect(state.desktop.sidebarCollapsed).toBe(true);
    expect(state.desktop.sidebarWidth).toBe(420);
    expect(state.mobile.floatingControlYRatio).toBe(0.42);
    expect(state.mobile.floatingControlSide).toBe('left');
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

    state = workspaceUiReducer(state, {
      type: 'mobile/setFloatingControlSide',
      next: 'right',
    });

    expect(state.mobile.floatingControlSide).toBe('right');

    state = workspaceUiReducer(state, {
      type: 'mobile/setFloatingControlYRatio',
      next: 0.83,
    });

    expect(state.mobile.floatingControlYRatio).toBe(0.83);

    state = workspaceUiReducer(state, {
      type: 'desktop/setSidebarWidth',
      next: 999,
    });

    expect(state.desktop.sidebarWidth).toBe(560);
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
    expect(mainTsx).toContain('desktopSidebarWidth: globalState.desktopSidebarWidth');
    expect(mainTsx).toContain('pinnedProjectIds: globalState.pinnedProjectIds ?? []');
    expect(mainTsx).toContain('floatingControlSide: globalState.floatingControlSide ?? readPortRelayFloatingSide() ?? \'right\'');
    expect(mainTsx).toContain('floatingControlYRatio,\n      floatingControlSide,\n      desktopSidebarWidth,');
    expect(mainTsx).toContain('const desktopSidebarWidth = workspaceUiState.desktop.sidebarWidth;');
    expect(mainTsx).toContain('const collapsedProjectIds = workspaceUiState.shared.collapsedProjectIds;');
    expect(mainTsx).toContain('const pinnedProjectIds = workspaceUiState.shared.pinnedProjectIds;');
    expect(mainTsx).toContain("dispatchWorkspaceUi({ type: 'desktop/setSidebarWidth', next });");
    expect(mainTsx).toContain("dispatchWorkspaceUi({ type: 'shared/setCollapsedProjectIds', next });");
    expect(mainTsx).toContain("dispatchWorkspaceUi({ type: 'shared/setPinnedProjectIds', next });");
  });

  test('persists desktop sidebar width as global app state', () => {
    const projectRoot = path.join(__dirname, '..');
    const persistenceTs = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'),
      'utf8',
    );

    expect(persistenceTs).toContain('desktopSidebarWidth: number;');
    expect(persistenceTs).toContain("export type PersistedFloatingControlSide = 'left' | 'right';");
    expect(persistenceTs).toContain('floatingControlYRatio: number;');
    expect(persistenceTs).toContain('floatingControlSide: PersistedFloatingControlSide;');
    expect(persistenceTs).not.toContain('useLatestPromptTitle: boolean;');
    expect(persistenceTs).toContain("desktopSidebarWidth: 'desktopSidebarWidth',");
    expect(persistenceTs).toContain("floatingControlYRatio: 'floatingControlYRatio',");
    expect(persistenceTs).toContain("floatingControlSlot: 'floatingControlSlot',");
    expect(persistenceTs).toContain("floatingControlSide: 'floatingControlSide',");
    expect(persistenceTs).not.toContain("useLatestPromptTitle: 'useLatestPromptTitle',");
    expect(persistenceTs).toContain('desktopSidebarWidth: 380,');
    expect(persistenceTs).toContain('floatingControlYRatio: FLOATING_CONTROL_DEFAULT_Y_RATIO,');
    expect(persistenceTs).toContain("floatingControlSide: 'right',");
    expect(persistenceTs).not.toContain('useLatestPromptTitle: false,');
    expect(persistenceTs).not.toContain('useLatestPromptTitle: typeof input.useLatestPromptTitle');
    expect(persistenceTs).toContain('desktopSidebarWidth: sanitizeDesktopSidebarWidth(input.desktopSidebarWidth, base.desktopSidebarWidth),');
    expect(persistenceTs).toContain('floatingControlYRatio,');
    expect(persistenceTs).toContain('floatingControlSide: sanitizeFloatingControlSide(input.floatingControlSide, base.floatingControlSide),');
    expect(persistenceTs).toContain(
      '{k: GLOBAL_KEYS.desktopSidebarWidth, v: serialize(this.state.global.desktopSidebarWidth), updatedAt: now}',
    );
    expect(persistenceTs).not.toContain('GLOBAL_KEYS.useLatestPromptTitle');
    expect(persistenceTs).toContain(
      '{k: GLOBAL_KEYS.desktopSidebarWidth, v: serialize(next.desktopSidebarWidth), updatedAt: now}',
    );
    expect(persistenceTs).toContain(
      '{k: GLOBAL_KEYS.floatingControlYRatio, v: serialize(next.floatingControlYRatio), updatedAt: now}',
    );
    expect(persistenceTs).toContain(
      '{k: GLOBAL_KEYS.floatingControlSide, v: serialize(next.floatingControlSide), updatedAt: now}',
    );
    expect(persistenceTs).not.toContain('next.useLatestPromptTitle');
  });
});
