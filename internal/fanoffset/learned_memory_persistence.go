package fanoffset

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
)

const (
	learnedMemorySnapshotVersion = 1
	learnedMemorySnapshotEvery   = 24
)

type learnedMemoryEntry struct {
	CurveSignature string  `json:"curveSignature"`
	ZoneTemp       int     `json:"zoneTemp"`
	Offset         float64 `json:"offset"`
	Confidence     float64 `json:"confidence"`
	Successes      int     `json:"successes"`
	Failures       int     `json:"failures"`
	HasSeed        bool    `json:"hasSeed"`
}

type learnedMemorySnapshot struct {
	Version int                  `json:"version"`
	SavedAt time.Time            `json:"savedAt"`
	Entries []learnedMemoryEntry `json:"entries"`
}

type learnedMemoryJournalRecord struct {
	Version int                `json:"version"`
	At      time.Time          `json:"at"`
	Op      string             `json:"op"`
	Entry   learnedMemoryEntry `json:"entry"`
}

type learnedMemoryPersistence struct {
	mu sync.Mutex

	dir          string
	snapshotPath string
	journalPath  string
	logger       types.Logger

	pendingJournal int
	snapshotEvery  int
}

func newLearnedMemoryPersistence(baseDir string, logger types.Logger) (*learnedMemoryPersistence, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil, errors.New("empty learned memory persistence dir")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}
	return &learnedMemoryPersistence{
		dir:            baseDir,
		snapshotPath:   filepath.Join(baseDir, "learned-memory.snapshot.json"),
		journalPath:    filepath.Join(baseDir, "learned-memory.journal"),
		logger:         logger,
		snapshotEvery:  learnedMemorySnapshotEvery,
		pendingJournal: 0,
	}, nil
}

func (p *learnedMemoryPersistence) Load() (*learnedMemoryStore, error) {
	if p == nil {
		return newLearnedMemoryStore(), nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	store := newLearnedMemoryStore()
	if err := p.loadSnapshotLocked(store); err != nil {
		return nil, err
	}
	applied, err := p.applyJournalLocked(store)
	if err != nil {
		return nil, err
	}
	p.pendingJournal = 0
	if applied > 0 {
		if err := p.snapshotLocked(store); err != nil {
			p.logWarn("风扇偏移长期记忆载入后压缩 journal 失败", "error", err)
		}
	}
	return store, nil
}

func (p *learnedMemoryPersistence) RecordSet(curveSignature string, zoneTemp int, state learnedState) (bool, error) {
	return p.appendRecord(learnedMemoryJournalRecord{
		Version: learnedMemorySnapshotVersion,
		At:      time.Now(),
		Op:      "upsert",
		Entry:   learnedMemoryEntryFromState(curveSignature, zoneTemp, state),
	})
}

func (p *learnedMemoryPersistence) RecordDelete(curveSignature string, zoneTemp int) (bool, error) {
	return p.appendRecord(learnedMemoryJournalRecord{
		Version: learnedMemorySnapshotVersion,
		At:      time.Now(),
		Op:      "delete",
		Entry: learnedMemoryEntry{
			CurveSignature: curveSignature,
			ZoneTemp:       zoneTemp,
		},
	})
}

func (p *learnedMemoryPersistence) Snapshot(store *learnedMemoryStore) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotLocked(store)
}

func (p *learnedMemoryPersistence) appendRecord(record learnedMemoryJournalRecord) (bool, error) {
	if p == nil {
		return false, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.appendJournalLocked(record); err != nil {
		return false, err
	}
	p.pendingJournal++
	return p.pendingJournal >= max(1, p.snapshotEvery), nil
}

func (p *learnedMemoryPersistence) loadSnapshotLocked(store *learnedMemoryStore) error {
	data, err := os.ReadFile(p.snapshotPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var snapshot learnedMemorySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	for _, entry := range snapshot.Entries {
		store.Set(entry.CurveSignature, entry.ZoneTemp, entry.toState())
	}
	return nil
}

func (p *learnedMemoryPersistence) applyJournalLocked(store *learnedMemoryStore) (int, error) {
	file, err := os.Open(p.journalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	applied := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record learnedMemoryJournalRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			p.logWarn("风扇偏移长期记忆 journal 存在损坏记录，已跳过", "error", err)
			continue
		}
		switch record.Op {
		case "delete":
			store.Delete(record.Entry.CurveSignature, record.Entry.ZoneTemp)
		default:
			store.Set(record.Entry.CurveSignature, record.Entry.ZoneTemp, record.Entry.toState())
		}
		applied++
	}
	if err := scanner.Err(); err != nil {
		return applied, err
	}
	return applied, nil
}

func (p *learnedMemoryPersistence) appendJournalLocked(record learnedMemoryJournalRecord) error {
	if err := os.MkdirAll(p.dir, 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(p.journalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if _, err := file.Write(line); err != nil {
		return err
	}
	return file.Sync()
}

func (p *learnedMemoryPersistence) snapshotLocked(store *learnedMemoryStore) error {
	if store == nil {
		store = newLearnedMemoryStore()
	}
	entries := make([]learnedMemoryEntry, 0, len(store.byKey))
	for key, state := range store.byKey {
		entries = append(entries, learnedMemoryEntryFromState(key.curveSignature, key.zoneTemp, state))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CurveSignature == entries[j].CurveSignature {
			return entries[i].ZoneTemp < entries[j].ZoneTemp
		}
		return entries[i].CurveSignature < entries[j].CurveSignature
	})
	snapshot := learnedMemorySnapshot{
		Version: learnedMemorySnapshotVersion,
		SavedAt: time.Now(),
		Entries: entries,
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(p.snapshotPath, data, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(p.journalPath, nil, 0644); err != nil {
		return err
	}
	p.pendingJournal = 0
	return nil
}

func learnedMemoryEntryFromState(curveSignature string, zoneTemp int, state learnedState) learnedMemoryEntry {
	return learnedMemoryEntry{
		CurveSignature: curveSignature,
		ZoneTemp:       zoneTemp,
		Offset:         state.offset,
		Confidence:     state.confidence,
		Successes:      state.successes,
		Failures:       state.failures,
		HasSeed:        state.hasSeed,
	}
}

func (e learnedMemoryEntry) toState() learnedState {
	return learnedState{
		offset:     e.Offset,
		confidence: e.Confidence,
		successes:  e.Successes,
		failures:   e.Failures,
		hasSeed:    e.HasSeed,
	}
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil && !errors.Is(err, os.ErrPermission) {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(tmpPath, path); err != nil {
			return err
		}
	}
	keep = true
	return nil
}

func (p *learnedMemoryPersistence) logWarn(msg any, args ...any) {
	if p != nil && p.logger != nil {
		p.logger.Warn(msg, args...)
	}
}
