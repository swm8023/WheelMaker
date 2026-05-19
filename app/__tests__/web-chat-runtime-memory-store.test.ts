import fs from 'fs';
import path from 'path';

function projectRoot(): string {
  return path.join(__dirname, '..');
}

function readMain(): string {
  return fs.readFileSync(path.join(projectRoot(), 'web', 'src', 'main.tsx'), 'utf8');
}

describe('web chat runtime memory store', () => {
  test('does not keep an active runtime set or evict session message stores', () => {
    const main = readMain();

    expect(main).not.toContain("import {createChatActiveRuntimeSet} from './chat/chatActiveRuntimeSet';");
    expect(main).not.toContain('chatActiveRuntimeSetRef');
    expect(main).not.toContain('capacity: 5');
    expect(main).not.toContain('delete chatTurnStoreRef.current');
    expect(main).not.toContain('delete chatMessageStoreRef.current');
    expect(main).not.toContain('delete chatFinishedCursorRef.current');
  });

  test('keeps every received session message in memory and reconnects known in-memory sessions', () => {
    const main = readMain();

    expect(main).toContain('const runtimeKeysFromChatStores = (): string[] =>');
    expect(main).toContain('Object.keys(chatTurnStoreRef.current)');
    expect(main).toContain('Object.keys(chatMessageStoreRef.current)');
    expect(main).toContain('Object.keys(chatFinishedCursorRef.current)');
    expect(main).toContain('const runtimeKeys = new Set(runtimeKeysFromChatStores());');
    expect(main).not.toContain('if (!isSelectedSession && !chatActiveRuntimeSetRef.current.isActive(runtimeKey))');
    expect(main).toContain('if (!knownSession && !isSelectedSession) {');
    expect(main).toContain('refreshChatProjectSessions(eventProjectId).catch(() => undefined);');
    expect(main).toContain('maybeNotifyChatMessage(message, existingSession, eventProjectId);');
  });
});
