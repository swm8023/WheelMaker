package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	sessionTurnFileMagic          = "WMT2"
	sessionTurnFileVersion        = uint16(2)
	sessionTurnFileLegacyVersion  = uint16(1)
	sessionTurnFilePreambleSize   = 8
	sessionTurnFileMetaSize       = 8
	sessionTurnsPerFileLegacy     = 128
	sessionTurnsPerFile           = 256
	sessionTurnFileHeadSize       = sessionTurnFilePreambleSize + sessionTurnsPerFile*sessionTurnFileMetaSize
	sessionTurnFileLegacyHeadSize = sessionTurnFilePreambleSize + sessionTurnsPerFileLegacy*sessionTurnFileMetaSize
)

var errTurnNotFound = errors.New("turn not found")

type sessionTurnFileFormat struct {
	version      uint16
	turnsPerFile int
	headSize     int
}

var (
	sessionTurnFileCurrentFormat = sessionTurnFileFormat{
		version:      sessionTurnFileVersion,
		turnsPerFile: sessionTurnsPerFile,
		headSize:     sessionTurnFileHeadSize,
	}
	sessionTurnFileLegacyFormat = sessionTurnFileFormat{
		version:      sessionTurnFileLegacyVersion,
		turnsPerFile: sessionTurnsPerFileLegacy,
		headSize:     sessionTurnFileLegacyHeadSize,
	}
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
	format, err := s.activeTurnFileFormat(ctx, projectName, sessionID)
	if err != nil {
		return 0, err
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
		if err := s.writeTurn(ctx, projectName, sessionID, format, next, []byte(content)); err != nil {
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
	format, err := s.activeTurnFileFormat(ctx, projectName, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]sessionViewTurn, 0, latestTurnIndex-afterTurnIndex)
	for turnIndex := afterTurnIndex + 1; turnIndex <= latestTurnIndex; turnIndex++ {
		content, err := s.readTurn(ctx, projectName, sessionID, format, turnIndex)
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

func (s *fileSessionTurnStore) DeleteTurns(ctx context.Context, projectName, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return nil
	}
	if err := os.RemoveAll(s.turnDir(projectName, sessionID)); err != nil {
		return fmt.Errorf("delete turn dir: %w", err)
	}
	return nil
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

func (s *fileSessionTurnStore) activeTurnFileFormat(ctx context.Context, projectName, sessionID string) (sessionTurnFileFormat, error) {
	if err := ctx.Err(); err != nil {
		return sessionTurnFileFormat{}, err
	}
	dir := s.turnDir(projectName, sessionID)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return sessionTurnFileCurrentFormat, nil
	}
	if err != nil {
		return sessionTurnFileFormat{}, fmt.Errorf("read turn dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var fileNo int64
		if _, err := fmt.Sscanf(entry.Name(), "t%06d.bin", &fileNo); err != nil {
			continue
		}
		return readTurnFileFormat(filepath.Join(dir, entry.Name()))
	}
	return sessionTurnFileCurrentFormat, nil
}

func (s *fileSessionTurnStore) writeTurn(ctx context.Context, projectName, sessionID string, format sessionTurnFileFormat, turnIndex int64, content []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, fileNo, slot := s.turnPath(projectName, sessionID, format, turnIndex)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir turn dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("open turn file: %w", err)
	}
	defer f.Close()
	header, actualFormat, err := readOrInitTurnHeader(f, format)
	if err != nil {
		return fmt.Errorf("read turn header: %w", err)
	}
	if actualFormat.version != format.version {
		return fmt.Errorf("turn file version %d does not match active session version %d", actualFormat.version, format.version)
	}
	firstEmpty, end, err := turnHeaderState(header, actualFormat, fileNo)
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
	slotOffset := int64(sessionTurnFilePreambleSize + slot*sessionTurnFileMetaSize)
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

func (s *fileSessionTurnStore) readTurn(ctx context.Context, projectName, sessionID string, format sessionTurnFileFormat, turnIndex int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, _, slot := s.turnPath(projectName, sessionID, format, turnIndex)
	raw, actualFormat, err := readTurnFileBytes(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("turn %d: %w", turnIndex, errTurnNotFound)
		}
		return nil, err
	}
	if actualFormat.version != format.version {
		return nil, fmt.Errorf("turn file version %d does not match active session version %d", actualFormat.version, format.version)
	}
	if slot < 0 || slot >= actualFormat.turnsPerFile {
		return nil, fmt.Errorf("turn %d: %w", turnIndex, errTurnNotFound)
	}
	header := raw[:actualFormat.headSize]
	offset, length := turnSlot(header, slot)
	if offset == 0 || length == 0 {
		return nil, fmt.Errorf("turn %d: %w", turnIndex, errTurnNotFound)
	}
	end := int(offset) + int(length)
	if int(offset) < actualFormat.headSize || end > len(raw) {
		return nil, fmt.Errorf("turn %d slot points outside file", turnIndex)
	}
	return raw[int(offset):end], nil
}

func (s *fileSessionTurnStore) latestTurnIndexInFile(path string, fileNo int64) (int64, error) {
	raw, format, err := readTurnFileBytes(path)
	if err != nil {
		return 0, err
	}
	header := raw[:format.headSize]
	latest := int64(0)
	for slot := 0; slot < format.turnsPerFile; slot++ {
		offset, length := turnSlot(header, slot)
		if offset == 0 || length == 0 {
			break
		}
		if int(offset) < format.headSize || int(offset)+int(length) > len(raw) {
			return 0, fmt.Errorf("turn slot %d points outside file", slot)
		}
		latest = fileNo*int64(format.turnsPerFile) + int64(slot) + 1
	}
	return latest, nil
}

func (s *fileSessionTurnStore) turnDir(projectName, sessionID string) string {
	return filepath.Join(s.root, safeHistoryPathPart(projectName), safeHistoryPathPart(sessionID), "turns")
}

func (s *fileSessionTurnStore) turnPath(projectName, sessionID string, format sessionTurnFileFormat, turnIndex int64) (string, int64, int) {
	fileNo := (turnIndex - 1) / int64(format.turnsPerFile)
	slot := int((turnIndex - 1) % int64(format.turnsPerFile))
	path := filepath.Join(s.turnDir(projectName, sessionID), fmt.Sprintf("t%06d.bin", fileNo))
	return path, fileNo, slot
}

func readOrInitTurnHeader(f *os.File, format sessionTurnFileFormat) ([]byte, sessionTurnFileFormat, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, sessionTurnFileFormat{}, err
	}
	if info.Size() == 0 {
		header := make([]byte, format.headSize)
		copy(header[0:4], sessionTurnFileMagic)
		binary.LittleEndian.PutUint16(header[4:6], format.version)
		if _, err := f.WriteAt(header, 0); err != nil {
			return nil, sessionTurnFileFormat{}, err
		}
		return header, format, nil
	}
	if info.Size() < sessionTurnFilePreambleSize {
		return nil, sessionTurnFileFormat{}, fmt.Errorf("turn file header too short")
	}
	preamble := make([]byte, sessionTurnFilePreambleSize)
	if _, err := f.ReadAt(preamble, 0); err != nil {
		return nil, sessionTurnFileFormat{}, err
	}
	actualFormat, err := detectTurnFileFormat(preamble)
	if err != nil {
		return nil, sessionTurnFileFormat{}, err
	}
	if info.Size() < int64(actualFormat.headSize) {
		return nil, sessionTurnFileFormat{}, fmt.Errorf("turn file header too short")
	}
	header := make([]byte, actualFormat.headSize)
	if _, err := f.ReadAt(header, 0); err != nil {
		return nil, sessionTurnFileFormat{}, err
	}
	if _, err := validateTurnHeader(header); err != nil {
		return nil, sessionTurnFileFormat{}, err
	}
	return header, actualFormat, nil
}

func readTurnFileFormat(path string) (sessionTurnFileFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return sessionTurnFileFormat{}, fmt.Errorf("read turn file: %w", err)
	}
	defer f.Close()
	preamble := make([]byte, sessionTurnFilePreambleSize)
	if _, err := io.ReadFull(f, preamble); err != nil {
		return sessionTurnFileFormat{}, fmt.Errorf("turn file header too short")
	}
	return detectTurnFileFormat(preamble)
}

