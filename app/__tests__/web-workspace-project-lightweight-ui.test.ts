import fs from 'fs';
import path from 'path';

function readMain(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
}

function readStyles(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');
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
    if (char === '{') depth += 1;
    if (char === '}') {
      depth -= 1;
      if (depth === 0) return source.slice(bodyStart, index + 1);
    }
  }
  throw new Error(`Unable to extract ${functionName}`);
}

describe('workspace project lightweight UI wiring', () => {
  test('user project switching uses lightweight sync instead of switchProject', () => {
    const main = readMain();
    const syncBody = extractFunctionBody(main, 'syncWorkspaceProject');
    const projectMenuBlock = main.slice(
      main.indexOf('const projectMenu ='),
      main.indexOf('const refreshButtonContent'),
    );

    expect(syncBody).toContain('workspaceController.switchProjectLightweight');
    expect(syncBody).toContain('workspaceStore.rememberGlobalState({');
    expect(syncBody).toContain('selectedProjectId: nextProjectId');
    expect(projectMenuBlock).toContain('syncWorkspaceProject(projectItem.projectId');
    expect(projectMenuBlock).not.toContain('switchProject(projectItem.projectId)');
  });

  test('chat session selection syncs workspace project without waiting for file/git loads', () => {
    const main = readMain();
    const body = extractFunctionBody(main, 'selectProjectChatSession');

    expect(body).toContain('syncWorkspaceProject(targetProjectId');
    expect(body).toContain("reason: 'chat'");
    expect(body).not.toContain('switchProject(');
  });

  test('pc file and git sidebars render the workspace selector above section titles', () => {
    const main = readMain();

    expect(main).toContain('const renderWorkspaceProjectSelector = () =>');
    expect(main).toContain('{isWide ? renderWorkspaceProjectSelector() : null}');
    expect(main).toContain('<div className="workspace-project-label">WORKSPACE</div>');
    expect(main).toContain('workspace-project-menu');
  });

  test('hydrating a new workspace project clears stale directory hashes', () => {
    const main = readMain();
    const start = main.indexOf('const applyHydratedProjectState = (');
    const end = main.indexOf('const togglePinSelectedFile', start);
    expect(start).toBeGreaterThanOrEqual(0);
    expect(end).toBeGreaterThan(start);

    const body = main.slice(start, end);
    expect(body).toContain('dirHashRef.current = {};');
  });

  test('workspace project selector has dedicated compact sidebar styles', () => {
    const styles = readStyles();

    expect(styles).toContain('.workspace-project-selector');
    expect(styles).toContain('.workspace-project-button');
    expect(styles).toContain('.workspace-project-menu');
    expect(styles).toContain('.workspace-project-menu-item.selected');
  });
});
