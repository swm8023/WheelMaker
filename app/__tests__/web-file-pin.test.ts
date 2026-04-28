import fs from 'fs';
import path from 'path';

describe('web file pin feature', () => {
  test('supports pinning files and switching via pinned list', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const [pinnedFiles, setPinnedFiles] = useState<string[]>([])');
    expect(mainTsx).toContain('const hasPinnedFiles = pinnedFiles.length > 0;');
    expect(mainTsx).toMatch(/const isSelectedFilePinned = selectedFile\s*\? pinnedFiles\.includes\(selectedFile\)\s*:\s*false;/);
    expect(mainTsx).toMatch(/setPinnedFiles\(prev =>\s*prev\.includes\(selectedFile\)\s*\? prev\.filter\(path => path !== selectedFile\)\s*:\s*\[\.\.\.prev, selectedFile\],\s*\);/);
    expect(mainTsx).toContain('setPinnedFiles([]);');
    expect(mainTsx).toContain('file-side-actions');
    expect(mainTsx).toContain('file-pin-floating');
    expect(mainTsx).toContain('<span className="pinned-label">Pinned</span>');
    expect(mainTsx).toContain('{hasPinnedFiles ? (');
    expect(mainTsx).toMatch(/className=\{`pinned-pin-toggle file-pin-floating \$\{\s*isSelectedFilePinned \? 'active' : ''\s*\}`\}/);
    expect(mainTsx).toMatch(/className=\{`pinned-entry \$\{\s*selectedFile === path \? 'active' : ''\s*\}`\}/);
    expect(mainTsx).toMatch(/className=\{`file-action-panel side-action-panel \$\{\s*gotoToolsOpen \? 'open' : ''\s*\}`\}/);
    expect(mainTsx).toMatch(/className=\{`file-action-panel side-action-panel \$\{\s*searchToolsOpen \? 'open' : ''\s*\}`\}/);
    expect(mainTsx).toContain('codicon-word-wrap view-tool-icon');
    expect(mainTsx).toContain('codicon-list-ordered view-tool-icon');
    expect(mainTsx).toContain('search-input');
    expect(mainTsx).toContain('search-nav');
    expect(mainTsx).toContain('goto-input');
    expect(mainTsx).toContain('goto-trigger');
  });
});
