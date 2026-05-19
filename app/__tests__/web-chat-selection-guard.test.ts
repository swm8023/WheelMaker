import {
  resolveChatListSelection,
  resolveSelectedChatVisibilityRecovery,
  shouldApplyPreservedChatLoad,
} from '../web/src/chat/chatSelectionGuard';
import { chatSessionKeyFromParts, encodeChatSessionKey } from '../web/src/chat/chatSessionKey';

describe('web chat selection guards', () => {
  test('preserved session reads can only write back to the same composite selected key', () => {
    const selected = chatSessionKeyFromParts('project-a', 'session-a');
    const staleSnapshot = encodeChatSessionKey(chatSessionKeyFromParts('project-b', 'session-b'));

    expect(shouldApplyPreservedChatLoad(selected, staleSnapshot)).toBe(false);
    expect(shouldApplyPreservedChatLoad(selected, encodeChatSessionKey(selected))).toBe(true);
    expect(shouldApplyPreservedChatLoad(selected, '')).toBe(true);
  });

  test('list loading prefers the live selected session over a stale preferred session', () => {
    expect(
      resolveChatListSelection({
        activeProjectId: 'project-a',
        availableSessionIds: ['session-b', 'session-a'],
        currentKey: chatSessionKeyFromParts('project-a', 'session-a'),
        persistedKey: chatSessionKeyFromParts('project-a', 'session-b'),
        preferredSelection: 'session-b',
      }),
    ).toEqual({sessionId: 'session-a', canMutateSelection: true});
  });

  test('background project list loading cannot steal a different selected chat', () => {
    expect(
      resolveChatListSelection({
        activeProjectId: 'project-b',
        availableSessionIds: ['session-b'],
        currentKey: chatSessionKeyFromParts('project-a', 'session-a'),
        persistedKey: chatSessionKeyFromParts('project-b', 'session-b'),
        preferredSelection: 'session-b',
      }),
    ).toEqual({sessionId: '', canMutateSelection: false});
  });

  test('online list loading preserves the live selected session during transient list gaps', () => {
    expect(
      resolveChatListSelection({
        activeProjectId: 'project-a',
        availableSessionIds: ['session-newest'],
        currentKey: chatSessionKeyFromParts('project-a', 'session-missing'),
        persistedKey: chatSessionKeyFromParts('project-a', 'session-missing'),
      }),
    ).toEqual({sessionId: 'session-missing', canMutateSelection: true});
  });

  test('online list loading falls back when only a stale persisted selection is missing', () => {
    expect(
      resolveChatListSelection({
        activeProjectId: 'project-a',
        availableSessionIds: ['session-newest'],
        currentKey: null,
        persistedKey: chatSessionKeyFromParts('project-a', 'session-missing'),
      }),
    ).toEqual({sessionId: 'session-newest', canMutateSelection: true});
  });

  test('recovers selected chat visibility after a reload leaves the panel empty', () => {
    expect(
      resolveSelectedChatVisibilityRecovery({
        tab: 'chat',
        connected: true,
        chatLoading: false,
        selectedRuntimeKey: 'project-a/session-a',
        visibleRuntimeKey: '',
        visibleMessageCount: 0,
        cachedMessageCount: 3,
        attemptedRuntimeKey: '',
      }),
    ).toBe('restore-cache');
    expect(
      resolveSelectedChatVisibilityRecovery({
        tab: 'chat',
        connected: true,
        chatLoading: false,
        selectedRuntimeKey: 'project-a/session-a',
        visibleRuntimeKey: 'project-a/session-a',
        visibleMessageCount: 0,
        cachedMessageCount: 0,
        attemptedRuntimeKey: '',
      }),
    ).toBe('read-session');
    expect(
      resolveSelectedChatVisibilityRecovery({
        tab: 'chat',
        connected: true,
        chatLoading: false,
        selectedRuntimeKey: 'project-a/session-a',
        visibleRuntimeKey: 'project-a/session-a',
        visibleMessageCount: 0,
        cachedMessageCount: 0,
        attemptedRuntimeKey: 'project-a/session-a',
      }),
    ).toBe('none');
  });
});
