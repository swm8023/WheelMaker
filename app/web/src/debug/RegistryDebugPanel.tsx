import React from 'react';
import {Virtuoso, type VirtuosoHandle} from 'react-virtuoso';
import {
  filterRegistryDebugRecords,
  type RegistryDebugRecord,
} from './registryDebug';

type RegistryDebugPanelFrame = {
  left: number;
  top: number;
  width: number;
  height: number;
};

type RegistryDebugPanelInteraction =
  | {
      kind: 'drag';
      pointerId: number;
      originX: number;
      originY: number;
      startFrame: RegistryDebugPanelFrame;
    }
  | {
      kind: 'resize';
      pointerId: number;
      originX: number;
      originY: number;
      startFrame: RegistryDebugPanelFrame;
    };

export type RegistryDebugPanelProps = {
  records: RegistryDebugRecord[];
  selectedRecordId: number | null;
  onSelectedRecordIdChange: (recordId: number | null) => void;
  selectedSessionId: string;
  onSelectedSessionIdChange: (sessionId: string) => void;
  includeMultiSessionRecords: boolean;
  onIncludeMultiSessionRecordsChange: (include: boolean) => void;
  onClear: () => void;
  onClose: () => void;
};

const PANEL_MIN_WIDTH = 620;
const PANEL_MIN_HEIGHT = 360;
const PANEL_MARGIN = 16;

function defaultPanelFrame(): RegistryDebugPanelFrame {
  const viewportWidth = window.innerWidth || 1280;
  const viewportHeight = window.innerHeight || 800;
  const width = Math.min(980, Math.max(PANEL_MIN_WIDTH, viewportWidth - 96));
  const height = Math.min(640, Math.max(PANEL_MIN_HEIGHT, viewportHeight - 112));
  return {
    left: Math.max(PANEL_MARGIN, viewportWidth - width - 32),
    top: Math.max(PANEL_MARGIN, 72),
    width,
    height,
  };
}

function clampPanelFrame(frame: RegistryDebugPanelFrame): RegistryDebugPanelFrame {
  const viewportWidth = window.innerWidth || frame.width + PANEL_MARGIN * 2;
  const viewportHeight = window.innerHeight || frame.height + PANEL_MARGIN * 2;
  const width = Math.min(
    Math.max(PANEL_MIN_WIDTH, Math.round(frame.width)),
    Math.max(PANEL_MIN_WIDTH, viewportWidth - PANEL_MARGIN * 2),
  );
  const height = Math.min(
    Math.max(PANEL_MIN_HEIGHT, Math.round(frame.height)),
    Math.max(PANEL_MIN_HEIGHT, viewportHeight - PANEL_MARGIN * 2),
  );
  return {
    left: Math.min(
      Math.max(PANEL_MARGIN, Math.round(frame.left)),
      Math.max(PANEL_MARGIN, viewportWidth - width - PANEL_MARGIN),
    ),
    top: Math.min(
      Math.max(PANEL_MARGIN, Math.round(frame.top)),
      Math.max(PANEL_MARGIN, viewportHeight - height - PANEL_MARGIN),
    ),
    width,
    height,
  };
}

function recordLabel(record: RegistryDebugRecord): string {
  const method = record.method || record.phase;
  const request = typeof record.requestId === 'number' ? `#${record.requestId}` : '';
  return [record.timeText, record.direction, method, request].filter(Boolean).join(' ');
}

function selectedRecordValue(record: RegistryDebugRecord | undefined): unknown {
  if (!record) {
    return null;
  }
  if (record.envelope) {
    return record.envelope;
  }
  if (record.lifecycle) {
    return record.lifecycle;
  }
  return {
    phase: record.phase,
    error: record.parseError,
    raw: record.raw,
  };
}

