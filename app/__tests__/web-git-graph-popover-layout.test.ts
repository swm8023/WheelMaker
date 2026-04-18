import fs from 'fs';
import path from 'path';

describe('web git graph popover layout', () => {
  test('uses centered/stretched graph axis and responsive desktop/mobile popover policy', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('if (isWide) {');
    expect(mainTsx).toContain('Math.max(320, Math.round(window.innerWidth * 0.42))');
    expect(mainTsx).toContain('Math.round(window.innerWidth * 0.92)');
    expect(mainTsx).toContain(").closest('.list');");
    expect(mainTsx).toContain('const panelMidY = panelRect');
    expect(mainTsx).toContain('const topZoneY = panelRect');
    expect(mainTsx).toContain('const bottomZoneY = panelRect');
    expect(mainTsx).toContain('y = preferBelow ? bottomZoneY : topZoneY;');

    expect(stylesCss).toMatch(/\.git-worktree-row\s*\{\s*margin-top:\s*0;/);
    expect(stylesCss).toMatch(/\.git-commit-row\s*\{\s*margin-top:\s*0;/);
    expect(stylesCss).toContain('border-left-color: transparent;');
    expect(stylesCss).toContain('padding-left: 10px;');
    expect(stylesCss).toContain('align-self: stretch;');
    expect(stylesCss).toContain('--git-graph-axis: 6px;');
    expect(stylesCss).toContain('left: var(--git-graph-axis);');
    expect(stylesCss).toContain('transform: translate(-50%, -50%);');
    expect(stylesCss).toMatch(/\.git-graph-line\s*\{[^}]*top:\s*-1px;[^}]*bottom:\s*-1px;[^}]*left:\s*var\(--git-graph-axis\);/);
  });
});

