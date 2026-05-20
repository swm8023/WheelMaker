import fs from 'fs';
import path from 'path';
import {
  CHAT_USER_SCROLL_LOCK_MS,
  isChatUserScrollLocked,
  nextChatUserScrollLockUntil,
  resolveChatSessionReadWindowUpdate,
  resolveChatScrollBottomTop,
  resolveChatScrollToBottomVisibility,
  shouldAutoScrollChatToBottom,
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

  test('delegates virtual row measurement and bottom scrolling to Virtuoso', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const virtualList = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'chat', 'ChatVirtuosoTurnList.tsx'),
      'utf8',
    );

    expect(virtualList).toContain("from 'react-virtuoso';");
    expect(virtualList).toContain('type VirtuosoHandle');
    expect(virtualList).toContain('totalListHeightChanged={handleTotalListHeightChanged}');
    expect(virtualList).toContain('virtuosoRef.current?.autoscrollToBottom();');
    expect(mainTsx).toContain("chatVirtuosoListRef.current?.scrollToBottom('auto');");
    expect(mainTsx).not.toContain('chatVirtuosoListRef.current?.autoscrollToBottom();');
    expect(mainTsx).not.toContain('container.scrollTop = nextScrollTop;');
    expect(mainTsx).not.toContain("container.querySelector<HTMLElement>('.chat-virtuoso-list') ?? container");
    expect(mainTsx).not.toContain('scrollChatToBottom(false);');
    expect(mainTsx).not.toContain('run(CHAT_BOTTOM_SCROLL_RETRY_FRAMES);');
    expect(mainTsx).not.toContain('keepSettling:');
  });

  test('keeps virtualizer item-count follow logic inside the Virtuoso wrapper', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const scrollIntent = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'chat', 'chatScrollIntent.ts'), 'utf8');
    const virtualList = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'chat', 'ChatVirtuosoTurnList.tsx'),
      'utf8',
    );

    expect(virtualList).toContain("followOutput={() => (shouldAutoscrollNow() ? 'auto' : false)}");
    expect(virtualList).toContain('totalListHeightChanged={handleTotalListHeightChanged}');
    expect(virtualList).toContain('requestScrollToLastDisplayItem(');
    expect(scrollIntent).not.toContain('resolveChatBottomFollowAction');
    expect(mainTsx).not.toContain('chatDisplayItemCountRef');
    expect(mainTsx).not.toContain('resolveChatBottomFollowAction');
    expect(mainTsx).not.toContain('chatBottomFollowAction');
    expect(mainTsx).not.toContain('autoscrollChatToBottom');
  });

  test('uses the app follow intent instead of stale Virtuoso bottom state for chat output following', () => {
    const projectRoot = path.join(__dirname, '..');
    const virtualList = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'chat', 'ChatVirtuosoTurnList.tsx'),
      'utf8',
    );

    expect(virtualList).toContain("followOutput={() => (shouldAutoscrollNow() ? 'auto' : false)}");
    expect(virtualList).toContain('if (shouldAutoscrollNow()) {');
    expect(virtualList).not.toContain('if (atBottomRef.current && shouldAutoscrollNow())');
  });

  test('shows the scroll-to-bottom button from the actual chat scroll container position', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(
      resolveChatScrollToBottomVisibility({
        scrollTop: 300,
        scrollHeight: 1200,
        clientHeight: 500,
        threshold: 80,
      }),
    ).toEqual({atBottom: false, showScrollToBottom: true});
    expect(
      resolveChatScrollToBottomVisibility({
        scrollTop: 620,
        scrollHeight: 1200,
        clientHeight: 500,
        threshold: 80,
      }),
    ).toEqual({atBottom: true, showScrollToBottom: false});
    expect(mainTsx).toContain('const handleChatScroll = useCallback((event: React.UIEvent<HTMLDivElement>) => {');
    expect(mainTsx).toContain('resolveChatScrollToBottomVisibility({');
    expect(mainTsx).toContain('onScroll={handleChatScroll}');
  });

  test('settles programmatic chat bottom scrolling against the actual scroll parent', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const virtualList = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'chat', 'ChatVirtuosoTurnList.tsx'),
      'utf8',
    );

    expect(resolveChatScrollBottomTop({scrollHeight: 1200, clientHeight: 500})).toBe(700);
    expect(resolveChatScrollBottomTop({scrollHeight: 300, clientHeight: 500})).toBe(0);
    expect(mainTsx).not.toContain('CHAT_HISTORY_BOTTOM_BUFFER');
    expect(mainTsx).not.toContain('bottomBuffer={CHAT_HISTORY_BOTTOM_BUFFER}');
    expect(virtualList).toContain('const scrollToLastDisplayItem = React.useCallback(');
    expect(virtualList).not.toContain('offset: virtuosoContext.bottomBuffer,');
    expect(virtualList).toContain('const requestScrollToLastDisplayItem = React.useCallback(');
    expect(virtualList).toContain('function scrollElementToBottom(');
    expect(virtualList).toContain('resolveChatScrollBottomTop({');
    expect(virtualList).toContain('const settleScrollParentToBottom = React.useCallback(');
    expect(virtualList).toContain('settleScrollParentToBottom(behavior);');
    expect(virtualList).toContain("settleScrollParentToBottom('auto');");
    expect(virtualList).toContain('onAtBottomChange?.(true);');
  });

  test('documents Virtuoso as the chat dynamic turn virtualizer', () => {
    const repositoryRoot = path.join(__dirname, '..', '..');
    const context = fs.readFileSync(path.join(repositoryRoot, 'CONTEXT.md'), 'utf8');

    expect(context).toContain('implemented with `react-virtuoso`');
    expect(context).toContain('Virtuoso Measurement Cache');
    expect(context).not.toContain('implemented with `@tanstack/react-virtual`');
    expect(context).not.toContain('Measured Height Cache');
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
