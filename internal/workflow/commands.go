package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"seedvault/internal/evidence"
	"seedvault/internal/persistence"
	"seedvault/internal/quality"
)

func (s *Service) CreateBatch(command CreateBatchCommand) (BatchRecord, error) {
	return s.CreateBatchContext(context.Background(), command)
}

// CreateBatchContext 接收请求生命周期，供 HTTP 写入链路传播取消信号。
func (s *Service) CreateBatchContext(_ context.Context, command CreateBatchCommand) (BatchRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	command.BatchID = strings.TrimSpace(command.BatchID)
	if err := requireRole(command.Role, "receiver"); err != nil {
		return BatchRecord{}, err
	}
	if err := requireKey(command.IdempotencyKey); err != nil {
		return BatchRecord{}, err
	}
	if record, done, err := s.idempotentBatch(command.IdempotencyKey, command.BatchID); done || err != nil {
		return record, err
	}
	if command.BatchID == "" {
		command.BatchID = newID("batch")
	}
	if _, exists := s.state.Batches[command.BatchID]; exists {
		return BatchRecord{}, invalid("batch_id", "批次编号已存在")
	}
	if strings.TrimSpace(command.Actor) == "" {
		return BatchRecord{}, invalid("actor", "操作人不能为空")
	}
	if strings.TrimSpace(command.SpeciesName) == "" {
		return BatchRecord{}, invalid("species_name", "物种名称不能为空")
	}
	if _, ok := quality.ProfileForSpecies(command.SpeciesName); !ok {
		return BatchRecord{}, invalid("species_name", "该物种没有已配置的质量方案")
	}
	if strings.TrimSpace(command.SourceRegion) == "" {
		return BatchRecord{}, invalid("source_region", "来源地区不能为空")
	}
	if command.SampleCount < 1 {
		return BatchRecord{}, invalid("sample_count", "分装样本数必须大于零")
	}
	if strings.TrimSpace(command.StorageCondition) == "" {
		return BatchRecord{}, invalid("storage_condition", "贮藏条件不能为空")
	}
	date, err := time.Parse("2006-01-02", command.HarvestDate)
	if err != nil || date.After(s.now()) {
		return BatchRecord{}, invalid("harvest_date", "采收日期必须是有效且不晚于今天的 YYYY-MM-DD")
	}
	now := s.now().UTC()
	record := &BatchRecord{Batch: SeedBatch{
		BatchID: command.BatchID, SpeciesName: strings.TrimSpace(command.SpeciesName), SourceRegion: strings.TrimSpace(command.SourceRegion),
		HarvestDate: command.HarvestDate, SampleCount: command.SampleCount, StorageCondition: strings.TrimSpace(command.StorageCondition),
		Status: StatusDraft, Version: 1, CreatedAt: now, CreatedBy: strings.TrimSpace(command.Actor),
	}, Tests: []evidence.TestRun{}, Remediations: []evidence.Remediation{}, Reviews: []ReviewDecision{}, Timeline: []AuditEntry{}}
	if err := s.commitRecord("batch.created", command.Actor, command.IdempotencyKey, record, nil); err != nil {
		return BatchRecord{}, err
	}
	return cloneBatch(record), nil
}

func (s *Service) RecordTest(command RecordTestCommand) (BatchRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := requireRole(command.Role, "tester"); err != nil {
		return BatchRecord{}, err
	}
	if err := requireKey(command.IdempotencyKey); err != nil {
		return BatchRecord{}, err
	}
	if record, done, err := s.idempotentBatch(command.IdempotencyKey, command.BatchID); done || err != nil {
		return record, err
	}
	record, err := s.mutableBatch(command.BatchID)
	if err != nil {
		return BatchRecord{}, err
	}
	if err := requireVersion(record, command.ExpectedVersion); err != nil {
		return BatchRecord{}, err
	}
	switch record.Batch.Status {
	case StatusDraft, StatusTesting, StatusReviewReturned:
	default:
		return BatchRecord{}, stateError(record.Batch.Status, "录入检测")
	}
	command.Test.Operator = strings.TrimSpace(command.Actor)
	run, err := evidence.NormalizeTest(record.Batch.BatchID, command.TestID, command.Test, s.now())
	if err != nil {
		return BatchRecord{}, invalid("test", err.Error())
	}
	if run.SupersedesTestID != "" {
		return BatchRecord{}, invalid("supersedes_test_id", "替代复测必须通过整改流程提交")
	}
	if err := evidence.ValidateLink(run, record.Tests); err != nil {
		return BatchRecord{}, invalid("test_id", err.Error())
	}
	record.Tests = append(record.Tests, run)
	record.Quality = quality.Evaluate(record.Batch.SpeciesName, record.Batch.SampleCount, run)
	if record.Quality.Passed {
		record.Batch.Status = StatusReadyReview
	} else {
		record.Batch.Status = StatusRemediationRequired
	}
	record.Batch.Version++
	if err := s.commitRecord("test.recorded", command.Actor, command.IdempotencyKey, record, nil); err != nil {
		return BatchRecord{}, err
	}
	return cloneBatch(record), nil
}

