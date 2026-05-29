import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import { VoiceRecordingBar } from '../web/src/features/speech/VoiceRecordingBar';

function renderText(
  status:
    | 'permission'
    | 'starting'
    | 'buffering'
    | 'recording'
    | 'finishing'
    | 'recognizing',
) {
  let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
  ReactTestRenderer.act(() => {
    renderer = ReactTestRenderer.create(
      <VoiceRecordingBar
        status={status}
        cancelIntent={false}
        elapsedMs={1200}
        level={0.2}
      />,
    );
  });
  return JSON.stringify(renderer!.toJSON());
}

describe('VoiceRecordingBar', () => {
  test('renders voice input transfer states', () => {
    expect(renderText('permission')).toContain('Waiting for microphone...');
    expect(renderText('starting')).toContain('Starting...');
    expect(renderText('buffering')).toContain('Connecting...');
    expect(renderText('recording')).toContain('Recording');
    expect(renderText('finishing')).toContain('Finishing...');
    expect(renderText('recognizing')).toContain('Recognizing...');
  });

  test('cancel intent overrides transfer state copy', () => {
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
    ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceRecordingBar
          status="buffering"
          cancelIntent={true}
          elapsedMs={1200}
        />,
      );
    });

    expect(JSON.stringify(renderer!.toJSON())).toContain('Release to cancel');
  });
});
