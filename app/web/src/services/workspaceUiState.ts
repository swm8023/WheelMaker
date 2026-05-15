import type { LayoutMode } from './responsiveLayout';
import type {
  PersistedFloatingControlSlot,
  PersistedTab,
} from './workspacePersistence';

export type WorkspaceUiStateValue<T> = T | ((current: T) => T);

export const DESKTOP_SIDEBAR_WIDTH_MIN = 320;
export const DESKTOP_SIDEBAR_WIDTH_DEFAULT = 380;
export const DESKTOP_SIDEBAR_WIDTH_MAX = 560;

export type WorkspaceFloatingDragState = {
  active: boolean;
  pressing: boolean;
  pointerId: number;
  originY: number;
  startTop: number;
  currentTop: number;
  cooldownUntil: number;
};

export type WorkspaceUiState = {
  shared: {
    tab: PersistedTab;
    settingsOpen: boolean;
    collapsedProjectIds: string[];
    pinnedProjectIds: string[];
  };
  desktop: {
    sidebarCollapsed: boolean;
    sidebarWidth: number;
  };
  mobile: {
    drawerOpen: boolean;
    floatingControlSlot: PersistedFloatingControlSlot;
    chatConfigOverflowOpen: boolean;
  };
  transient: {
    chatKeyboardInset: number;
    floatingKeyboardOffset: number;
    floatingDragState: WorkspaceFloatingDragState | null;
  };
};

export type WorkspaceUiStateInput = {
  tab?: unknown;
  settingsOpen?: unknown;
  sidebarCollapsed?: unknown;
  desktopSidebarWidth?: unknown;
  collapsedProjectIds?: unknown;
  desktopCollapsedProjectIds?: unknown;
  pinnedProjectIds?: unknown;
  drawerOpen?: unknown;
  floatingControlSlot?: unknown;
  chatConfigOverflowOpen?: unknown;
  chatKeyboardInset?: unknown;
  floatingKeyboardOffset?: unknown;
  floatingDragState?: WorkspaceFloatingDragState | null;
};

export type WorkspaceUiAction =
  | { type: 'shared/setTab'; next: WorkspaceUiStateValue<PersistedTab> }
  | { type: 'shared/setSettingsOpen'; next: WorkspaceUiStateValue<boolean> }
  | { type: 'shared/setCollapsedProjectIds'; next: WorkspaceUiStateValue<string[]> }
  | { type: 'shared/setPinnedProjectIds'; next: WorkspaceUiStateValue<string[]> }
  | { type: 'desktop/setSidebarCollapsed'; next: WorkspaceUiStateValue<boolean> }
  | { type: 'desktop/setSidebarWidth'; next: WorkspaceUiStateValue<number> }
  | { type: 'mobile/setDrawerOpen'; next: WorkspaceUiStateValue<boolean> }
  | {
      type: 'mobile/setFloatingControlSlot';
      next: WorkspaceUiStateValue<PersistedFloatingControlSlot>;
    }
  | {
      type: 'mobile/setChatConfigOverflowOpen';
      next: WorkspaceUiStateValue<boolean>;
    }
  | { type: 'transient/setChatKeyboardInset'; next: WorkspaceUiStateValue<number> }
  | {
      type: 'transient/setFloatingKeyboardOffset';
      next: WorkspaceUiStateValue<number>;
    }
  | {
      type: 'transient/setFloatingDragState';
      next: WorkspaceUiStateValue<WorkspaceFloatingDragState | null>;
    }
  | { type: 'layout/modeChanged'; from: LayoutMode; to: LayoutMode };

function resolveNext<T>(current: T, next: WorkspaceUiStateValue<T>): T {
  return typeof next === 'function'
    ? (next as (current: T) => T)(current)
    : next;
}

function sanitizeTab(value: unknown): PersistedTab {
  return value === 'chat' || value === 'git' ? value : 'file';
}

function sanitizeFloatingControlSlot(value: unknown): PersistedFloatingControlSlot {
  return value === 'upper' ||
    value === 'upper-middle' ||
    value === 'center' ||
    value === 'lower-middle'
    ? value
    : 'upper-middle';
}

function sanitizeInset(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(0, Math.round(value))
    : 0;
}

export function sanitizeDesktopSidebarWidth(
  value: unknown,
  fallback = DESKTOP_SIDEBAR_WIDTH_DEFAULT,
): number {
  const numeric = typeof value === 'number' && Number.isFinite(value) ? value : fallback;
  return Math.min(
    DESKTOP_SIDEBAR_WIDTH_MAX,
    Math.max(DESKTOP_SIDEBAR_WIDTH_MIN, Math.round(numeric)),
  );
}

function sanitizeStringList(value: unknown): string[] {
  return Array.isArray(value)
    ? Array.from(new Set(value.filter(item => typeof item === 'string' && item)))
    : [];
}

function resetTransientState(): WorkspaceUiState['transient'] {
  return {
    chatKeyboardInset: 0,
    floatingKeyboardOffset: 0,
    floatingDragState: null,
  };
}

