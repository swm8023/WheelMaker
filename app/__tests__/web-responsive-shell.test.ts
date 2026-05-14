import fs from 'fs';
import path from 'path';
import { transformSync } from '@babel/core';

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
    expect(shellTsx).toMatch(
      /<div\s+className=\{`drawer-overlay \$\{drawerOpen \? 'show' : ''\}`\}\s+onClick=\{onCloseDrawer\}\s+\/>/,
    );
    expect(shellTsx).toMatch(
      /<aside\s+className=\{`drawer \$\{drawerOpen \? 'show' : ''\}`\}[\s\S]*?onClick=\{event => event\.stopPropagation\(\)\}/,
    );
  });

  test('keeps the React runtime import required by the web JSX transform', () => {
    const projectRoot = path.join(__dirname, '..');
    const shellPath = path.join(projectRoot, 'web', 'src', 'shell', 'ResponsiveShell.tsx');
    const shellTsx = fs.readFileSync(shellPath, 'utf8');

    const output =
      transformSync(shellTsx, {
        filename: shellPath,
        babelrc: false,
        configFile: false,
        presets: ['@babel/preset-env', '@babel/preset-react', '@babel/preset-typescript'],
      })?.code ?? '';

    expect(output).toMatch(/\.createElement\(/);
    expect(output).toMatch(/require\(["']react["']\)|from ["']react["']/);
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
