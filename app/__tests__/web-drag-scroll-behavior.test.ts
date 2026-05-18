import fs from 'fs';
import path from 'path';
import {
  CHAT_USER_SCROLL_LOCK_MS,
  isChatUserScrollLocked,
  nextChatUserScrollLockUntil,
  shouldAutoScrollChatToBottom,
  shouldHandleChatVirtualWindowScroll,
} from '../web/src/chat/chatScrollIntent';

describe('web drag scroll behavior', () => {
  test('prevents horizontal overscroll bounce while dragging code in file and git views', () => {
    const projectRoot = path.join(__dirname, '..');
    const styles = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(styles).toContain('.workspace-right {');
    expect(styles).toContain('overscroll-behavior-x: none;');
    expect(styles).toContain('.scroll-panel {');
    expect(styles).toContain('overscroll-behavior-x: contain;');
  });

  test('pauses chat auto-follow while the user is wheel scrolling', () => {
    const now = 1000;
    const lockUntil = nextChatUserScrollLockUntil(now);

    expect(lockUntil).toBe(now + CHAT_USER_SCROLL_LOCK_MS);
    expect(isChatUserScrollLocked(lockUntil, now + CHAT_USER_SCROLL_LOCK_MS - 1)).toBe(true);
    expect(isChatUserScrollLocked(lockUntil, now + CHAT_USER_SCROLL_LOCK_MS)).toBe(false);
    expect(
      shouldAutoScrollChatToBottom({
        force: false,
        followsLatest: true,
        pointerScrolling: false,
        userScrollLocked: true,
      }),
    ).toBe(false);
    expect(
      shouldAutoScrollChatToBottom({
        force: true,
        followsLatest: false,
        pointerScrolling: false,
        userScrollLocked: true,
      }),
    ).toBe(true);
  });

  test('ignores programmatic chat scrolls for virtual window expansion', () => {
    expect(shouldHandleChatVirtualWindowScroll(true)).toBe(false);
    expect(shouldHandleChatVirtualWindowScroll(false)).toBe(true);
  });

  test('keeps responding prompt animation from changing chat scroll overflow', () => {
    const projectRoot = path.join(__dirname, '..');
    const styles = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');
    const animationStart = styles.indexOf('@keyframes chat-prompt-dots-wave');
    const animationEnd = styles.indexOf('.chat-prompt-status-done', animationStart);
    const promptDotsAnimation = styles.slice(animationStart, animationEnd);

    expect(animationStart).toBeGreaterThanOrEqual(0);
    expect(animationEnd).toBeGreaterThan(animationStart);
    expect(promptDotsAnimation).not.toContain('transform:');
    expect(styles).toMatch(
      /\.chat-prompt-status-dots \{[\s\S]*contain: paint;[\s\S]*\}/,
    );
  });
});
