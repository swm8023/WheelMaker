package app

import (
	"testing"

	"github.com/swm8023/wheelmaker/internal/im2"
)

func TestChannelImplementsIM2Channel(t *testing.T) {
	var _ im2.Channel = New()
}

func TestChannelIDIsApp(t *testing.T) {
	if got := New().ID(); got != "app" {
		t.Fatalf("ID=%q, want app", got)
	}
}
