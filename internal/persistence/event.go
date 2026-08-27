package persistence

import (
	"encoding/json"
	"time"
)

const SchemaVersion = 1

// Event 是账本中的不可变事件封套。
type Event struct {
	SchemaVersion  int             `json:"schema_version"`
	Sequence       uint64          `json:"sequence"`
	EventID        string          `json:"event_id"`
	BatchID        string          `json:"batch_id"`
	Type           string          `json:"type"`
	Actor          string          `json:"actor"`
	IdempotencyKey string          `json:"idempotency_key"`
	OccurredAt     time.Time       `json:"occurred_at"`
	Payload        json.RawMessage `json:"payload"`
	PreviousHash   string          `json:"previous_hash"`
	Hash           string          `json:"hash"`
}

// PendingEvent 由工作流提交，序号与哈希由 Store 分配。
type PendingEvent struct {
	EventID        string
	BatchID        string
	Type           string
	Actor          string
	IdempotencyKey string
	OccurredAt     time.Time
	Payload        any
}

// SnapshotFile 是可恢复投影的通用外壳。
type SnapshotFile struct {
	SchemaVersion int             `json:"schema_version"`
	LastSequence  uint64          `json:"last_sequence"`
	LastHash      string          `json:"last_hash"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Projection    json.RawMessage `json:"projection"`
}