func (s *Service) SubmitRemediation(command RemediateCommand) (BatchRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := requireRole(command.Role, "receiver", "tester"); err != nil {
		return BatchRecord{}, err
	}
	if err := requireKey(command.IdempotencyKey); err != nil {
		return BatchRecord{}, err
	}
	if record, done, err := s.idempotentBatch(command.IdempotencyKey, command.BatchID); done || err != nil {
		return record, err
	}
	record, err := s.mutableBatch(command.BatchID)
	if err != nil {
		return BatchRecord{}, err
	}
	if err := requireVersion(record, command.ExpectedVersion); err != nil {
		return BatchRecord{}, err
	}
	if record.Batch.Status != StatusRemediationRequired && record.Batch.Status != StatusReviewReturned {
		return BatchRecord{}, stateError(record.Batch.Status, "提交整改复测")
	}
	if strings.TrimSpace(command.Explanation) == "" {
		return BatchRecord{}, invalid("explanation", "整改说明不能为空")
	}
	if len(command.IssueCodes) == 0 {
		return BatchRecord{}, invalid("issue_codes", "至少引用一个问题代码")
	}
	original, found := findTest(record.Tests, command.OriginalTestID)
	if !found {
		return BatchRecord{}, invalid("original_test_id", "引用的原测不存在")
	}
	if err := validateIssueRefs(record.Quality.Issues, command.IssueCodes); err != nil {
		return BatchRecord{}, err
	}
	command.Retest.Operator = strings.TrimSpace(command.Actor)
	command.Retest.SupersedesTestID = command.OriginalTestID
	retest, err := evidence.NormalizeTest(record.Batch.BatchID, command.RetestID, command.Retest, s.now())
	if err != nil {
		return BatchRecord{}, invalid("retest", err.Error())
	}
	if err := evidence.ValidateLink(retest, record.Tests); err != nil {
		return BatchRecord{}, invalid("retest", err.Error())
	}
	remediationID := strings.TrimSpace(command.RemediationID)
	if remediationID == "" {
		remediationID = newID("remediation")
	}
	for _, item := range record.Remediations {
		if item.RemediationID == remediationID {
			return BatchRecord{}, invalid("remediation_id", "整改编号已存在")
		}
	}
	remediation := evidence.Remediation{RemediationID: remediationID, BatchID: record.Batch.BatchID, IssueCodes: uniqueStrings(command.IssueCodes), Explanation: strings.TrimSpace(command.Explanation), OriginalTest: original.TestID, RetestID: retest.TestID, SubmittedBy: command.Actor, SubmittedAt: s.now().UTC()}
	record.Tests = append(record.Tests, retest)
	record.Remediations = append(record.Remediations, remediation)
	record.Quality = quality.CompareRetest(record.Batch.SpeciesName, record.Batch.SampleCount, original, retest)
	if record.Quality.Passed {
		record.Batch.Status = StatusReadyReview
	} else {
		record.Batch.Status = StatusRemediationRequired
	}
	record.Batch.Version++
	if err := s.commitRecord("remediation.submitted", command.Actor, command.IdempotencyKey, record, nil); err != nil {
		return BatchRecord{}, err
	}
	return cloneBatch(record), nil
}

