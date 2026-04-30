import fs from 'fs';
import path from 'path';

describe('web file open race guard', () => {
  test('drops stale file read responses and reloads on project switch', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const fileReadSeqRef = useRef(0);');
    expect(mainTsx).toContain('const requestSeq = fileReadSeqRef.current + 1;');
    expect(mainTsx).toContain('fileReadSeqRef.current = requestSeq;');
    expect(mainTsx).toContain('if (requestSeq !== fileReadSeqRef.current) return;');
    expect(mainTsx).toContain('fileReadSeqRef.current += 1;');
    expect(mainTsx).toContain('}, [projectId, selectedFile]);');
  });
});
