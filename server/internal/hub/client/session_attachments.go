package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

const (
	attachmentChunkSize = 1024 * 1024
	attachmentMaxBytes  = 50 * 1024 * 1024
	attachmentIdleTTL   = 3 * time.Minute
)

var attachmentNow = time.Now

type attachmentManager struct {
	mu      sync.Mutex
	uploads map[string]*attachmentUpload
}

type attachmentUpload struct {
	UploadID    string
	ProjectName string
	SessionID   string
	Name        string
	MimeType    string
	Size        int64
	Received    int64
	Root        string
	PartPath    string
	CreatedAt   time.Time
	TouchedAt   time.Time
}

type attachmentSidecar struct {
	AttachmentID string    `json:"attachmentId"`
	ProjectName  string    `json:"projectName"`
	SessionID    string    `json:"sessionId"`
	Name         string    `json:"name"`
	MimeType     string    `json:"mimeType,omitempty"`
	Size         int64     `json:"size"`
	SHA256       string    `json:"sha256"`
	FileName     string    `json:"fileName"`
	URI          string    `json:"uri"`
	CreatedAt    time.Time `json:"createdAt"`
	Sent         bool      `json:"sent,omitempty"`
}

type attachmentRef struct {
	sidecarPath string
	filePath    string
}

func newAttachmentManager() *attachmentManager {
	return &attachmentManager{uploads: map[string]*attachmentUpload{}}
}

func (c *Client) handleSessionAttachmentStart(ctx context.Context, payload json.RawMessage) (any, error) {
	var req struct {
		SessionID string `json:"sessionId"`
		Name      string `json:"name"`
		MimeType  string `json:"mimeType,omitempty"`
		Size      int64  `json:"size"`
	}
	if err := decodeSessionRequestPayload(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid session.attachment.start payload: %w", err)
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	if _, err := c.SessionByID(ctx, sessionID); err != nil {
		return nil, err
	}
	root, err := c.sessionAttachmentRoot(sessionID)
	if err != nil {
		return nil, err
	}
	upload, err := c.attachmentManager().start(c.projectName, sessionID, root, req.Name, req.MimeType, req.Size)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":        true,
		"sessionId": sessionID,
		"uploadId":  upload.UploadID,
		"chunkSize": attachmentChunkSize,
		"expiresIn": int(attachmentIdleTTL / time.Second),
	}, nil
}

func (c *Client) handleSessionAttachmentChunk(ctx context.Context, payload json.RawMessage) (any, error) {
	var req struct {
		SessionID string `json:"sessionId"`
		UploadID  string `json:"uploadId"`
		Offset    int64  `json:"offset"`
		Data      string `json:"data"`
	}
	if err := decodeSessionRequestPayload(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid session.attachment.chunk payload: %w", err)
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	received, err := c.attachmentManager().appendChunk(sessionID, req.UploadID, req.Offset, req.Data)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "sessionId": sessionID, "uploadId": strings.TrimSpace(req.UploadID), "received": received}, nil
}

func (c *Client) handleSessionAttachmentFinish(ctx context.Context, payload json.RawMessage) (any, error) {
	var req struct {
		SessionID string `json:"sessionId"`
		UploadID  string `json:"uploadId"`
		SHA256    string `json:"sha256"`
	}
	if err := decodeSessionRequestPayload(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid session.attachment.finish payload: %w", err)
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	attachment, block, err := c.attachmentManager().finish(sessionID, req.UploadID, req.SHA256)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "sessionId": sessionID, "attachment": attachment, "block": block}, nil
}

func (c *Client) handleSessionAttachmentCancel(ctx context.Context, payload json.RawMessage) (any, error) {
	var req struct {
		SessionID string `json:"sessionId"`
		UploadID  string `json:"uploadId"`
	}
	if err := decodeSessionRequestPayload(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid session.attachment.cancel payload: %w", err)
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.attachmentManager().cancel(sessionID, req.UploadID); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "sessionId": sessionID, "uploadId": strings.TrimSpace(req.UploadID)}, nil
}

func (c *Client) handleSessionAttachmentDelete(ctx context.Context, payload json.RawMessage) (any, error) {
	var req struct {
		SessionID    string `json:"sessionId"`
		AttachmentID string `json:"attachmentId"`
	}
	if err := decodeSessionRequestPayload(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid session.attachment.delete payload: %w", err)
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := c.sessionAttachmentRoot(sessionID)
	if err != nil {
		return nil, err
	}
	if err := c.attachmentManager().deleteAttachment(root, sessionID, req.AttachmentID); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "sessionId": sessionID, "attachmentId": strings.TrimSpace(req.AttachmentID)}, nil
}

