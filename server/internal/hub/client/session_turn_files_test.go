package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSessionTurnStoreWritesAndReadsTurnsAcrossChunks(t *testing.T) {
	store := newFileSessionTurnStore(t.TempDir())
	ctx := context.Background()

	contents := make([]string, 130)
	for i := range contents {
		contents[i] = fmt.Sprintf(`{"method":"system","param":{"text":"turn-%03d"}}`, i+1)
	}
	latest, err := store.WriteTurns(ctx, "proj1", "sess-1", 1, contents)
	if err != nil {
		t.Fatalf("WriteTurns: %v", err)
	}
	if latest != 130 {
		t.Fatalf("latest = %d, want 130", latest)
	}

	turns, err := store.ReadTurns(ctx, "proj1", "sess-1", 127, 130)
	if err != nil {
		t.Fatalf("ReadTurns: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3", len(turns))
	}
	for i, turn := range turns {
		wantIndex := int64(128 + i)
		if turn.TurnIndex != wantIndex {
			t.Fatalf("turn[%d].TurnIndex = %d, want %d", i, turn.TurnIndex, wantIndex)
		}
		if !turn.Finished {
			t.Fatalf("turn[%d].Finished = false, want true", i)
		}
		wantContent := contents[wantIndex-1]
		if turn.Content != wantContent {
			t.Fatalf("turn[%d].Content = %q, want %q", i, turn.Content, wantContent)
		}
	}
}

func TestFileSessionTurnStoreWritesVersion2FilesWith256Turns(t *testing.T) {
	root := t.TempDir()
	store := newFileSessionTurnStore(root)
	ctx := context.Background()

	contents := make([]string, 257)
	for i := range contents {
		contents[i] = fmt.Sprintf(`{"method":"system","param":{"text":"turn-%03d"}}`, i+1)
	}
	latest, err := store.WriteTurns(ctx, "proj1", "sess-1", 1, contents)
	if err != nil {
		t.Fatalf("WriteTurns: %v", err)
	}
	if latest != 257 {
		t.Fatalf("latest = %d, want 257", latest)
	}

	firstFile := filepath.Join(store.turnDir("proj1", "sess-1"), "t000000.bin")
	firstRaw, err := os.ReadFile(firstFile)
	if err != nil {
		t.Fatalf("ReadFile first turn file: %v", err)
	}
	if version := binary.LittleEndian.Uint16(firstRaw[4:6]); version != 2 {
		t.Fatalf("first turn file version = %d, want 2", version)
	}
	if code := firstRaw[6]; code != 0 {
		t.Fatalf("first turn file chunk size code = %d, want 0", code)
	}
	if reserved := firstRaw[7]; reserved != 0 {
		t.Fatalf("first turn file reserved byte = %d, want 0", reserved)
	}
	if occupied := countOccupiedTurnSlots(t, firstRaw, 256); occupied != 256 {
		t.Fatalf("first turn file occupied slots = %d, want 256", occupied)
	}

	secondFile := filepath.Join(store.turnDir("proj1", "sess-1"), "t000001.bin")
	secondRaw, err := os.ReadFile(secondFile)
	if err != nil {
		t.Fatalf("ReadFile second turn file: %v", err)
	}
	if version := binary.LittleEndian.Uint16(secondRaw[4:6]); version != 2 {
		t.Fatalf("second turn file version = %d, want 2", version)
	}
	if code := secondRaw[6]; code != 0 {
		t.Fatalf("second turn file chunk size code = %d, want 0", code)
	}
	if reserved := secondRaw[7]; reserved != 0 {
		t.Fatalf("second turn file reserved byte = %d, want 0", reserved)
	}
	if occupied := countOccupiedTurnSlots(t, secondRaw, 256); occupied != 1 {
		t.Fatalf("second turn file occupied slots = %d, want 1", occupied)
	}

	turns, err := store.ReadTurns(ctx, "proj1", "sess-1", 254, latest)
	if err != nil {
		t.Fatalf("ReadTurns: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3", len(turns))
	}
	for i, turn := range turns {
		wantIndex := int64(255 + i)
		if turn.TurnIndex != wantIndex {
			t.Fatalf("turn[%d].TurnIndex = %d, want %d", i, turn.TurnIndex, wantIndex)
		}
		if turn.Content != contents[wantIndex-1] {
			t.Fatalf("turn[%d].Content = %q, want %q", i, turn.Content, contents[wantIndex-1])
		}
	}
}