func (s *Service) Review(command ReviewCommand) (BatchRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := requireRole(command.Role, "reviewer"); err != nil {
		return BatchRecord{}, err
	}
	if err := requireKey(command.IdempotencyKey); err != nil {
		return BatchRecord{}, err
	}
	if record, done, err := s.idempotentBatch(command.IdempotencyKey, command.BatchID); done || err != nil {
		return record, err
	}
	record, err := s.mutableBatch(command.BatchID)
	if err != nil {
		return BatchRecord{}, err
	}
	if err := requireVersion(record, command.ExpectedVersion); err != nil {
		return BatchRecord{}, err
	}
	if record.Batch.Status != StatusReadyReview {
		return BatchRecord{}, stateError(record.Batch.Status, "提交复核决定")
	}
	if strings.TrimSpace(command.Actor) == "" {
		return BatchRecord{}, invalid("actor", "复核员不能为空")
	}
	if command.Actor == record.Batch.CreatedBy {
		return BatchRecord{}, invalid("actor", "复核员必须独立于批次创建人")
	}
	for _, test := range record.Tests {
		if test.Operator == command.Actor {
			return BatchRecord{}, invalid("actor", "复核员不得参与该批次检测")
		}
	}
	decision := strings.ToUpper(strings.TrimSpace(command.Decision))
	if decision != "APPROVE" && decision != "RETURN" {
		return BatchRecord{}, invalid("decision", "decision 必须是 APPROVE 或 RETURN")
	}
	if decision == "RETURN" && (len(command.IssueRefs) == 0 || strings.TrimSpace(command.Comment) == "") {
		return BatchRecord{}, invalid("issue_refs", "退回时必须填写问题引用和说明")
	}
	reviewID := strings.TrimSpace(command.ReviewID)
	if reviewID == "" {
		reviewID = newID("review")
	}
	review := ReviewDecision{ReviewID: reviewID, BatchID: record.Batch.BatchID, Reviewer: command.Actor, Decision: decision, IssueRefs: uniqueStrings(command.IssueRefs), Comment: strings.TrimSpace(command.Comment), CreatedAt: s.now().UTC()}
	record.Reviews = append(record.Reviews, review)
	eventType := "review.returned"
	if decision == "APPROVE" {
		record.Batch.Status = StatusReviewApproved
		eventType = "review.approved"
	} else {
		record.Batch.Status = StatusReviewReturned
	}
	record.Batch.Version++
	if err := s.commitRecord(eventType, command.Actor, command.IdempotencyKey, record, nil); err != nil {
		return BatchRecord{}, err
	}
	return cloneBatch(record), nil
}

func (s *Service) Freeze(command FreezeCommand) (BatchRecord, Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := requireRole(command.Role, "administrator"); err != nil {
		return BatchRecord{}, Credential{}, err
	}
	if err := requireKey(command.IdempotencyKey); err != nil {
		return BatchRecord{}, Credential{}, err
	}
	if record, done, err := s.idempotentBatch(command.IdempotencyKey, command.BatchID); done || err != nil {
		if err != nil {
			return BatchRecord{}, Credential{}, err
		}
		credential := s.credentialForBatch(record.Batch.BatchID)
		return record, credential, nil
	}
	record, err := s.mutableBatch(command.BatchID)
	if err != nil {
		return BatchRecord{}, Credential{}, err
	}
	if err := requireVersion(record, command.ExpectedVersion); err != nil {
		return BatchRecord{}, Credential{}, err
	}
	if record.Batch.Status != StatusReviewApproved {
		return BatchRecord{}, Credential{}, stateError(record.Batch.Status, "冻结核验快照")
	}
	record.Manifest = evidence.BuildManifest(record.Tests, record.Remediations)
	manifestDigest := evidence.ManifestDigest(record.Manifest)
	record.Batch.Status = StatusFrozen
	record.Batch.Version++
	credentialID := newID("credential")
	digest, err := snapshotDigest(record)
	if err != nil {
		return BatchRecord{}, Credential{}, &DomainError{Code: CodePersistence, Message: err.Error()}
	}
	credential := &Credential{CredentialID: credentialID, BatchID: record.Batch.BatchID, SnapshotDigest: digest, ManifestDigest: manifestDigest, IssuedBy: command.Actor, IssuedAt: s.now().UTC(), Status: "VALID"}
	credential.VerifyText = fmt.Sprintf("SEEDVAULT:%s:%s", credential.CredentialID, credential.SnapshotDigest)
	last := len(record.Reviews) - 1
	record.Reviews[last].SnapshotDigest = digest
	record.Reviews[last].CredentialID = credentialID
	if err := s.commitRecord("batch.frozen", command.Actor, command.IdempotencyKey, record, credential); err != nil {
		return BatchRecord{}, Credential{}, err
	}
	return cloneBatch(record), *credential, nil
}

func (s *Service) VerifyCredential(id, digest string) Verification {
	s.mu.RLock()
	defer s.mu.RUnlock()
	credential, ok := s.state.Credentials[strings.TrimSpace(id)]
	if !ok {
		return Verification{Valid: false, Status: "NOT_FOUND", Message: "未找到该入库凭据"}
	}
	copyCredential := *credential
	if credential.Status == "REVOKED" {
		return Verification{Valid: false, Status: "REVOKED", Message: "凭据已撤销", Credential: &copyCredential}
	}
	if subtleDigest(strings.TrimSpace(digest), credential.SnapshotDigest) == false {
		return Verification{Valid: false, Status: "MISMATCH", Message: "摘要不匹配，凭据内容可能被改动", Credential: &copyCredential}
	}
	return Verification{Valid: true, Status: "VALID", Message: "凭据有效，摘要与冻结快照一致", Credential: &copyCredential}
}

