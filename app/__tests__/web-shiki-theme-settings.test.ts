import fs from 'fs';
import path from 'path';

describe('web shiki theme settings', () => {
  test('uses bundled Shiki theme metadata and grouped theme settings UI', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const shikiRenderer = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'shikiRenderer.ts'), 'utf8');
    const workspacePersistence = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'), 'utf8');
    const shikiThemes = fs.readFileSync(path.join(projectRoot, 'node_modules', 'shiki', 'dist', 'themes.mjs'), 'utf8');

    expect(shikiRenderer).toContain('bundledThemesInfo');
    expect(shikiRenderer).toContain('type BundledTheme');
    expect(shikiRenderer).toContain("export type CodeThemeId = 'auto-plus' | BundledTheme;");
    expect(shikiRenderer).toContain('export const CODE_THEME_OPTION_GROUPS');
    expect(shikiRenderer).toContain('label: \'Dark Themes\'');
    expect(shikiRenderer).toContain('label: \'Light Themes\'');
    expect(shikiRenderer).toContain('appearance: info.type');
    expect(shikiThemes).toContain('"id": "material-theme-darker"');

    expect(mainTsx).toContain('CODE_THEME_OPTION_GROUPS');
    expect(mainTsx).toContain('<optgroup key={group.label} label={group.label}>');
    expect(mainTsx).toContain('CODE_THEME_OPTIONS[0].label');
    expect(mainTsx).toContain('Code Font');
    expect(mainTsx).toContain('Font Size');
    expect(mainTsx).toContain('Line Height');
    expect(mainTsx).toContain('Tab Size');

    expect(workspacePersistence).toContain("isCodeThemeId(input.codeTheme)");
    expect(workspacePersistence).toContain('codeTheme: typeof input.codeTheme === \'string\' && isCodeThemeId(input.codeTheme) ? input.codeTheme : base.codeTheme');
  });
});
