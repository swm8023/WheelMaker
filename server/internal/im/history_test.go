package im

import (
	"context"
	"testing"
)

func TestMemoryHistoryStore_ListFiltersBySession(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryHistoryStore()
	if err := store.Append(ctx, HistoryEvent{SessionID: "s1", Direction: HistoryInbound, Text: "a"}); err != nil {
		t.Fatalf("Append s1: %v", err)
	}
	if err := store.Append(ctx, HistoryEvent{SessionID: "s2", Direction: HistoryInbound, Text: "b"}); err != nil {
		t.Fatalf("Append s2: %v", err)
	}

	got, err := store.List(ctx, "s1", HistoryQuery{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Text != "a" {
		t.Fatalf("events=%+v", got)
	}
}
