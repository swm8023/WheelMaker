import fs from 'fs';
import path from 'path';
import {
  GESTURE_LONG_PRESS_CANCEL_PX,
  GESTURE_LONG_PRESS_MS,
  GESTURE_MOVE_LONG_PRESS_MS,
  GESTURE_SELECTION_PX,
  resolveGestureDirectionCandidate,
  resolveGesturePressIntent,
  shouldStartGestureMove,
} from '../web/src/services/gestureNavigation';

function projectRoot(): string {
  return path.join(__dirname, '..');
}

function readMain(): string {
  return fs.readFileSync(path.join(projectRoot(), 'web', 'src', 'main.tsx'), 'utf8');
}

function readStyles(): string {
  return fs.readFileSync(path.join(projectRoot(), 'web', 'src', 'styles.css'), 'utf8');
}

function readWorkspacePersistence(): string {
  return fs.readFileSync(
    path.join(projectRoot(), 'web', 'src', 'services', 'workspacePersistence.ts'),
    'utf8',
  );
}

describe('gesture navigation', () => {
  test('persists gesture navigation as a default-off appearance preference', () => {
    const persistence = readWorkspacePersistence();

    expect(persistence).toContain('gestureNavigation: boolean;');
    expect(persistence).toContain("gestureNavigation: 'gestureNavigation',");
    expect(persistence).toContain('gestureNavigation: false,');
    expect(persistence).toContain(
      "gestureNavigation: typeof input.gestureNavigation === 'boolean' ? input.gestureNavigation : base.gestureNavigation",
    );
    expect(persistence).toContain(
      '{k: GLOBAL_KEYS.gestureNavigation, v: serialize(this.state.global.gestureNavigation), updatedAt: now}',
    );
    expect(persistence).toContain(
      '{k: GLOBAL_KEYS.gestureNavigation, v: serialize(next.gestureNavigation), updatedAt: now}',
    );
  });

  test('resolves pre-expansion press movement without distance-triggered drag', () => {
    expect(resolveGesturePressIntent({
      elapsedMs: GESTURE_LONG_PRESS_MS - 1,
      distancePx: GESTURE_LONG_PRESS_CANCEL_PX - 1,
    })).toBe('pressing');
    expect(resolveGesturePressIntent({
      elapsedMs: GESTURE_LONG_PRESS_MS,
      distancePx: GESTURE_LONG_PRESS_CANCEL_PX - 1,
    })).toBe('expand');
    expect(resolveGesturePressIntent({
      elapsedMs: GESTURE_LONG_PRESS_MS - 1,
      distancePx: GESTURE_LONG_PRESS_CANCEL_PX + 2,
    })).toBe('neutral');
    expect(resolveGesturePressIntent({
      elapsedMs: GESTURE_LONG_PRESS_MS - 1,
      distancePx: GESTURE_SELECTION_PX + 1,
    })).toBe('neutral');
  });

  test('only enters gesture movement after a two second hold without a navigation candidate', () => {
    expect(shouldStartGestureMove({
      elapsedMs: GESTURE_MOVE_LONG_PRESS_MS - 1,
      candidate: null,
    })).toBe(false);
    expect(shouldStartGestureMove({
      elapsedMs: GESTURE_MOVE_LONG_PRESS_MS,
      candidate: null,
    })).toBe(true);
    expect(shouldStartGestureMove({
      elapsedMs: GESTURE_MOVE_LONG_PRESS_MS,
      candidate: 'chat',
    })).toBe(false);
  });

  test('resolves gesture direction candidates from fixed mobile directions', () => {
    expect(resolveGestureDirectionCandidate({deltaX: 0, deltaY: -GESTURE_SELECTION_PX - 1})).toBe('chat');
    expect(resolveGestureDirectionCandidate({deltaX: -GESTURE_SELECTION_PX - 1, deltaY: 0})).toBe('file');
    expect(resolveGestureDirectionCandidate({deltaX: 0, deltaY: GESTURE_SELECTION_PX + 1})).toBe('git');
    expect(resolveGestureDirectionCandidate({deltaX: GESTURE_SELECTION_PX + 1, deltaY: 0})).toBeNull();
    expect(resolveGestureDirectionCandidate({deltaX: 0, deltaY: GESTURE_SELECTION_PX - 1})).toBeNull();
  });

  test('wires gesture navigation through appearance settings and mobile floating controls', () => {
    const main = readMain();

    expect(main).toContain("import {");
    expect(main).toContain("} from './services/gestureNavigation';");
    expect(main).toContain('const [gestureNavigation, setGestureNavigation] = useState(');
    expect(main).toContain('typeof persistedGlobal.gestureNavigation === \'boolean\'');
    expect(main).toContain('gestureNavigation,');
    expect(main).toContain('<span>Gesture Navigation</span>');
    expect(main).toContain('checked={gestureNavigation}');
    expect(main).toContain('onChange={e => setGestureNavigation(e.target.checked)}');
    expect(main).toContain("gestureNavigation ? (");
    expect(main).toContain('className="gesture-nav-control"');
    expect(main).toContain('className="gesture-nav-badge"');
    expect(main).toContain('className="floating-nav-group"');
    expect(main).toContain('data-gesture-nav-tab="chat"');
    expect(main).toContain('data-gesture-nav-tab="file"');
    expect(main).toContain('data-gesture-nav-tab="git"');
    expect(main).toContain('aria-label="Chat"');
    expect(main).toContain('aria-label="File"');
    expect(main).toContain('aria-label="Git"');
    expect(main).toContain("codicon-comment-discussion");
    expect(main).toContain("codicon-files");
    expect(main).toContain("codicon-source-control");
    expect(main).toContain('handleFloatingDrawerToggle');
    expect(main).toContain('handleFloatingNavSelect');
    expect(main).toContain('GESTURE_MOVE_LONG_PRESS_MS');
  });

  test('styles gesture navigation as circular drawer-centered controls', () => {
    const styles = readStyles();

    expect(styles).toContain('.gesture-nav-control');
    expect(styles).toContain('.gesture-nav-button');
    expect(styles).toContain('.gesture-nav-badge');
    expect(styles).toContain('.gesture-nav-option');
    expect(styles).toContain('.gesture-nav-option-chat');
    expect(styles).toContain('.gesture-nav-option-file');
    expect(styles).toContain('.gesture-nav-option-git');
    expect(styles).toContain(".gesture-nav-option[data-candidate='true']");
  });
});