export function RegistryDebugPanel({
  records,
  selectedRecordId,
  onSelectedRecordIdChange,
  selectedSessionId,
  onSelectedSessionIdChange,
  includeMultiSessionRecords,
  onIncludeMultiSessionRecordsChange,
  onClear,
  onClose,
}: RegistryDebugPanelProps) {
  const virtuosoRef = React.useRef<VirtuosoHandle | null>(null);
  const interactionRef = React.useRef<RegistryDebugPanelInteraction | null>(null);
  const [panelFrame, setPanelFrame] = React.useState<RegistryDebugPanelFrame>(() => defaultPanelFrame());
  const [atBottom, setAtBottom] = React.useState(true);

  const sessionIds = React.useMemo(
    () => Array.from(new Set(records.flatMap(record => record.sessionIds))).sort(),
    [records],
  );
  const filteredRecords = React.useMemo(
    () => filterRegistryDebugRecords(records, selectedSessionId, includeMultiSessionRecords),
    [includeMultiSessionRecords, records, selectedSessionId],
  );
  const selectedRecord = React.useMemo(
    () => filteredRecords.find(record => record.id === selectedRecordId),
    [filteredRecords, selectedRecordId],
  );
  const selectedEnvelopeOrLifecycle = React.useMemo(
    () => selectedRecordValue(selectedRecord),
    [selectedRecord],
  );
  const selectedJson = React.useMemo(
    () => JSON.stringify(selectedEnvelopeOrLifecycle, null, 2),
    [selectedEnvelopeOrLifecycle],
  );

  React.useEffect(() => {
    if (selectedSessionId === 'All' || sessionIds.includes(selectedSessionId)) {
      return;
    }
    onSelectedSessionIdChange('All');
  }, [onSelectedSessionIdChange, selectedSessionId, sessionIds]);

  React.useEffect(() => {
    if (filteredRecords.length === 0) {
      if (selectedRecordId !== null) {
        onSelectedRecordIdChange(null);
      }
      return;
    }
    if (selectedRecordId !== null && filteredRecords.some(record => record.id === selectedRecordId)) {
      return;
    }
    onSelectedRecordIdChange(filteredRecords[filteredRecords.length - 1].id);
  }, [filteredRecords, onSelectedRecordIdChange, selectedRecordId]);

  const beginDrag = React.useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      event.preventDefault();
      interactionRef.current = {
        kind: 'drag',
        pointerId: event.pointerId,
        originX: event.clientX,
        originY: event.clientY,
        startFrame: panelFrame,
      };
      event.currentTarget.setPointerCapture(event.pointerId);
    },
    [panelFrame],
  );

  const beginResize = React.useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      event.preventDefault();
      event.stopPropagation();
      interactionRef.current = {
        kind: 'resize',
        pointerId: event.pointerId,
        originX: event.clientX,
        originY: event.clientY,
        startFrame: panelFrame,
      };
      event.currentTarget.setPointerCapture(event.pointerId);
    },
    [panelFrame],
  );

  const movePanelPointer = React.useCallback((event: React.PointerEvent<HTMLElement>) => {
    const interaction = interactionRef.current;
    if (!interaction || interaction.pointerId !== event.pointerId) {
      return;
    }
    event.preventDefault();
    const deltaX = event.clientX - interaction.originX;
    const deltaY = event.clientY - interaction.originY;
    if (interaction.kind === 'drag') {
      setPanelFrame(clampPanelFrame({
        ...interaction.startFrame,
        left: interaction.startFrame.left + deltaX,
        top: interaction.startFrame.top + deltaY,
      }));
      return;
    }
    setPanelFrame(clampPanelFrame({
      ...interaction.startFrame,
      width: interaction.startFrame.width + deltaX,
      height: interaction.startFrame.height + deltaY,
    }));
  }, []);

  const finishPanelPointer = React.useCallback((event: React.PointerEvent<HTMLElement>) => {
    const interaction = interactionRef.current;
    if (!interaction || interaction.pointerId !== event.pointerId) {
      return;
    }
    event.preventDefault();
    interactionRef.current = null;
    try {
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
    } catch {
      // Pointer capture may already be released by the browser.
    }
  }, []);

  const jumpToLatest = React.useCallback(() => {
    if (filteredRecords.length === 0) {
      return;
    }
    virtuosoRef.current?.scrollToIndex({
      index: 'LAST',
      align: 'end',
      behavior: 'auto',
    });
    setAtBottom(true);
  }, [filteredRecords.length]);

  const copySelectedJson = React.useCallback(() => {
    if (!selectedJson || selectedJson === 'null') {
      return;
    }
    navigator.clipboard?.writeText(selectedJson).catch(() => undefined);
  }, [selectedJson]);

  return (
    <div
      className="registry-debug-panel"
      style={{
        left: panelFrame.left,
        top: panelFrame.top,
        width: panelFrame.width,
        height: panelFrame.height,
      }}
      role="dialog"
      aria-label="Registry debug"
    >
      <div
        className="registry-debug-panel-header"
        onPointerDown={beginDrag}
        onPointerMove={movePanelPointer}
        onPointerUp={finishPanelPointer}
        onPointerCancel={finishPanelPointer}
      >
        <div className="registry-debug-title">
          <span className="codicon codicon-debug-alt" />
          <span>Registry Debug</span>
          <span className="registry-debug-count">{records.length}</span>
        </div>
        <div className="registry-debug-actions">
          <button type="button" className="registry-debug-action" onClick={onClear}>
            Clear
          </button>
          <button type="button" className="registry-debug-icon-action" onClick={onClose} aria-label="Close debug panel" title="Close">
            <span className="codicon codicon-close" />
          </button>
        </div>
      </div>
      <div className="registry-debug-toolbar">
        <label className="registry-debug-select-label">
          <span>Session</span>
          <select
            className="registry-debug-select"
            value={selectedSessionId}
            onChange={event => onSelectedSessionIdChange(event.target.value)}
          >
            <option value="All">All</option>
            {sessionIds.map(sessionId => (
              <option key={sessionId} value={sessionId}>{sessionId}</option>
            ))}
          </select>
        </label>
        <label className="registry-debug-check">
          <input
            type="checkbox"
            checked={includeMultiSessionRecords}
            onChange={event => onIncludeMultiSessionRecordsChange(event.target.checked)}
          />
          <span>Include multi-session records</span>
        </label>
      </div>
      <div className="registry-debug-body">
        <div className="registry-debug-list-pane">
          <div className="registry-debug-list-header">
            <span>Time</span>
            <span>Dir</span>
            <span>Phase</span>
            <span>Method</span>
            <span>Req</span>
            <span>Project</span>
            <span>Session</span>
          </div>
          <div className="registry-debug-list-body">
            <Virtuoso<RegistryDebugRecord>
              ref={virtuosoRef}
              data={filteredRecords}
              computeItemKey={(index, record) => record.id}
              followOutput={() => (atBottom ? 'auto' : false)}
              atBottomStateChange={setAtBottom}
              itemContent={(index, record) => (
                <button
                  type="button"
                  className={`registry-debug-row${record.id === selectedRecordId ? ' selected' : ''}`}
                  title={recordLabel(record)}
                  onClick={() => onSelectedRecordIdChange(record.id)}
                >
                  <span className="registry-debug-cell time">{record.timeText}</span>
                  <span className={`registry-debug-cell direction ${record.direction}`}>{record.direction}</span>
                  <span className="registry-debug-cell phase">{record.phase}</span>
                  <span className="registry-debug-cell method">{record.method || '-'}</span>
                  <span className="registry-debug-cell request">{typeof record.requestId === 'number' ? record.requestId : '-'}</span>
                  <span className="registry-debug-cell project">{record.projectId || '-'}</span>
                  <span className="registry-debug-cell session">{record.sessionIds.join(', ') || '-'}</span>
                </button>
              )}
            />
          </div>
          {!atBottom && filteredRecords.length > 0 ? (
            <button type="button" className="registry-debug-jump" onClick={jumpToLatest}>
              Jump to latest
            </button>
          ) : null}
        </div>
        <div className="registry-debug-detail-pane">
          <div className="registry-debug-detail-header">
            <span>{selectedRecord ? recordLabel(selectedRecord) : 'No record selected'}</span>
            <button
              type="button"
              className="registry-debug-action"
              onClick={copySelectedJson}
              disabled={!selectedRecord}
            >
              Copy
            </button>
          </div>
          {selectedRecord ? (
            <pre className="registry-debug-json">{selectedJson}</pre>
          ) : (
            <div className="registry-debug-empty">No record selected.</div>
          )}
        </div>
      </div>
      <button
        type="button"
        className="registry-debug-resize-handle"
        aria-label="Resize debug panel"
        title="Resize"
        onPointerDown={beginResize}
        onPointerMove={movePanelPointer}
        onPointerUp={finishPanelPointer}
        onPointerCancel={finishPanelPointer}
      />
    </div>
  );
}
