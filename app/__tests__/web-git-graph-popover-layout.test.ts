import fs from 'fs';
import path from 'path';

describe('web git graph popover layout', () => {
  test('uses centered graph axis and responsive desktop/mobile popover policy', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('if (isWide) {');
    expect(mainTsx).toContain('Math.max(320, Math.round(window.innerWidth * 0.42))');
    expect(mainTsx).toContain('Math.round(window.innerWidth * 0.92)');
    expect(mainTsx).toContain('const preferBelow = clickMidY <= viewportMidY;');

    expect(stylesCss).toMatch(/\.git-row-spacer\s*\{\s*width:\s*6px;/);
    expect(stylesCss).toMatch(/\.git-tree-child \.git-row-spacer\s*\{\s*width:\s*12px;/);
    expect(stylesCss).toMatch(/\.git-graph-line\s*\{[^}]*left:\s*50%;[^}]*transform:\s*translateX\(-50%\);/);
  });
});