export function createWorkspaceUiState(input: WorkspaceUiStateInput = {}): WorkspaceUiState {
  return {
    shared: {
      tab: sanitizeTab(input.tab),
      settingsOpen: typeof input.settingsOpen === 'boolean' ? input.settingsOpen : false,
      collapsedProjectIds: sanitizeStringList(
        Array.isArray(input.collapsedProjectIds)
          ? input.collapsedProjectIds
          : input.desktopCollapsedProjectIds,
      ),
      pinnedProjectIds: sanitizeStringList(input.pinnedProjectIds),
    },
    desktop: {
      sidebarCollapsed:
        typeof input.sidebarCollapsed === 'boolean' ? input.sidebarCollapsed : false,
      sidebarWidth: sanitizeDesktopSidebarWidth(input.desktopSidebarWidth),
    },
    mobile: {
      drawerOpen: typeof input.drawerOpen === 'boolean' ? input.drawerOpen : false,
      floatingControlSlot: sanitizeFloatingControlSlot(input.floatingControlSlot),
      chatConfigOverflowOpen:
        typeof input.chatConfigOverflowOpen === 'boolean'
          ? input.chatConfigOverflowOpen
          : false,
    },
    transient: {
      chatKeyboardInset: sanitizeInset(input.chatKeyboardInset),
      floatingKeyboardOffset: sanitizeInset(input.floatingKeyboardOffset),
      floatingDragState: input.floatingDragState ?? null,
    },
  };
}

export function workspaceUiReducer(
  state: WorkspaceUiState,
  action: WorkspaceUiAction,
): WorkspaceUiState {
  switch (action.type) {
    case 'shared/setTab':
      return {
        ...state,
        shared: {
          ...state.shared,
          tab: sanitizeTab(resolveNext(state.shared.tab, action.next)),
        },
      };
    case 'shared/setSettingsOpen':
      return {
        ...state,
        shared: {
          ...state.shared,
          settingsOpen: !!resolveNext(state.shared.settingsOpen, action.next),
        },
      };
    case 'shared/setCollapsedProjectIds':
      return {
        ...state,
        shared: {
          ...state.shared,
          collapsedProjectIds: sanitizeStringList(
            resolveNext(state.shared.collapsedProjectIds, action.next),
          ),
        },
      };
    case 'shared/setPinnedProjectIds':
      return {
        ...state,
        shared: {
          ...state.shared,
          pinnedProjectIds: sanitizeStringList(
            resolveNext(state.shared.pinnedProjectIds, action.next),
          ),
        },
      };
    case 'desktop/setSidebarCollapsed':
      return {
        ...state,
        desktop: {
          ...state.desktop,
          sidebarCollapsed: !!resolveNext(state.desktop.sidebarCollapsed, action.next),
        },
      };
    case 'desktop/setSidebarWidth':
      return {
        ...state,
        desktop: {
          ...state.desktop,
          sidebarWidth: sanitizeDesktopSidebarWidth(
            resolveNext(state.desktop.sidebarWidth, action.next),
            state.desktop.sidebarWidth,
          ),
        },
      };
    case 'mobile/setDrawerOpen':
      return {
        ...state,
        mobile: {
          ...state.mobile,
          drawerOpen: !!resolveNext(state.mobile.drawerOpen, action.next),
        },
      };
    case 'mobile/setFloatingControlSlot':
      return {
        ...state,
        mobile: {
          ...state.mobile,
          floatingControlSlot: sanitizeFloatingControlSlot(
            resolveNext(state.mobile.floatingControlSlot, action.next),
          ),
        },
      };
    case 'mobile/setChatConfigOverflowOpen':
      return {
        ...state,
        mobile: {
          ...state.mobile,
          chatConfigOverflowOpen: !!resolveNext(
            state.mobile.chatConfigOverflowOpen,
            action.next,
          ),
        },
      };
    case 'transient/setChatKeyboardInset':
      return {
        ...state,
        transient: {
          ...state.transient,
          chatKeyboardInset: sanitizeInset(
            resolveNext(state.transient.chatKeyboardInset, action.next),
          ),
        },
      };
    case 'transient/setFloatingKeyboardOffset':
      return {
        ...state,
        transient: {
          ...state.transient,
          floatingKeyboardOffset: sanitizeInset(
            resolveNext(state.transient.floatingKeyboardOffset, action.next),
          ),
        },
      };
    case 'transient/setFloatingDragState':
      return {
        ...state,
        transient: {
          ...state.transient,
          floatingDragState: resolveNext(state.transient.floatingDragState, action.next),
        },
      };
    case 'layout/modeChanged':
      if (action.from === action.to) {
        return state;
      }
      return {
        ...state,
        mobile: {
          ...state.mobile,
          drawerOpen: false,
          chatConfigOverflowOpen: false,
        },
        transient: resetTransientState(),
      };
    default:
      return state;
  }
}
