package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	sessionArchiveManifestVersion = 1
	sessionArchivePackFile        = "archive.pack"
	sessionArchiveSegmentMagic    = "WMSA"
	sessionArchiveSegmentVersion  = uint16(1)
	sessionArchiveCodecGzip       = byte(1)
	sessionArchiveMaxChunkCode    = byte(10)
	sessionArchiveGapMethod       = "session/archive_gap"
)

type sessionArchiveStore struct {
	root string
	mu   sync.Mutex
}

type sessionArchiveManifest struct {
	Version   int                                    `json:"version"`
	UpdatedAt string                                 `json:"updatedAt"`
	Sessions  map[string]sessionArchiveManifestEntry `json:"sessions"`
}

type sessionArchiveManifestEntry struct {
	SessionID          string `json:"sessionId"`
	ProjectName        string `json:"projectName"`
	Title              string `json:"title,omitempty"`
	AgentType          string `json:"agentType,omitempty"`
	Storage            string `json:"storage"`
	File               string `json:"file"`
	Offset             int64  `json:"offset"`
	Length             int64  `json:"length"`
	UncompressedLength int64  `json:"uncompressedLength"`
	Codec              string `json:"codec"`
	SHA256             string `json:"sha256"`
	UncompressedSHA256 string `json:"uncompressedSha256"`
	TurnCount          int    `json:"turnCount"`
	GapCount           int    `json:"gapCount"`
	WMT2Version        int    `json:"wmt2Version"`
	ChunkSizeCode      int    `json:"chunkSizeCode"`
	ArchivedAt         string `json:"archivedAt"`
	CreatedAt          string `json:"createdAt,omitempty"`
	UpdatedAt          string `json:"updatedAt,omitempty"`
}

func newSessionArchiveStore(root string) *sessionArchiveStore {
	return &sessionArchiveStore{root: strings.TrimSpace(root)}
}

