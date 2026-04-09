package client

import (
	"testing"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

func TestNormalizeIMPromptBlocks_PreservesImageAndText(t *testing.T) {
	blocks := normalizeIMPromptBlocks([]acp.ContentBlock{
		{Type: acp.ContentBlockTypeImage, MimeType: "image/png", Data: "aGVsbG8="},
		{Type: acp.ContentBlockTypeText, Text: "  hello  "},
	})
	if len(blocks) != 2 {
		t.Fatalf("blocks=%+v, want 2", blocks)
	}
	if blocks[0].Type != acp.ContentBlockTypeImage {
		t.Fatalf("first block type=%q, want image", blocks[0].Type)
	}
	if blocks[1].Type != acp.ContentBlockTypeText || blocks[1].Text != "hello" {
		t.Fatalf("second block=%+v", blocks[1])
	}
}
