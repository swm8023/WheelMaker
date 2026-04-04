import fs from 'fs';
import path from 'path';

describe('web registry client close policy', () => {
  test('does not treat websocket error event as immediate disconnect', () => {
    const projectRoot = path.join(__dirname, '..');
    const clientTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryClient.ts'), 'utf8');

    expect(clientTs).toContain('ws.onclose = () => this.handleSocketClosed(ws);');
    expect(clientTs).not.toContain('ws.onerror = () => this.handleSocketClosed(ws);');
  });
});
