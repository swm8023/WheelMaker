package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

const (
	sessionTurnFileMagic    = "WMT2"
	sessionTurnFileVersion  = uint16(1)
	sessionTurnsPerFile     = 128
	sessionTurnFileMetaSize = 8
	sessionTurnFileHeadSize = 4 + 2 + 2 + sessionTurnsPerFile*sessionTurnFileMetaSize
)

type sessionViewTurn struct {
	SessionID string `json:"sessionId"`
	TurnIndex int64  `json:"turnIndex"`
	Content   string `json:"content"`
	Finished  bool   `json:"finished"`
}

type fileSessionTurnStore struct {
	root string
}

func newFileSessionTurnStore(root string) *fileSessionTurnStore {
	return &fileSessionTurnStore{root: root}
}

func ReadSessionTurnFiles(ctx context.Context, root, projectName, sessionID string, afterTurnIndex, latestTurnIndex int64) ([]string, error) {
	store := newFileSessionTurnStore(root)
	turns, err := store.ReadTurns(ctx, projectName, sessionID, afterTurnIndex, latestTurnIndex)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(turns))
	for _, turn := range turns {
		out = append(out, turn.Content)
	}
	return out, nil
}

func WriteSessionTurnFiles(ctx context.Context, root, projectName, sessionID string, startTurnIndex int64, contents []string) (int64, error) {
	return newFileSessionTurnStore(root).WriteTurns(ctx, projectName, sessionID, startTurnIndex, contents)
}

func (s *fileSessionTurnStore) WriteTurns(ctx context.Context, projectName, sessionID string, startTurnIndex int64, contents []string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if s == nil {
		return 0, fmt.Errorf("session turn store is required")
	}
	if startTurnIndex <= 0 {
		return 0, fmt.Errorf("start turn index is required")
	}
	if len(contents) == 0 {
		return startTurnIndex - 1, nil
	}
	latest, err := s.latestTurnIndex(ctx, projectName, sessionID)
	if err != nil {
		return 0, err
	}
	if startTurnIndex != latest+1 {
		return 0, fmt.Errorf("start turn index %d does not follow latest turn index %d", startTurnIndex, latest)
	}
	next := startTurnIndex
	for _, content := range contents {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if content == "" {
			return 0, fmt.Errorf("turn content is required")
		}
		if len(content) > math.MaxUint32 {
			return 0, fmt.Errorf("turn content too large")
		}
		if err := s.writeTurn(ctx, projectName, sessionID, next, []byte(content)); err != nil {
			return 0, err
		}
		next++
	}
	return next - 1, nil
}

func (s *fileSessionTurnStore) ReadTurns(ctx context.Context, projectName, sessionID string, afterTurnIndex, latestTurnIndex int64) ([]sessionViewTurn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("session turn store is required")
	}
	if latestTurnIndex <= afterTurnIndex {
		return nil, nil
	}
	out := make([]sessionViewTurn, 0, latestTurnIndex-afterTurnIndex)
	for turnIndex := afterTurnIndex + 1; turnIndex <= latestTurnIndex; turnIndex++ {
		content, err := s.readTurn(ctx, projectName, sessionID, turnIndex)
		if err != nil {
			return nil, err
		}
		out = append(out, sessionViewTurn{
			SessionID: sessionID,
			TurnIndex: turnIndex,
			Content:   string(content),
			Finished:  true,
		})
	}
	return out, nil
}

func (s *fileSessionTurnStore) latestTurnIndex(ctx context.Context, projectName, sessionID string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	dir := s.turnDir(projectName, sessionID)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read turn dir: %w", err)
	}
	latest := int64(0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var fileNo int64
		if _, err := fmt.Sscanf(entry.Name(), "t%06d.bin", &fileNo); err != nil {
			continue
		}
		fileLatest, err := s.latestTurnIndexInFile(filepath.Join(dir, entry.Name()), fileNo)
		if err != nil {
			return 0, err
		}
		if fileLatest > latest {
			latest = fileLatest
		}
	}
	return latest, nil
}

func (s *fileSessionTurnStore) writeTurn(ctx context.Context, projectName, sessionID string, turnIndex int64, content []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, fileNo, slot := s.turnPath(projectName, sessionID, turnIndex)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir turn dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("open turn file: %w", err)
	}
	defer f.Close()
	header, err := readOrInitTurnHeader(f)
	if err != nil {
		return fmt.Errorf("read turn header: %w", err)
	}
	firstEmpty, end, err := turnHeaderState(header, fileNo)
	if err != nil {
		return err
	}
	if slot != firstEmpty {
		return fmt.Errorf("turn slot %d is not next writable slot %d", slot, firstEmpty)
	}
	if err := f.Truncate(end); err != nil {
		return fmt.Errorf("truncate orphan turn body: %w", err)
	}
	if _, err := f.Seek(end, io.SeekStart); err != nil {
		return fmt.Errorf("seek turn body: %w", err)
	}
	if _, err := f.Write(content); err != nil {
		return fmt.Errorf("write turn body: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync turn body: %w", err)
	}
	slotOffset := int64(8 + slot*sessionTurnFileMetaSize)
	slotBuf := make([]byte, sessionTurnFileMetaSize)
	binary.LittleEndian.PutUint32(slotBuf[0:4], uint32(end))
	binary.LittleEndian.PutUint32(slotBuf[4:8], uint32(len(content)))
	if _, err := f.WriteAt(slotBuf, slotOffset); err != nil {
		return fmt.Errorf("write turn header slot: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync turn header: %w", err)
	}
	return nil
}

