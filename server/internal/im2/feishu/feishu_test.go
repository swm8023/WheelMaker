package feishu

import (
	"testing"

	"github.com/swm8023/wheelmaker/internal/im2"
)

func TestChannelImplementsIM2Channel(t *testing.T) {
	var _ im2.Channel = New(Config{})
}

func TestChannelIDIsFeishu(t *testing.T) {
	if got := New(Config{}).ID(); got != "feishu" {
		t.Fatalf("ID=%q, want feishu", got)
	}
}
