import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import {VoiceRecordingBar} from '../web/src/features/speech/VoiceRecordingBar';

function renderText(status: 'buffering' | 'recording' | 'finishing') {
  let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
  ReactTestRenderer.act(() => {
    renderer = ReactTestRenderer.create(
      <VoiceRecordingBar
        status={status}
        elapsedMs={1200}
        level={0.2}
      />,
    );
  });
  return JSON.stringify(renderer!.toJSON());
}

describe('VoiceRecordingBar', () => {
  test('renders voice input transfer states', () => {
    expect(renderText('buffering')).toContain('Connecting...');
    expect(renderText('recording')).toContain('Recording');
    expect(renderText('finishing')).toContain('Finishing...');
  });

  test('does not render legacy cancel guidance', () => {
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
    ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <VoiceRecordingBar
          status="buffering"
          elapsedMs={1200}
        />,
      );
    });

    const text = JSON.stringify(renderer!.toJSON());
    expect(text).toContain('Connecting...');
    expect(text).not.toContain('Release to cancel');
  });
});
