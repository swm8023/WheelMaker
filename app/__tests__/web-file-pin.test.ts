import fs from 'fs';
import path from 'path';

describe('web file pin feature', () => {
  test('supports pinning files and switching via pinned list', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const [pinnedFiles, setPinnedFiles] = useState<string[]>([])');
    expect(mainTsx).toContain('const isSelectedFilePinned = selectedFile ? pinnedFiles.includes(selectedFile) : false;');
    expect(mainTsx).toContain('setPinnedFiles(prev => prev.includes(selectedFile) ? prev.filter(path => path !== selectedFile) : [...prev, selectedFile]);');
    expect(mainTsx).toContain('setPinnedFiles([]);');
    expect(mainTsx).toContain('<span className="pinned-label">Pinned</span>');
    expect(mainTsx).toContain("className={`pinned-entry ${selectedFile === path ? 'active' : ''}`}");
    expect(mainTsx).toContain("isSelectedFilePinned ? 'Unpin' : 'Pin'");
  });
});
