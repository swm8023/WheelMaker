package client

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

func (s *Session) promptBlocksForAgent(blocks []acp.ContentBlock) ([]acp.ContentBlock, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	s.mu.Lock()
	agentType := s.agentType
	caps := s.agentState.AgentCapabilities
	s.mu.Unlock()

	if strings.EqualFold(agentType, string(acp.ACPProviderCodex)) {
		return blocks, nil
	}
	if caps.PromptCapabilities == nil || !caps.PromptCapabilities.Image {
		return blocks, nil
	}

	out := make([]acp.ContentBlock, 0, len(blocks))
	changed := false
	for _, block := range blocks {
		imageBlock, ok, err := resourceLinkToACPImageBlock(block)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, imageBlock)
			changed = true
			continue
		}
		out = append(out, block)
	}
	if !changed {
		return blocks, nil
	}
	return out, nil
}

func resourceLinkToACPImageBlock(block acp.ContentBlock) (acp.ContentBlock, bool, error) {
	if block.Type != acp.ContentBlockTypeResourceLink {
		return acp.ContentBlock{}, false, nil
	}
	mimeType, ok := promptImageMimeType(block.MimeType, block.Name, block.URI)
	if !ok {
		return acp.ContentBlock{}, false, nil
	}
	parsed, err := url.Parse(strings.TrimSpace(block.URI))
	if err != nil || !strings.EqualFold(parsed.Scheme, "file") {
		return acp.ContentBlock{}, false, nil
	}
	path := attachmentFileURIPath(parsed)
	if path == "" || !filepath.IsAbs(path) {
		return acp.ContentBlock{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return acp.ContentBlock{}, false, fmt.Errorf("read image attachment %q: %w", promptAttachmentLabel(block.Name, path), err)
	}
	if len(data) == 0 {
		return acp.ContentBlock{}, false, fmt.Errorf("image attachment %q is empty", promptAttachmentLabel(block.Name, path))
	}
	if len(data) > attachmentMaxBytes {
		return acp.ContentBlock{}, false, fmt.Errorf("image attachment %q exceeds %d bytes", promptAttachmentLabel(block.Name, path), attachmentMaxBytes)
	}
	return acp.ContentBlock{
		Type:     acp.ContentBlockTypeImage,
		MimeType: mimeType,
		Data:     base64.StdEncoding.EncodeToString(data),
	}, true, nil
}

func promptImageMimeType(mimeType, name, uri string) (string, bool) {
	if mimeType != "" {
		return supportedPromptImageMimeType(mimeType)
	}
	for _, value := range []string{name, uri} {
		if imageMimeType, ok := promptImageMimeTypeFromExtension(value); ok {
			return imageMimeType, true
		}
	}
	return "", false
}

func supportedPromptImageMimeType(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image/png":
		return "image/png", true
	case "image/jpeg", "image/jpg":
		return "image/jpeg", true
	case "image/webp":
		return "image/webp", true
	case "image/gif":
		return "image/gif", true
	default:
		return "", false
	}
}

func promptImageMimeTypeFromExtension(value string) (string, bool) {
	switch strings.ToLower(filepath.Ext(value)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".webp":
		return "image/webp", true
	case ".gif":
		return "image/gif", true
	default:
		return "", false
	}
}

func promptAttachmentLabel(name, path string) string {
	if name != "" {
		return name
	}
	return path
}
