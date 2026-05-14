package client

import (
	"context"
	"fmt"
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
