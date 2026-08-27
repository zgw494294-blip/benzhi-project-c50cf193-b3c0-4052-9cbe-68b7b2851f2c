package persistence

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store 串行化所有写入，并维护已验证的账本头。
type Store struct {
	mu           sync.Mutex
	dir          string
	ledgerPath   string
	snapshotPath string
	lastSequence uint64
	lastHash     string
}

// Open 验证现有账本后打开本地持久层。
func Open(dir string) (*Store, error) {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "." || dir == "" {
		return nil, errors.New("持久化目录必须是明确的子目录")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("创建持久化目录: %w", err)
	}
	store := &Store{dir: dir, ledgerPath: filepath.Join(dir, "events.jsonl"), snapshotPath: filepath.Join(dir, "projection.json")}
	events, err := store.loadLedger()
	if err != nil {
		return nil, err
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		store.lastSequence = last.Sequence
		store.lastHash = last.Hash
	}
	return store, nil
}

// Events 返回启动重放所需的已验证事件。
func (s *Store) Events() ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLedger()
}

func (s *Store) loadLedger() ([]Event, error) {
	file, err := os.Open(s.ledgerPath)
	if errors.Is(err, os.ErrNotExist) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("打开事件账本: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	events := make([]Event, 0)
	var previous string
	var sequence uint64
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("解析事件账本第 %d 行: %w", len(events)+1, err)
		}
		if event.SchemaVersion != SchemaVersion {
			return nil, fmt.Errorf("事件 %s schema_version=%d 不受支持", event.EventID, event.SchemaVersion)
		}
		if event.Sequence != sequence+1 {
			return nil, fmt.Errorf("事件序号不连续: 期望 %d，得到 %d", sequence+1, event.Sequence)
		}
		if event.PreviousHash != previous {
			return nil, fmt.Errorf("事件 %d 的摘要链前驱不匹配", event.Sequence)
		}
		expected, err := hashEvent(event)
		if err != nil {
			return nil, err
		}
		if event.Hash != expected {
			return nil, fmt.Errorf("事件 %d 摘要校验失败", event.Sequence)
		}
		events = append(events, event)
		previous, sequence = event.Hash, event.Sequence
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取事件账本: %w", err)
	}
	return events, nil
}

// Commit 在同一提交锁内追加事件、sync 文件并原子替换投影。
func (s *Store) Commit(pending PendingEvent, projection any) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.Marshal(pending.Payload)
	if err != nil {
		return Event{}, fmt.Errorf("编码事件载荷: %w", err)
	}
	event := Event{
		SchemaVersion: SchemaVersion, Sequence: s.lastSequence + 1,
		EventID: pending.EventID, BatchID: pending.BatchID, Type: pending.Type,
		Actor: pending.Actor, IdempotencyKey: pending.IdempotencyKey,
		OccurredAt: pending.OccurredAt.UTC(), Payload: payload, PreviousHash: s.lastHash,
	}
	event.Hash, err = hashEvent(event)
	if err != nil {
		return Event{}, err
	}
	line, err := json.Marshal(event)
	if err != nil {
		return Event{}, fmt.Errorf("编码事件: %w", err)
	}
	file, err := os.OpenFile(s.ledgerPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return Event{}, fmt.Errorf("打开事件账本写入: %w", err)
	}
	if _, err = file.Write(append(line, '\n')); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return Event{}, fmt.Errorf("追加事件: %w", err)
	}
	if closeErr != nil {
		return Event{}, fmt.Errorf("关闭事件账本: %w", closeErr)
	}
	if err := s.writeSnapshotLocked(projection, event.Sequence, event.Hash); err != nil {
		return Event{}, err
	}
	s.lastSequence, s.lastHash = event.Sequence, event.Hash
	return event, nil
}

func hashEvent(event Event) (string, error) {
	event.Hash = ""
	data, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("编码摘要输入: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Close 验证目录句柄可同步；Store 不持有常驻文件描述符。
func (s *Store) Close() error {
	directory, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func copyAll(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	return err
}