func (s *Service) mutableBatch(id string) (*BatchRecord, error) {
	record, ok := s.state.Batches[strings.TrimSpace(id)]
	if !ok {
		return nil, &DomainError{Code: CodeNotFound, Message: "批次不存在"}
	}
	clone := cloneBatch(record)
	return &clone, nil
}

func (s *Service) idempotentBatch(key, batchID string) (BatchRecord, bool, error) {
	ref, exists := s.state.Idempotency[strings.TrimSpace(key)]
	if !exists {
		return BatchRecord{}, false, nil
	}
	if strings.TrimSpace(batchID) != "" && ref != strings.TrimSpace(batchID) {
		return BatchRecord{}, true, &DomainError{Code: CodeConflict, Message: "幂等键已用于另一个批次"}
	}
	record, ok := s.state.Batches[ref]
	if !ok {
		return BatchRecord{}, true, &DomainError{Code: CodePersistence, Message: "幂等记录引用的批次不存在"}
	}
	return cloneBatch(record), true, nil
}

func (s *Service) commitRecord(eventType, actor, key string, record *BatchRecord, credential *Credential) error {
	payloadRecord := cloneBatch(record)
	payload := eventProjection{Batch: &payloadRecord, Credential: credential, ResultRef: record.Batch.BatchID}
	next := s.cloneProjection()
	next.Batches[record.Batch.BatchID] = &payloadRecord
	if credential != nil {
		copied := *credential
		next.Credentials[copied.CredentialID] = &copied
	}
	next.Idempotency[strings.TrimSpace(key)] = record.Batch.BatchID
	if err := validateProjection(next); err != nil {
		return &DomainError{Code: CodePersistence, Message: "提交前聚合校验失败: " + err.Error()}
	}
	event, err := s.store.Commit(persistence.PendingEvent{EventID: newID("event"), BatchID: record.Batch.BatchID, Type: eventType, Actor: strings.TrimSpace(actor), IdempotencyKey: strings.TrimSpace(key), OccurredAt: s.now().UTC(), Payload: payload}, next)
	if err != nil {
		return &DomainError{Code: CodePersistence, Message: "持久化提交失败: " + err.Error()}
	}
	audit := AuditEntry{Sequence: event.Sequence, EventID: event.EventID, Type: event.Type, Actor: event.Actor, Summary: eventSummary(event.Type), OccurredAt: event.OccurredAt, Hash: event.Hash}
	record.Timeline = append(record.Timeline, audit)
	stored := cloneBatch(record)
	next.Batches[record.Batch.BatchID] = &stored
	if err := s.store.ReplaceSnapshot(next); err != nil {
		_ = s.replay()
		return &DomainError{Code: CodePersistence, Message: "更新审计投影失败: " + err.Error()}
	}
	s.state = next
	return nil
}

func (s *Service) cloneProjection() projection {
	data, _ := json.Marshal(s.state)
	var result projection
	_ = json.Unmarshal(data, &result)
	if result.Batches == nil {
		result = emptyProjection()
	}
	return result
}

func findTest(tests []evidence.TestRun, id string) (evidence.TestRun, bool) {
	for _, test := range tests {
		if test.TestID == strings.TrimSpace(id) {
			return test, true
		}
	}
	return evidence.TestRun{}, false
}

func validateIssueRefs(issues []quality.Issue, refs []string) error {
	available := map[string]bool{}
	for _, issue := range issues {
		available[issue.Code] = true
	}
	for _, ref := range refs {
		if !available[strings.TrimSpace(ref)] {
			return invalid("issue_codes", "问题引用不存在: "+ref)
		}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func snapshotDigest(record *BatchRecord) (string, error) {
	input := struct {
		Batch        SeedBatch               `json:"batch"`
		Tests        []evidence.TestRun      `json:"tests"`
		Remediations []evidence.Remediation  `json:"remediations"`
		Quality      quality.Result          `json:"quality"`
		Reviews      []ReviewDecision        `json:"reviews"`
		Manifest     []evidence.ManifestItem `json:"manifest"`
	}{record.Batch, record.Tests, record.Remediations, record.Quality, record.Reviews, record.Manifest}
	data, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func subtleDigest(actual, expected string) bool {
	if len(actual) != len(expected) {
		return false
	}
	var different byte
	for i := range actual {
		different |= actual[i] ^ expected[i]
	}
	return different == 0
}

func (s *Service) credentialForBatch(batchID string) Credential {
	for _, credential := range s.state.Credentials {
		if credential.BatchID == batchID {
			return *credential
		}
	}
	return Credential{}
}