func readTurnFileBytes(path string) ([]byte, sessionTurnFileFormat, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, sessionTurnFileFormat{}, fmt.Errorf("read turn file: %w", err)
	}
	format, err := validateTurnHeader(raw)
	if err != nil {
		return nil, sessionTurnFileFormat{}, err
	}
	return raw, format, nil
}

func validateTurnHeader(header []byte) (sessionTurnFileFormat, error) {
	format, err := detectTurnFileFormat(header)
	if err != nil {
		return sessionTurnFileFormat{}, err
	}
	if len(header) < format.headSize {
		return sessionTurnFileFormat{}, fmt.Errorf("turn file header too short")
	}
	return format, nil
}

func detectTurnFileFormat(header []byte) (sessionTurnFileFormat, error) {
	if len(header) < sessionTurnFilePreambleSize {
		return sessionTurnFileFormat{}, fmt.Errorf("turn file header too short")
	}
	if string(header[0:4]) != sessionTurnFileMagic {
		return sessionTurnFileFormat{}, fmt.Errorf("invalid turn file magic")
	}
	version := binary.LittleEndian.Uint16(header[4:6])
	switch version {
	case sessionTurnFileVersion:
		return sessionTurnFileCurrentFormat, nil
	case sessionTurnFileLegacyVersion:
		return sessionTurnFileLegacyFormat, nil
	default:
		return sessionTurnFileFormat{}, fmt.Errorf("unsupported turn file version %d", version)
	}
}

func turnHeaderState(header []byte, format sessionTurnFileFormat, fileNo int64) (int, int64, error) {
	if _, err := validateTurnHeader(header); err != nil {
		return 0, 0, err
	}
	end := int64(format.headSize)
	for slot := 0; slot < format.turnsPerFile; slot++ {
		offset, length := turnSlot(header, slot)
		if offset == 0 || length == 0 {
			return slot, end, nil
		}
		if int(offset) < format.headSize {
			return 0, 0, fmt.Errorf("turn file %d slot %d offset before body", fileNo, slot)
		}
		nextEnd := int64(offset) + int64(length)
		if nextEnd < end {
			return 0, 0, fmt.Errorf("turn file %d slot %d overlaps previous body", fileNo, slot)
		}
		end = nextEnd
	}
	return format.turnsPerFile, end, nil
}

func turnSlot(header []byte, slot int) (uint32, uint32) {
	pos := sessionTurnFilePreambleSize + slot*sessionTurnFileMetaSize
	return binary.LittleEndian.Uint32(header[pos : pos+4]), binary.LittleEndian.Uint32(header[pos+4 : pos+8])
}

func safeHistoryPathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	replacer := strings.NewReplacer(
		"\\", "_",
		"/", "_",
		":", "_",
		"*", "_",
		"?", "_",
		`"`, "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(value)
}
