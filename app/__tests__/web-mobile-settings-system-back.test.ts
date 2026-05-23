import fs from 'fs';
import path from 'path';
import {
  createMobileSettingsHistoryState,
  isMobileSettingsHistoryState,
  mobileSettingsHistoryKey,
  resolveMobileSettingsPopAction,
} from '../web/src/services/mobileSettingsHistory';

function readMain(): string {
  return fs.readFileSync(path.join(__dirname, '..', 'web', 'src', 'main.tsx'), 'utf8');
}

function readStyles(): string {
  return fs.readFileSync(path.join(__dirname, '..', 'web', 'src', 'styles.css'), 'utf8');
}

describe('mobile settings system back', () => {
  test('marks and keys mobile settings history states', () => {
    const root = createMobileSettingsHistoryState(null);
    const update = createMobileSettingsHistoryState('update');

    expect(isMobileSettingsHistoryState(root)).toBe(true);
    expect(isMobileSettingsHistoryState(update)).toBe(true);
    expect(isMobileSettingsHistoryState({})).toBe(false);
    expect(mobileSettingsHistoryKey(null)).toBe('mobile-settings:root');
    expect(mobileSettingsHistoryKey('update')).toBe('mobile-settings:update');
  });

  test('resolves native back actions for settings layers', () => {
    expect(resolveMobileSettingsPopAction({
      nextState: createMobileSettingsHistoryState(null),
      settingsOpen: true,
      settingsDetailView: 'skills',
    })).toBe('back-to-list');

    expect(resolveMobileSettingsPopAction({
      nextState: null,
      settingsOpen: true,
      settingsDetailView: 'portRelay',
    })).toBe('close-settings');

    expect(resolveMobileSettingsPopAction({
      nextState: null,
      settingsOpen: true,
      settingsDetailView: null,
    })).toBe('close-settings');

    expect(resolveMobileSettingsPopAction({
      nextState: createMobileSettingsHistoryState(null),
      settingsOpen: true,
      settingsDetailView: null,
    })).toBe('none');

    expect(resolveMobileSettingsPopAction({
      nextState: null,
      settingsOpen: false,
      settingsDetailView: null,
    })).toBe('none');
  });

  test('wires mobile settings to history and mobile title actions', () => {
    const main = readMain();

    expect(main).toContain("} from './services/mobileSettingsHistory';");
    expect(main).toContain('window.history.pushState(createMobileSettingsHistoryState(settingsDetailView');
    expect(main).toContain("window.addEventListener('popstate', handleMobileSettingsPopState)");
    expect(main).toContain('resolveMobileSettingsPopAction({');
    expect(main).toContain('const mobileSettingsTitle = settingsDetailView');
    expect(main).toContain('const mobileSettingsActions = settingsDetailView');
    expect(main).toContain('renderSettingsContent(false, { hideDetailHeader: true })');
    expect(main).toContain('handleMobileSettingsBackButton');
    expect(main).toContain('renderSettingsDetailActions(settingsDetailView)');
    expect(main).not.toContain('mobileSettingsSwipe');
  });

  test('centers the mobile settings title independently from title bar actions', () => {
    const styles = readStyles();

    const navBlock = styles.match(/\.mobile-settings-nav \{[\s\S]*?\n\}/)?.[0] ?? '';
    const titleBlock = styles.match(/\.mobile-settings-title \{[\s\S]*?\n\}/)?.[0] ?? '';
    const actionsBlock = styles.match(/\.mobile-settings-actions \{[\s\S]*?\n\}/)?.[0] ?? '';

    expect(navBlock).toContain('position: relative;');
    expect(navBlock).toContain('height: calc(var(--wm-safe-area-top) + 52px);');
    expect(navBlock).toContain('align-items: center;');
    expect(navBlock).toContain('padding: var(--wm-safe-area-top) 12px 0;');
    expect(titleBlock).toContain('position: absolute;');
    expect(titleBlock).toContain('left: 50%;');
    expect(titleBlock).toContain('top: calc(var(--wm-safe-area-top) + 26px);');
    expect(titleBlock).toContain('transform: translate(-50%, -50%);');
    expect(actionsBlock).toContain('position: relative;');
    expect(actionsBlock).toContain('z-index: 1;');
  });
});
