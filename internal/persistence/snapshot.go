package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (s *Store) writeSnapshotLocked(projection any, sequence uint64, hash string) error {
	payload, err := json.Marshal(projection)
	if err != nil {
		return fmt.Errorf("编码投影: %w", err)
	}
	wrapper := SnapshotFile{SchemaVersion: SchemaVersion, LastSequence: sequence, LastHash: hash, UpdatedAt: time.Now().UTC(), Projection: payload}
	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("编码快照: %w", err)
	}
	temporary, err := os.CreateTemp(s.dir, ".projection-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时快照: %w", err)
	}
	name := temporary.Name()
	cleanup := func() { _ = os.Remove(name) }
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		cleanup()
		return err
	}
	if _, err := temporary.Write(data); err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err != nil {
		cleanup()
		return fmt.Errorf("写入临时快照: %w", err)
	}
	if closeErr != nil {
		cleanup()
		return fmt.Errorf("关闭临时快照: %w", closeErr)
	}
	if err := os.Rename(name, s.snapshotPath); err != nil {
		cleanup()
		return fmt.Errorf("原子替换快照: %w", err)
	}
	directory, err := os.Open(filepath.Dir(s.snapshotPath))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// LoadSnapshot 读取快照并验证它与账本头一致。
func (s *Store) LoadSnapshot(target any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.snapshotPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取投影快照: %w", err)
	}
	var wrapper SnapshotFile
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return false, fmt.Errorf("解析投影快照: %w", err)
	}
	if wrapper.SchemaVersion != SchemaVersion {
		return false, fmt.Errorf("快照 schema_version=%d 不受支持", wrapper.SchemaVersion)
	}
	if wrapper.LastSequence != s.lastSequence || wrapper.LastHash != s.lastHash {
		return false, errors.New("投影快照与事件账本头不一致，需要重放恢复")
	}
	if err := json.Unmarshal(wrapper.Projection, target); err != nil {
		return false, fmt.Errorf("解析投影内容: %w", err)
	}
	return true, nil
}

// ReplaceSnapshot 在完整重放后重建投影，不追加业务事件。
func (s *Store) ReplaceSnapshot(projection any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeSnapshotLocked(projection, s.lastSequence, s.lastHash)
}
