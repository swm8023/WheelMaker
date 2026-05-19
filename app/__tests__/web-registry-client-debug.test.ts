import fs from 'fs';
import path from 'path';

describe('web registry client debug hooks', () => {
  const projectRoot = path.join(__dirname, '..');
  const clientPath = path.join(projectRoot, 'web', 'src', 'services', 'registryClient.ts');

  test('keeps debug capture at the registry websocket boundary', () => {
    const clientTs = fs.readFileSync(clientPath, 'utf8');

    expect(clientTs).toContain("import type {RegistryDebugCaptureEvent} from '../debug/registryDebug';");
    expect(clientTs).toContain('export type RegistryDebugSink = (event: RegistryDebugCaptureEvent) => void;');
    expect(clientTs).toContain('constructor(private readonly timeoutMs = 8000, private readonly debugSink?: RegistryDebugSink) {}');
    expect(clientTs).toContain('private emitDebug(event: RegistryDebugCaptureEvent): void {');
  });

  test('records outbound wire json before websocket send', () => {
    const clientTs = fs.readFileSync(clientPath, 'utf8');

    expect(clientTs).toContain('const raw = JSON.stringify(envelope);');
    const emitIndex = clientTs.indexOf("this.emitDebug({kind: 'outbound', envelope, raw});");
    const sendIndex = clientTs.indexOf('this.ws?.send(raw);');
    expect(emitIndex).toBeGreaterThanOrEqual(0);
    expect(sendIndex).toBeGreaterThan(emitIndex);
    expect(clientTs).not.toContain('this.ws?.send(JSON.stringify(envelope));');
  });

  test('records inbound raw messages, parse failures, and lifecycle events', () => {
    const clientTs = fs.readFileSync(clientPath, 'utf8');

    expect(clientTs).toContain("this.emitDebug({kind: 'parse_error', raw: event.data, error:");
    expect(clientTs).toContain("this.emitDebug({kind: 'inbound', envelope, raw: event.data});");
    expect(clientTs).toContain("phase: 'connect_start'");
    expect(clientTs).toContain("phase: 'connect_open'");
    expect(clientTs).toContain("phase: 'connect_close'");
    expect(clientTs).toContain("phase: 'connect_error'");
  });
});
