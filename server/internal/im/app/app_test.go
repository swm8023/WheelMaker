package app

import (
	"testing"

	"github.com/swm8023/wheelmaker/internal/im"
)

func TestChannelImplementsIMChannel(t *testing.T) {
	var _ im.Channel = New()
}

func TestChannelIDIsApp(t *testing.T) {
	if got := New().ID(); got != "app" {
		t.Fatalf("ID=%q, want app", got)
	}
}
