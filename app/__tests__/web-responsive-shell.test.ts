import fs from 'fs';
import path from 'path';

describe('web responsive shell split', () => {
  test('defines desktop and mobile shells behind one responsive shell module', () => {
    const projectRoot = path.join(__dirname, '..');
    const shellPath = path.join(projectRoot, 'web', 'src', 'shell', 'ResponsiveShell.tsx');

    expect(fs.existsSync(shellPath)).toBe(true);

    const shellTsx = fs.readFileSync(shellPath, 'utf8');

    expect(shellTsx).toContain("import type { LayoutMode } from '../services/responsiveLayout';");
    expect(shellTsx).toContain('export function DesktopShell(');
    expect(shellTsx).toContain('export function MobileShell(');
    expect(shellTsx).toContain('export function ResponsiveShell(');
    expect(shellTsx).toContain("mode === 'desktop'");

    expect(shellTsx).toMatch(
      /export function DesktopShell[\s\S]*?className=\{`workspace theme-\$\{themeMode\}`\}[\s\S]*?<aside className="workspace-left">\{sidebar\}<\/aside>/,
    );
    expect(shellTsx).toMatch(
      /export function MobileShell[\s\S]*?className=\{`workspace theme-\$\{themeMode\} narrow-shell`\}[\s\S]*?className=\{`drawer-overlay \$\{drawerOpen \? 'show' : ''\}`\}/,
    );
  });

  test('main delegates shell structure instead of owning desktop and mobile containers inline', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("import { ResponsiveShell } from './shell/ResponsiveShell';");
    expect(mainTsx).toContain('<ResponsiveShell');
    expect(mainTsx).toContain('mode={layoutMode}');
    expect(mainTsx).toContain('desktopHeader={wideHeader}');
    expect(mainTsx).toContain('floatingControlStack={floatingControlStack}');
    expect(mainTsx).toContain('mobileSettingsScreen={mobileSettingsScreen}');
    expect(mainTsx).toContain('sidebar={renderSidebar()}');
    expect(mainTsx).toContain('main={renderMain()}');

    const appReturnStart = mainTsx.indexOf('return (\n    <ResponsiveShell');
    const appReturnEnd = mainTsx.indexOf('\n  );\n}', appReturnStart);
    expect(appReturnStart).toBeGreaterThanOrEqual(0);
    expect(appReturnEnd).toBeGreaterThan(appReturnStart);
    const appReturn = mainTsx.slice(appReturnStart, appReturnEnd);

    expect(appReturn).not.toContain('className="body"');
    expect(appReturn).not.toContain('className={`drawer-overlay');
    expect(appReturn).not.toContain('className="workspace-left"');
  });
});
