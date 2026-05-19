import fs from 'fs';
import path from 'path';
import {
  CHAT_USER_SCROLL_LOCK_MS,
  isChatUserScrollLocked,
  nextChatUserScrollLockUntil,
  resolveChatBottomScrollTop,
  resolveChatSessionReadWindowUpdate,
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

  test('delegates virtual row measurement to Virtuoso instead of manual resize retries', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const virtualList = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'chat', 'ChatVirtualTurnList.tsx'),
      'utf8',
    );

    expect(resolveChatBottomScrollTop({scrollHeight: 1200, clientHeight: 500})).toBe(700);
    expect(resolveChatBottomScrollTop({scrollHeight: 300, clientHeight: 500})).toBe(0);
    expect(virtualList).toContain("from 'react-virtuoso';");
    expect(mainTsx).not.toContain("container.querySelector<HTMLElement>('.chat-virtual-list') ?? container");
    expect(mainTsx).not.toContain('scrollChatToBottom(false);');
    expect(mainTsx).not.toContain('run(CHAT_BOTTOM_SCROLL_RETRY_FRAMES);');
    expect(mainTsx).not.toContain('keepSettling:');
  });

  test('keeps incremental session reads from resetting a history scroll window', () => {
    expect(
      resolveChatSessionReadWindowUpdate({
        useIncremental: true,
        followsLatest: false,
      }),
    ).toEqual({followLatest: false});
    expect(
      resolveChatSessionReadWindowUpdate({
        useIncremental: true,
        followsLatest: true,
      }),
    ).toEqual({followLatest: true});
    expect(
      resolveChatSessionReadWindowUpdate({
        useIncremental: false,
        followsLatest: false,
      }),
    ).toEqual({resetToLatest: true});
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
