import fs from 'fs';
import path from 'path';

function readMain(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
}

function extractFunctionBody(source: string, functionName: string): string {
  const marker = `const ${functionName} = async`;
  const start = source.indexOf(marker);
  expect(start).toBeGreaterThanOrEqual(0);
  const arrowStart = source.indexOf(') => {', start);
  expect(arrowStart).toBeGreaterThanOrEqual(0);
  const bodyStart = source.indexOf('{', arrowStart);
  let depth = 0;
  for (let index = bodyStart; index < source.length; index += 1) {
    const char = source[index];
    if (char === '{') {
      depth += 1;
    }
    if (char === '}') {
      depth -= 1;
      if (depth === 0) {
        return source.slice(bodyStart, index + 1);
      }
    }
  }
  throw new Error(`Unable to extract ${functionName}`);
}

describe('main chat composite key migration', () => {
  test('selected chat state uses project-scoped keys and does not switch workspace project', () => {
    const main = readMain();
    const selectProjectChatSessionBody = extractFunctionBody(main, 'selectProjectChatSession');

    expect(main).toContain("from './chat/chatSessionKey'");
    expect(main).toContain('const selectedChatKeyRef = useRef<ChatSessionKey | null>(null);');
    expect(main).toContain('const [selectedChatKey, setSelectedChatKey] = useState<ChatSessionKey | null>(null);');
    expect(main).toContain('encodeChatSessionKey(');
    expect(selectProjectChatSessionBody).not.toContain('switchProject(');
    expect(selectProjectChatSessionBody).toContain('workspaceStore.rememberSelectedChatSessionKey');
  });

  test('session read and send use the selected session project id', () => {
    const main = readMain();

    expect(main).toContain('service.readProjectSession(');
    expect(main).toContain('service.sendProjectSessionMessage(');
    expect(main).toContain('service.setProjectSessionConfig(');
    expect(main).not.toContain('service.readSession(\n        sessionId,');
    expect(main).not.toContain('service.sendSessionMessage({');
  });
});
