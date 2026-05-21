import fs from 'fs';
import path from 'path';

describe('local hub read UI settings', () => {
  test('persists local hub read acceleration as default enabled', () => {
    const projectRoot = path.join(__dirname, '..');
    const workspacePersistence = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'), 'utf8');

    expect(workspacePersistence).toContain('localHubReadEnabled: boolean;');
    expect(workspacePersistence).toContain("localHubReadEnabled: 'localHubReadEnabled',");
    expect(workspacePersistence).toContain('localHubReadEnabled: true,');
    expect(workspacePersistence).toContain(
      "localHubReadEnabled: typeof input.localHubReadEnabled === 'boolean' ? input.localHubReadEnabled : base.localHubReadEnabled",
    );
    expect(workspacePersistence).toContain('{k: GLOBAL_KEYS.localHubReadEnabled, v: serialize(next.localHubReadEnabled), updatedAt: now}');
  });

  test('adds settings switch and simple Remote Local hub tags', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('const [localHubReadEnabled, setLocalHubReadEnabled] = useState(');
    expect(mainTsx).toContain('service.setLocalHubReadEnabled(localHubReadEnabled);');
    expect(mainTsx).toContain("workspaceStore.rememberGlobalState({ localHubReadEnabled });");
    expect(mainTsx).toContain('<span>Local Hub Read</span>');
    expect(mainTsx).toContain('checked={localHubReadEnabled}');
    expect(mainTsx).toContain('setLocalHubReadEnabled(event.target.checked)');
    expect(mainTsx).toContain('localHubReadStatuses[hub.hubId] ?? \'Remote\'');
    expect(mainTsx).toContain('className={`chat-hub-read-tag ${readStatus.toLowerCase()}`}');
    expect(mainTsx).toContain('{readStatus}');
    expect(mainTsx).not.toContain('53123');

    expect(stylesCss).toContain('.chat-hub-read-tag {');
    expect(stylesCss).toContain('.chat-hub-read-tag.local {');
    expect(stylesCss).toContain('.chat-hub-read-tag.remote {');
  });
});
