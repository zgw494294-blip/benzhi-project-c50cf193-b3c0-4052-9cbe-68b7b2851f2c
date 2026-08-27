package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCommitAndRecover(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	projection := map[string]any{"value": "ok"}
	event, err := store.Commit(PendingEvent{EventID: "E-1", BatchID: "B-1", Type: "created", Actor: "tester", IdempotencyKey: "key-0001", OccurredAt: time.Now(), Payload: map[string]string{"x": "y"}}, projection)
	if err != nil {
		t.Fatal(err)
	}
	if event.Sequence != 1 || len(event.Hash) != 64 {
		t.Fatalf("事件封套错误: %#v", event)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var loaded map[string]any
	ok, err := reopened.LoadSnapshot(&loaded)
	if err != nil || !ok {
		t.Fatalf("快照恢复失败: %v", err)
	}
	if loaded["value"] != "ok" {
		t.Fatalf("投影错误: %#v", loaded)
	}
}

func TestStoreDetectsTamperedLedger(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Commit(PendingEvent{EventID: "E-1", BatchID: "B", Type: "x", OccurredAt: time.Now(), Payload: map[string]string{"secret": "original"}}, map[string]string{"v": "1"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for index := range data {
		if index+8 <= len(data) && string(data[index:index+8]) == "original" {
			copy(data[index:index+8], []byte("tampered"))
			break
		}
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("篡改账本后应拒绝启动")
	}
}