func (s *fileSessionTurnStore) readTurn(ctx context.Context, projectName, sessionID string, turnIndex int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, _, slot := s.turnPath(projectName, sessionID, turnIndex)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read turn file: %w", err)
	}
	if len(raw) < sessionTurnFileHeadSize {
		return nil, fmt.Errorf("turn file header too short")
	}
	if err := validateTurnHeader(raw[:sessionTurnFileHeadSize]); err != nil {
		return nil, err
	}
	offset, length := turnSlot(raw[:sessionTurnFileHeadSize], slot)
	if offset == 0 || length == 0 {
		return nil, fmt.Errorf("turn %d not found", turnIndex)
	}
	end := int(offset) + int(length)
	if int(offset) < sessionTurnFileHeadSize || end > len(raw) {
		return nil, fmt.Errorf("turn %d slot points outside file", turnIndex)
	}
	return raw[offset:end], nil
}

func (s *fileSessionTurnStore) latestTurnIndexInFile(path string, fileNo int64) (int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read turn file: %w", err)
	}
	if len(raw) < sessionTurnFileHeadSize {
		return 0, fmt.Errorf("turn file header too short")
	}
	header := raw[:sessionTurnFileHeadSize]
	if err := validateTurnHeader(header); err != nil {
		return 0, err
	}
	latest := int64(0)
	for slot := 0; slot < sessionTurnsPerFile; slot++ {
		offset, length := turnSlot(header, slot)
		if offset == 0 || length == 0 {
			break
		}
		if int(offset) < sessionTurnFileHeadSize || int(offset)+int(length) > len(raw) {
			return 0, fmt.Errorf("turn slot %d points outside file", slot)
		}
		latest = fileNo*sessionTurnsPerFile + int64(slot) + 1
	}
	return latest, nil
}

func (s *fileSessionTurnStore) turnDir(projectName, sessionID string) string {
	return filepath.Join(s.root, safeHistoryPathPart(projectName), safeHistoryPathPart(sessionID), "turns")
}

func (s *fileSessionTurnStore) turnPath(projectName, sessionID string, turnIndex int64) (string, int64, int) {
	fileNo := (turnIndex - 1) / sessionTurnsPerFile
	slot := int((turnIndex - 1) % sessionTurnsPerFile)
	path := filepath.Join(s.turnDir(projectName, sessionID), fmt.Sprintf("t%06d.bin", fileNo))
	return path, fileNo, slot
}

func readOrInitTurnHeader(f *os.File) ([]byte, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() == 0 {
		header := make([]byte, sessionTurnFileHeadSize)
		copy(header[0:4], sessionTurnFileMagic)
		binary.LittleEndian.PutUint16(header[4:6], sessionTurnFileVersion)
		if _, err := f.WriteAt(header, 0); err != nil {
			return nil, err
		}
		return header, nil
	}
	if info.Size() < sessionTurnFileHeadSize {
		return nil, fmt.Errorf("turn file header too short")
	}
	header := make([]byte, sessionTurnFileHeadSize)
	if _, err := f.ReadAt(header, 0); err != nil {
		return nil, err
	}
	if err := validateTurnHeader(header); err != nil {
		return nil, err
	}
	return header, nil
}

func validateTurnHeader(header []byte) error {
	if len(header) < sessionTurnFileHeadSize {
		return fmt.Errorf("turn file header too short")
	}
	if string(header[0:4]) != sessionTurnFileMagic {
		return fmt.Errorf("invalid turn file magic")
	}
	if version := binary.LittleEndian.Uint16(header[4:6]); version != sessionTurnFileVersion {
		return fmt.Errorf("unsupported turn file version %d", version)
	}
	return nil
}

func turnHeaderState(header []byte, fileNo int64) (int, int64, error) {
	if err := validateTurnHeader(header); err != nil {
		return 0, 0, err
	}
	end := int64(sessionTurnFileHeadSize)
	for slot := 0; slot < sessionTurnsPerFile; slot++ {
		offset, length := turnSlot(header, slot)
		if offset == 0 || length == 0 {
			return slot, end, nil
		}
		if int(offset) < sessionTurnFileHeadSize {
			return 0, 0, fmt.Errorf("turn file %d slot %d offset before body", fileNo, slot)
		}
		nextEnd := int64(offset) + int64(length)
		if nextEnd < end {
			return 0, 0, fmt.Errorf("turn file %d slot %d overlaps previous body", fileNo, slot)
		}
		end = nextEnd
	}
	return sessionTurnsPerFile, end, nil
}

func turnSlot(header []byte, slot int) (uint32, uint32) {
	pos := 8 + slot*sessionTurnFileMetaSize
	return binary.LittleEndian.Uint32(header[pos : pos+4]), binary.LittleEndian.Uint32(header[pos+4 : pos+8])
}
