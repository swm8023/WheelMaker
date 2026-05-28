import React from 'react';

export type VoiceRecordingBarProps = {
  elapsedMs: number;
  level?: number;
  status?: 'buffering' | 'recording' | 'finishing';
};

function formatElapsed(elapsedMs: number): string {
  const totalSeconds = Math.max(0, Math.floor(elapsedMs / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, '0')}`;
}

export function VoiceRecordingBar({
  elapsedMs,
  level = 0.35,
  status = 'recording',
}: VoiceRecordingBarProps) {
  const bars = [0.42, 0.72, 1, 0.62, 0.88].map((scale, index) => (
    <span
      key={index}
      style={{'--voice-bar-scale': String(Math.max(0.3, Math.min(1.25, scale + level * 0.35)))} as React.CSSProperties}
    />
  ));

  const statusText = status === 'buffering'
    ? 'Connecting...'
    : status === 'finishing'
      ? 'Finishing...'
      : 'Recording';

  return (
    <div className="voice-recording-bar" role="status" aria-live="polite">
      <span className="voice-recording-dot" aria-hidden="true" />
      <span className="voice-recording-wave" aria-hidden="true">
        {bars}
      </span>
      <span className="voice-recording-text">
        {statusText}
      </span>
      <span className="voice-recording-time">{formatElapsed(elapsedMs)}</span>
    </div>
  );
}