func (s *sessionArchiveStore) HasSession(ctx context.Context, projectName, sessionID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s == nil || strings.TrimSpace(s.root) == "" {
		return false, fmt.Errorf("session archive store is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, fmt.Errorf("session id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	manifest, err := s.readManifestLocked(projectName)
	if err != nil {
		return false, err
	}
	_, ok := manifest.Sessions[sessionID]
	return ok, nil
}

func (s *sessionArchiveStore) AppendSession(ctx context.Context, rec SessionRecord, contents []string, gapCount int) (sessionArchiveManifestEntry, bool, error) {
	if err := ctx.Err(); err != nil {
		return sessionArchiveManifestEntry{}, false, err
	}
	if s == nil || strings.TrimSpace(s.root) == "" {
		return sessionArchiveManifestEntry{}, false, fmt.Errorf("session archive store is required")
	}
	sessionID := strings.TrimSpace(rec.ID)
	projectName := strings.TrimSpace(rec.ProjectName)
	if sessionID == "" {
		return sessionArchiveManifestEntry{}, false, fmt.Errorf("session id is required")
	}
	if projectName == "" {
		return sessionArchiveManifestEntry{}, false, fmt.Errorf("project name is required")
	}

	wmt2Raw, chunkSizeCode, err := buildArchiveWMT2(contents)
	if err != nil {
		return sessionArchiveManifestEntry{}, false, err
	}
	compressed, err := gzipBytes(wmt2Raw)
	if err != nil {
		return sessionArchiveManifestEntry{}, false, err
	}
	segment, err := buildArchiveSegment(sessionID, compressed, int64(len(wmt2Raw)))
	if err != nil {
		return sessionArchiveManifestEntry{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	manifest, err := s.readManifestLocked(projectName)
	if err != nil {
		return sessionArchiveManifestEntry{}, false, err
	}
	if existing, ok := manifest.Sessions[sessionID]; ok {
		return existing, false, nil
	}

	projectDir := s.projectDir(projectName)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return sessionArchiveManifestEntry{}, false, fmt.Errorf("mkdir archive dir: %w", err)
	}
	packPath := filepath.Join(projectDir, sessionArchivePackFile)
	offset, err := appendArchiveSegment(packPath, segment)
	if err != nil {
		return sessionArchiveManifestEntry{}, false, err
	}

	now := time.Now().UTC()
	entry := sessionArchiveManifestEntry{
		SessionID:          sessionID,
		ProjectName:        projectName,
		Title:              strings.TrimSpace(rec.Title),
		AgentType:          strings.TrimSpace(rec.AgentType),
		Storage:            "pack",
		File:               sessionArchivePackFile,
		Offset:             offset,
		Length:             int64(len(segment)),
		UncompressedLength: int64(len(wmt2Raw)),
		Codec:              "gzip",
		SHA256:             sha256Hex(segment),
		UncompressedSHA256: sha256Hex(wmt2Raw),
		TurnCount:          len(contents),
		GapCount:           gapCount,
		WMT2Version:        int(sessionTurnFileVersion),
		ChunkSizeCode:      int(chunkSizeCode),
		ArchivedAt:         now.Format(time.RFC3339),
		CreatedAt:          formatArchiveTime(rec.CreatedAt),
		UpdatedAt:          formatArchiveTime(rec.LastActiveAt),
	}
	manifest.Version = sessionArchiveManifestVersion
	manifest.UpdatedAt = entry.ArchivedAt
	manifest.Sessions[sessionID] = entry
	if err := s.writeManifestLocked(projectName, manifest); err != nil {
		return sessionArchiveManifestEntry{}, false, err
	}
	return entry, true, nil
}

func (s *sessionArchiveStore) readManifestLocked(projectName string) (sessionArchiveManifest, error) {
	manifest := sessionArchiveManifest{
		Version:  sessionArchiveManifestVersion,
		Sessions: map[string]sessionArchiveManifestEntry{},
	}
	path := s.manifestPath(projectName)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return manifest, nil
	}
	if err != nil {
		return manifest, fmt.Errorf("read archive manifest: %w", err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, fmt.Errorf("decode archive manifest: %w", err)
	}
	if manifest.Version == 0 {
		manifest.Version = sessionArchiveManifestVersion
	}
	if manifest.Sessions == nil {
		manifest.Sessions = map[string]sessionArchiveManifestEntry{}
	}
	return manifest, nil
}

func (s *sessionArchiveStore) writeManifestLocked(projectName string, manifest sessionArchiveManifest) error {
	projectDir := s.projectDir(projectName)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return fmt.Errorf("mkdir archive dir: %w", err)
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode archive manifest: %w", err)
	}
	path := s.manifestPath(projectName)
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open archive manifest temp: %w", err)
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return fmt.Errorf("write archive manifest temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync archive manifest temp: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close archive manifest temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(path)
		if retryErr := os.Rename(tmpPath, path); retryErr != nil {
			return fmt.Errorf("replace archive manifest: %w", err)
		}
	}
	return nil
}

func (s *sessionArchiveStore) manifestPath(projectName string) string {
	return filepath.Join(s.projectDir(projectName), "manifest.json")
}

func (s *sessionArchiveStore) projectDir(projectName string) string {
	return filepath.Join(s.root, safeHistoryPathPart(projectName))
}

func buildArchiveWMT2(contents []string) ([]byte, byte, error) {
	if len(contents) > int(sessionTurnCapacityForCode(sessionArchiveMaxChunkCode)) {
		return nil, 0, fmt.Errorf("session turn count %d exceeds archive capacity", len(contents))
	}
	chunkSizeCode := archiveChunkSizeCode(len(contents))
	capacity := int(sessionTurnCapacityForCode(chunkSizeCode))
	headerSize := sessionTurnFilePreambleSize + capacity*sessionTurnFileMetaSize
	raw := make([]byte, headerSize)
	copy(raw[0:4], sessionTurnFileMagic)
	binary.LittleEndian.PutUint16(raw[4:6], sessionTurnFileVersion)
	raw[6] = chunkSizeCode
	raw[7] = sessionTurnFileReservedByte

	for slot, content := range contents {
		if content == "" {
			return nil, 0, fmt.Errorf("archive turn content is required")
		}
		if len(content) > math.MaxUint32 {
			return nil, 0, fmt.Errorf("archive turn content too large")
		}
		offset := len(raw)
		if offset+len(content) > math.MaxUint32 {
			return nil, 0, fmt.Errorf("archive WMT2 payload too large")
		}
		raw = append(raw, content...)
		slotPos := sessionTurnFilePreambleSize + slot*sessionTurnFileMetaSize
		binary.LittleEndian.PutUint32(raw[slotPos:slotPos+4], uint32(offset))
		binary.LittleEndian.PutUint32(raw[slotPos+4:slotPos+8], uint32(len(content)))
	}
	return raw, chunkSizeCode, nil
}

func archiveChunkSizeCode(turnCount int) byte {
	for code := byte(0); code <= sessionArchiveMaxChunkCode; code++ {
		if turnCount <= int(sessionTurnCapacityForCode(code)) {
			return code
		}
	}
	return sessionArchiveMaxChunkCode
}

func sessionTurnCapacityForCode(code byte) int {
	return sessionTurnsPerFile << code
}

func buildArchiveSegment(sessionID string, compressedPayload []byte, uncompressedLen int64) ([]byte, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if len(sessionID) > math.MaxUint16 {
		return nil, fmt.Errorf("session id too long")
	}
	if len(compressedPayload) > math.MaxInt64 {
		return nil, fmt.Errorf("archive payload too large")
	}
	headerSize := 26 + len(sessionID)
	segment := make([]byte, headerSize)
	copy(segment[0:4], sessionArchiveSegmentMagic)
	binary.LittleEndian.PutUint16(segment[4:6], sessionArchiveSegmentVersion)
	segment[6] = sessionArchiveCodecGzip
	segment[7] = 0
	binary.LittleEndian.PutUint16(segment[8:10], uint16(len(sessionID)))
	binary.LittleEndian.PutUint64(segment[10:18], uint64(len(compressedPayload)))
	binary.LittleEndian.PutUint64(segment[18:26], uint64(uncompressedLen))
	copy(segment[26:headerSize], sessionID)
	segment = append(segment, compressedPayload...)
	return segment, nil
}

func appendArchiveSegment(path string, segment []byte) (int64, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open archive pack: %w", err)
	}
	defer f.Close()
	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("seek archive pack: %w", err)
	}
	if _, err := f.Write(segment); err != nil {
		return 0, fmt.Errorf("write archive segment: %w", err)
	}
	if err := f.Sync(); err != nil {
		return 0, fmt.Errorf("sync archive pack: %w", err)
	}
	return offset, nil
}

func gzipBytes(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("gzip archive payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close gzip archive payload: %w", err)
	}
	return buf.Bytes(), nil
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

func archiveGapTurnJSON() string {
	return buildIMContentJSON(sessionArchiveGapMethod, map[string]string{"reason": "missing_turn"})
}

func formatArchiveTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
