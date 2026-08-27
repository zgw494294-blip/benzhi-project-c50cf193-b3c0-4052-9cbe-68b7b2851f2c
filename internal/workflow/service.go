package workflow

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"seedvault/internal/persistence"
)

type clockFunc func() time.Time

type projection struct {
	Batches     map[string]*BatchRecord `json:"batches"`
	Credentials map[string]*Credential  `json:"credentials"`
	Idempotency map[string]string       `json:"idempotency"`
}

type eventProjection struct {
	Batch      *BatchRecord `json:"batch,omitempty"`
	Credential *Credential  `json:"credential,omitempty"`
	ResultRef  string       `json:"result_ref"`
}

// Service 持有工作流投影，并用一把提交锁串行化版本检查和持久化。
type Service struct {
	mu    sync.RWMutex
	store *persistence.Store
	now   clockFunc
	state projection
}

func NewService(store *persistence.Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("persistence store 不能为空")
	}
	s := &Service{store: store, now: time.Now, state: emptyProjection()}
	loaded, err := store.LoadSnapshot(&s.state)
	if err == nil && loaded {
		s.normalizeMaps()
		if err := validateProjection(s.state); err != nil {
			return nil, fmt.Errorf("投影快照不变量失败: %w", err)
		}
		return s, nil
	}
	if err := s.replay(); err != nil {
		return nil, fmt.Errorf("启动重放失败: %w", err)
	}
	if err := store.ReplaceSnapshot(s.state); err != nil {
		return nil, fmt.Errorf("重建投影快照: %w", err)
	}
	return s, nil
}

func emptyProjection() projection {
	return projection{Batches: map[string]*BatchRecord{}, Credentials: map[string]*Credential{}, Idempotency: map[string]string{}}
}

func (s *Service) normalizeMaps() {
	if s.state.Batches == nil {
		s.state.Batches = map[string]*BatchRecord{}
	}
	if s.state.Credentials == nil {
		s.state.Credentials = map[string]*Credential{}
	}
	if s.state.Idempotency == nil {
		s.state.Idempotency = map[string]string{}
	}
}

func (s *Service) replay() error {
	events, err := s.store.Events()
	if err != nil {
		return err
	}
	s.state = emptyProjection()
	for _, event := range events {
		var payload eventProjection
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("重放事件 %d: %w", event.Sequence, err)
		}
		if payload.Batch != nil {
			payload.Batch.Timeline = append(payload.Batch.Timeline, AuditEntry{Sequence: event.Sequence, EventID: event.EventID, Type: event.Type, Actor: event.Actor, Summary: eventSummary(event.Type), OccurredAt: event.OccurredAt, Hash: event.Hash})
			s.state.Batches[payload.Batch.Batch.BatchID] = payload.Batch
		}
		if payload.Credential != nil {
			s.state.Credentials[payload.Credential.CredentialID] = payload.Credential
		}
		if event.IdempotencyKey != "" {
			s.state.Idempotency[event.IdempotencyKey] = payload.ResultRef
		}
	}
	return validateProjection(s.state)
}

func (s *Service) ListBatches() []BatchRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]BatchRecord, 0, len(s.state.Batches))
	for _, record := range s.state.Batches {
		result = append(result, cloneBatch(record))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Batch.CreatedAt.Equal(result[j].Batch.CreatedAt) {
			return result[i].Batch.BatchID < result[j].Batch.BatchID
		}
		return result[i].Batch.CreatedAt.After(result[j].Batch.CreatedAt)
	})
	return result
}

func (s *Service) GetBatch(id string) (BatchRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.state.Batches[strings.TrimSpace(id)]
	if !ok {
		return BatchRecord{}, &DomainError{Code: CodeNotFound, Message: "批次不存在"}
	}
	return cloneBatch(record), nil
}

func (s *Service) GetCredential(id string) (Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	credential, ok := s.state.Credentials[strings.TrimSpace(id)]
	if !ok {
		return Credential{}, &DomainError{Code: CodeNotFound, Message: "凭据不存在"}
	}
	return *credential, nil
}

func requireRole(actual string, allowed ...string) error {
	for _, role := range allowed {
		if strings.EqualFold(strings.TrimSpace(actual), role) {
			return nil
		}
	}
	return &DomainError{Code: CodeForbidden, Message: "当前角色无权执行此操作", Fields: map[string]string{"role": "需要角色: " + strings.Join(allowed, ", ")}}
}

func requireKey(key string) error {
	if len(strings.TrimSpace(key)) < 8 {
		return invalid("idempotency_key", "幂等键至少需要 8 个字符")
	}
	return nil
}

func requireVersion(record *BatchRecord, expected int) error {
	if record.Batch.Version != expected {
		return conflict(expected, record.Batch.Version)
	}
	return nil
}

func newID(prefix string) string {
	data := make([]byte, 8)
	if _, err := rand.Read(data); err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())))
		data = sum[:8]
	}
	return prefix + "-" + hex.EncodeToString(data)
}

func cloneBatch(record *BatchRecord) BatchRecord {
	data, _ := json.Marshal(record)
	var cloned BatchRecord
	_ = json.Unmarshal(data, &cloned)
	return cloned
}

func eventSummary(eventType string) string {
	return map[string]string{"batch.created": "建立批次档案", "test.recorded": "录入检测并执行质量检查", "remediation.submitted": "提交整改说明和替代复测", "review.returned": "复核退回补充", "review.approved": "独立复核通过", "batch.frozen": "冻结核验快照并签发凭据", "credential.revoked": "撤销入库凭据"}[eventType]
}
