import fs from 'fs';
import path from 'path';

describe('web voice input local buffering wiring', () => {
  test('starts local voice capture before registry stream is ready and flushes through a serial queue', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain("import {createVoiceInputBuffer");
    expect(mainTsx).toContain("from './features/speech/voiceInputBuffer';");
    expect(mainTsx).toContain("import {createVoiceInputSendQueue");
    expect(mainTsx).toContain("from './features/speech/voiceInputSendQueue';");
    expect(mainTsx).toContain('const VOICE_INPUT_FINISH_WAIT_MS = 3000;');
    expect(mainTsx).toContain('const voiceInputReconnectAvailable = () => (');
    expect(mainTsx).toContain("logVoiceInputState('warn', 'start_buffering_disconnected'");
    expect(mainTsx).toContain('voiceInputBufferRef.current = createVoiceInputBuffer({');
    expect(mainTsx).toContain('voiceSendQueueRef.current = createVoiceInputSendQueue({');
    expect(mainTsx).toContain('voiceSendQueueRef.current.enqueue(chunk.bytes)');
    expect(mainTsx).toContain('await queue.drain();');
    expect(mainTsx).toContain('status={voiceRecordingStatus}');
  });
});