func (c *Client) validateSessionAttachmentBlocks(ctx context.Context, sessionID string, blocks []acp.ContentBlock) ([]attachmentRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(blocks) == 0 {
		return nil, nil
	}
	root, err := c.sessionAttachmentRoot(sessionID)
	if err != nil {
		return nil, err
	}
	var refs []attachmentRef
	for _, block := range blocks {
		if block.Type != acp.ContentBlockTypeImage && block.Type != acp.ContentBlockTypeResourceLink {
			continue
		}
		uri := strings.TrimSpace(block.URI)
		if uri == "" {
			continue
		}
		parsed, err := url.Parse(uri)
		if err != nil || !strings.EqualFold(parsed.Scheme, "file") {
			continue
		}
		ref, err := c.attachmentManager().validateFileBlock(root, c.projectName, sessionID, parsed)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (c *Client) markSessionAttachmentsSent(refs []attachmentRef) error {
	if len(refs) == 0 {
		return nil
	}
	return c.attachmentManager().markSent(refs)
}

func (c *Client) attachmentManager() *attachmentManager {
	if c.attachments != nil {
		return c.attachments
	}
	c.attachments = newAttachmentManager()
	return c.attachments
}

func (c *Client) sessionAttachmentRoot(sessionID string) (string, error) {
	root, err := c.sessionHistoryRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, safeHistoryPathPart(c.projectName), safeHistoryPathPart(sessionID), "attachments"), nil
}

func (c *Client) sessionHistoryRoot() (string, error) {
	if c != nil && c.sessionRecorder != nil && c.sessionRecorder.turnStore != nil {
		if root := strings.TrimSpace(c.sessionRecorder.turnStore.root); root != "" {
			return root, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".wheelmaker", "db", "session"), nil
}

func (m *attachmentManager) start(projectName, sessionID, root, name, mimeType string, size int64) (*attachmentUpload, error) {
	if m == nil {
		return nil, fmt.Errorf("attachment manager is required")
	}
	name = attachmentDisplayName(name)
	mimeType = strings.TrimSpace(mimeType)
	if size < 0 {
		return nil, fmt.Errorf("attachment size must be non-negative")
	}
	if size > attachmentMaxBytes {
		return nil, fmt.Errorf("attachment size %d exceeds %d bytes", size, attachmentMaxBytes)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	now := attachmentNow()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked(now)

	var uploadID string
	var partPath string
	for i := 0; i < 8; i++ {
		uploadID = "upload-" + randomHex(12)
		if _, exists := m.uploads[uploadID]; exists {
			continue
		}
		partPath = filepath.Join(root, "."+uploadID+".part")
		f, err := os.OpenFile(partPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		uploadID = ""
	}
	if uploadID == "" {
		return nil, fmt.Errorf("allocate upload id")
	}
	upload := &attachmentUpload{
		UploadID:    uploadID,
		ProjectName: strings.TrimSpace(projectName),
		SessionID:   strings.TrimSpace(sessionID),
		Name:        name,
		MimeType:    mimeType,
		Size:        size,
		Root:        root,
		PartPath:    partPath,
		CreatedAt:   now,
		TouchedAt:   now,
	}
	m.uploads[uploadID] = upload
	return upload, nil
}

func (m *attachmentManager) appendChunk(sessionID, uploadID string, offset int64, encoded string) (int64, error) {
	if m == nil {
		return 0, fmt.Errorf("attachment manager is required")
	}
	now := attachmentNow()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked(now)
	upload, err := m.uploadLocked(sessionID, uploadID)
	if err != nil {
		return 0, err
	}
	if offset != upload.Received {
		return 0, fmt.Errorf("attachment upload offset mismatch: got %d want %d", offset, upload.Received)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return 0, fmt.Errorf("attachment chunk data must be valid base64: %w", err)
	}
	if len(data) > attachmentChunkSize {
		return 0, fmt.Errorf("attachment chunk exceeds %d bytes", attachmentChunkSize)
	}
	if upload.Received+int64(len(data)) > upload.Size {
		return 0, fmt.Errorf("attachment chunk exceeds declared size")
	}
	f, err := os.OpenFile(upload.PartPath, os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := f.Write(data); err != nil {
		return 0, err
	}
	upload.Received += int64(len(data))
	upload.TouchedAt = now
	return upload.Received, nil
}

func (m *attachmentManager) finish(sessionID, uploadID, expectedSHA string) (map[string]any, acp.ContentBlock, error) {
	if m == nil {
		return nil, acp.ContentBlock{}, fmt.Errorf("attachment manager is required")
	}
	now := attachmentNow()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked(now)
	upload, err := m.uploadLocked(sessionID, uploadID)
	if err != nil {
		return nil, acp.ContentBlock{}, err
	}
	expectedSHA = strings.ToLower(strings.TrimSpace(expectedSHA))
	if len(expectedSHA) != sha256.Size*2 {
		return nil, acp.ContentBlock{}, fmt.Errorf("attachment sha256 is required")
	}
	if upload.Received != upload.Size {
		return nil, acp.ContentBlock{}, fmt.Errorf("attachment upload incomplete: received %d of %d bytes", upload.Received, upload.Size)
	}
	raw, err := os.ReadFile(upload.PartPath)
	if err != nil {
		return nil, acp.ContentBlock{}, err
	}
	sum := sha256.Sum256(raw)
	actualSHA := hex.EncodeToString(sum[:])
	if actualSHA != expectedSHA {
		_ = os.Remove(upload.PartPath)
		delete(m.uploads, upload.UploadID)
		return nil, acp.ContentBlock{}, fmt.Errorf("attachment sha256 mismatch")
	}
	attachmentID := "sha256-" + actualSHA
	ext := attachmentExtension(upload.Name, upload.MimeType)
	fileName := attachmentID + ext
	finalPath := filepath.Join(upload.Root, fileName)
	if _, err := os.Stat(finalPath); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(upload.PartPath, finalPath); err != nil {
			return nil, acp.ContentBlock{}, err
		}
	} else if err != nil {
		return nil, acp.ContentBlock{}, err
	} else if err := os.Remove(upload.PartPath); err != nil {
		return nil, acp.ContentBlock{}, err
	}
	uri, err := fileURI(finalPath)
	if err != nil {
		return nil, acp.ContentBlock{}, err
	}
	sidecar := attachmentSidecar{
		AttachmentID: attachmentID,
		ProjectName:  upload.ProjectName,
		SessionID:    upload.SessionID,
		Name:         upload.Name,
		MimeType:     upload.MimeType,
		Size:         upload.Size,
		SHA256:       actualSHA,
		FileName:     fileName,
		URI:          uri,
		CreatedAt:    now,
	}
	if err := writeAttachmentSidecar(attachmentSidecarPath(finalPath), sidecar); err != nil {
		return nil, acp.ContentBlock{}, err
	}
	delete(m.uploads, upload.UploadID)
	attachment := attachmentView(sidecar)
	block := attachmentContentBlock(sidecar)
	return attachment, block, nil
}

func (m *attachmentManager) cancel(sessionID, uploadID string) error {
	if m == nil {
		return fmt.Errorf("attachment manager is required")
	}
	now := attachmentNow()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked(now)
	upload, err := m.uploadLocked(sessionID, uploadID)
	if err != nil {
		return err
	}
	_ = os.Remove(upload.PartPath)
	delete(m.uploads, upload.UploadID)
	return nil
}

func (m *attachmentManager) deleteAttachment(root, sessionID, attachmentID string) error {
	if m == nil {
		return fmt.Errorf("attachment manager is required")
	}
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID == "" {
		return fmt.Errorf("attachmentId is required")
	}
	if !strings.HasPrefix(attachmentID, "sha256-") || len(attachmentID) != len("sha256-")+sha256.Size*2 {
		return fmt.Errorf("invalid attachmentId")
	}
	sidecarPath := filepath.Join(root, attachmentID+".json")
	sidecar, err := readAttachmentSidecar(sidecarPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(sidecar.SessionID) != strings.TrimSpace(sessionID) {
		return fmt.Errorf("attachment does not belong to session")
	}
	if sidecar.Sent {
		return fmt.Errorf("sent attachment cannot be deleted")
	}
	filePath := filepath.Join(root, sidecar.FileName)
	if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(sidecarPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (m *attachmentManager) validateFileBlock(root, projectName, sessionID string, parsed *url.URL) (attachmentRef, error) {
	path := attachmentFileURIPath(parsed)
	if strings.TrimSpace(path) == "" {
		return attachmentRef{}, fmt.Errorf("attachment file uri has no path")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return attachmentRef{}, err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return attachmentRef{}, err
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return attachmentRef{}, fmt.Errorf("attachment file is outside session attachments")
	}
	sidecarPath := attachmentSidecarPath(pathAbs)
	sidecar, err := readAttachmentSidecar(sidecarPath)
	if err != nil {
		return attachmentRef{}, fmt.Errorf("attachment sidecar: %w", err)
	}
	if strings.TrimSpace(sidecar.ProjectName) != strings.TrimSpace(projectName) || strings.TrimSpace(sidecar.SessionID) != strings.TrimSpace(sessionID) {
		return attachmentRef{}, fmt.Errorf("attachment does not belong to session")
	}
	if filepath.Clean(filepath.Join(rootAbs, sidecar.FileName)) != filepath.Clean(pathAbs) {
		return attachmentRef{}, fmt.Errorf("attachment sidecar path mismatch")
	}
	return attachmentRef{sidecarPath: sidecarPath, filePath: pathAbs}, nil
}

func (m *attachmentManager) markSent(refs []attachmentRef) error {
	for _, ref := range refs {
		sidecar, err := readAttachmentSidecar(ref.sidecarPath)
		if err != nil {
			return err
		}
		sidecar.Sent = true
		if err := writeAttachmentSidecar(ref.sidecarPath, sidecar); err != nil {
			return err
		}
	}
	return nil
}

func (m *attachmentManager) uploadLocked(sessionID, uploadID string) (*attachmentUpload, error) {
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" {
		return nil, fmt.Errorf("uploadId is required")
	}
	upload := m.uploads[uploadID]
	if upload == nil {
		return nil, fmt.Errorf("attachment upload not found or expired")
	}
	if strings.TrimSpace(upload.SessionID) != strings.TrimSpace(sessionID) {
		return nil, fmt.Errorf("attachment upload does not belong to session")
	}
	return upload, nil
}

func (m *attachmentManager) cleanupExpiredLocked(now time.Time) {
	for uploadID, upload := range m.uploads {
		if now.Sub(upload.TouchedAt) <= attachmentIdleTTL {
			continue
		}
		_ = os.Remove(upload.PartPath)
		delete(m.uploads, uploadID)
	}
}

func (m *attachmentManager) uploadPartPathForTest(uploadID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if upload := m.uploads[strings.TrimSpace(uploadID)]; upload != nil {
		return upload.PartPath
	}
	return ""
}

func attachmentView(sidecar attachmentSidecar) map[string]any {
	return map[string]any{
		"id":       sidecar.AttachmentID,
		"name":     sidecar.Name,
		"mimeType": sidecar.MimeType,
		"size":     sidecar.Size,
		"sha256":   sidecar.SHA256,
		"uri":      sidecar.URI,
	}
}

func attachmentContentBlock(sidecar attachmentSidecar) acp.ContentBlock {
	return acp.ContentBlock{
		Type:     acp.ContentBlockTypeResourceLink,
		URI:      sidecar.URI,
		Name:     sidecar.Name,
		MimeType: sidecar.MimeType,
		Size:     int(sidecar.Size),
	}
}

func attachmentDisplayName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == string(os.PathSeparator) || name == "" {
		return "attachment"
	}
	name = strings.NewReplacer("\\", "_", "/", "_").Replace(name)
	return name
}

func attachmentExtension(name, mimeType string) string {
	if ext := safeAttachmentExtension(filepath.Ext(name)); ext != "" {
		return ext
	}
	extensions, err := mime.ExtensionsByType(strings.TrimSpace(mimeType))
	if err == nil && len(extensions) > 0 {
		return safeAttachmentExtension(extensions[0])
	}
	return ""
}

func safeAttachmentExtension(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if len(ext) < 2 || len(ext) > 24 || ext[0] != '.' {
		return ""
	}
	for _, r := range ext[1:] {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return ""
	}
	return ext
}

func attachmentSidecarPath(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + ".json"
}

func writeAttachmentSidecar(path string, sidecar attachmentSidecar) error {
	raw, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func readAttachmentSidecar(path string) (attachmentSidecar, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return attachmentSidecar{}, err
	}
	var sidecar attachmentSidecar
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		return attachmentSidecar{}, err
	}
	return sidecar, nil
}

func fileURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	uriPath := filepath.ToSlash(abs)
	if len(uriPath) >= 2 && uriPath[1] == ':' {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	return u.String(), nil
}

func attachmentFileURIPath(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	path := parsed.Path
	if parsed.Host != "" {
		path = "//" + parsed.Host + path
	}
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

func randomHex(n int) string {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d", attachmentNow().UnixNano())))
		return hex.EncodeToString(sum[:n])
	}
	return hex.EncodeToString(raw)
}