func TestFileSessionTurnStoreRejectsLegacyVersion1Files(t *testing.T) {
	root := t.TempDir()
	store := newFileSessionTurnStore(root)
	ctx := context.Background()

	writeLegacyV1TurnFiles(t, root, "proj1", "sess-1", []string{`{"method":"system","param":{"text":"legacy"}}`})

	if _, err := store.ReadTurns(ctx, "proj1", "sess-1", 0, 1); err == nil {
		t.Fatalf("ReadTurns with legacy v1 file unexpectedly succeeded")
	}
}

func TestFileSessionTurnStoreRejectsSkippedTurnIndex(t *testing.T) {
	store := newFileSessionTurnStore(t.TempDir())
	ctx := context.Background()

	if _, err := store.WriteTurns(ctx, "proj1", "sess-1", 2, []string{`{"method":"system"}`}); err == nil {
		t.Fatalf("WriteTurns with skipped first turn unexpectedly succeeded")
	}
}

func TestFileSessionTurnStorePreservesEmptySemanticTurns(t *testing.T) {
	store := newFileSessionTurnStore(t.TempDir())
	ctx := context.Background()

	contents := []string{
		`{"method":"prompt_request","param":{"contentBlocks":[]}}`,
		`{"method":"agent_message_chunk","param":{"text":""}}`,
		`{"method":"prompt_done","param":{"stopReason":""}}`,
	}
	latest, err := store.WriteTurns(ctx, "proj1", "sess-1", 1, contents)
	if err != nil {
		t.Fatalf("WriteTurns: %v", err)
	}
	if latest != 3 {
		t.Fatalf("latest = %d, want 3", latest)
	}

	turns, err := store.ReadTurns(ctx, "proj1", "sess-1", 0, latest)
	if err != nil {
		t.Fatalf("ReadTurns: %v", err)
	}
	if len(turns) != len(contents) {
		t.Fatalf("turns len = %d, want %d", len(turns), len(contents))
	}
	for i, turn := range turns {
		wantIndex := int64(i + 1)
		if turn.TurnIndex != wantIndex {
			t.Fatalf("turns[%d].TurnIndex = %d, want %d", i, turn.TurnIndex, wantIndex)
		}
		if turn.Content != contents[i] {
			t.Fatalf("turns[%d].Content = %q, want %q", i, turn.Content, contents[i])
		}
	}
}

func TestFileSessionTurnStoreRejectsMissingTurnContent(t *testing.T) {
	store := newFileSessionTurnStore(t.TempDir())
	ctx := context.Background()

	if _, err := store.WriteTurns(ctx, "proj1", "sess-1", 1, []string{""}); err == nil {
		t.Fatalf("WriteTurns with empty content unexpectedly succeeded")
	}
}

func countOccupiedTurnSlots(t *testing.T, raw []byte, capacity int) int {
	t.Helper()
	for slot := 0; slot < capacity; slot++ {
		pos := 8 + slot*8
		if len(raw) < pos+8 {
			t.Fatalf("turn file too short for slot %d", slot)
		}
		offset := binary.LittleEndian.Uint32(raw[pos : pos+4])
		length := binary.LittleEndian.Uint32(raw[pos+4 : pos+8])
		if offset == 0 || length == 0 {
			return slot
		}
	}
	return capacity
}

func writeLegacyV1TurnFiles(t *testing.T, root, projectName, sessionID string, contents []string) {
	t.Helper()
	const legacyTurnsPerFile = 128
	const legacyHeaderSize = 8 + legacyTurnsPerFile*8

	type legacyEntry struct {
		slot    int
		content []byte
	}
	groups := map[int64][]legacyEntry{}
	for i, content := range contents {
		turnIndex := int64(i + 1)
		fileNo := (turnIndex - 1) / legacyTurnsPerFile
		slot := int((turnIndex - 1) % legacyTurnsPerFile)
		groups[fileNo] = append(groups[fileNo], legacyEntry{slot: slot, content: []byte(content)})
	}

	dir := filepath.Join(root, safeHistoryPathPart(projectName), safeHistoryPathPart(sessionID), "turns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll legacy turn dir: %v", err)
	}
	for fileNo, entries := range groups {
		raw := make([]byte, legacyHeaderSize)
		copy(raw[0:4], sessionTurnFileMagic)
		binary.LittleEndian.PutUint16(raw[4:6], 1)
		for _, entry := range entries {
			offset := len(raw)
			raw = append(raw, entry.content...)
			slotPos := 8 + entry.slot*8
			binary.LittleEndian.PutUint32(raw[slotPos:slotPos+4], uint32(offset))
			binary.LittleEndian.PutUint32(raw[slotPos+4:slotPos+8], uint32(len(entry.content)))
		}
		path := filepath.Join(dir, fmt.Sprintf("t%06d.bin", fileNo))
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatalf("WriteFile legacy turn file: %v", err)
		}
	}
}
