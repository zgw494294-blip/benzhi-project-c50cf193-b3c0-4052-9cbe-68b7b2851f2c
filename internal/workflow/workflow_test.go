package workflow

import (
	"testing"
	"time"

	"seedvault/internal/evidence"
	"seedvault/internal/persistence"
)

func newTestService(t *testing.T) (*Service, *persistence.Store) {
	t.Helper()
	store, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC) }
	return service, store
}

func createTestBatch(t *testing.T, service *Service) BatchRecord {
	t.Helper()
	record, err := service.CreateBatch(CreateBatchCommand{BatchID: "B-001", SpeciesName: "小麦", SourceRegion: "甘肃", HarvestDate: "2026-08-20", SampleCount: 400, StorageCondition: "低温干燥", Actor: "接收员", Role: "receiver", IdempotencyKey: "create-key-001"})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestWorkflowEndToEndAndRecovery(t *testing.T) {
	service, store := newTestService(t)
	record := createTestBatch(t, service)
	bad := evidence.TestInput{MethodCode: "GB/T3543", Replicates: 4, GerminationRate: 70, PurityRate: 96, MoistureRate: 14, EvidenceDigest: evidence.DigestText("原测"), ObservedAt: time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)}
	record, err := service.RecordTest(RecordTestCommand{BatchID: "B-001", TestID: "T-1", ExpectedVersion: record.Batch.Version, Actor: "检测员", Role: "tester", IdempotencyKey: "test-key-0001", Test: bad})
	if err != nil {
		t.Fatal(err)
	}
	if record.Batch.Status != StatusRemediationRequired {
		t.Fatalf("状态错误: %s", record.Batch.Status)
	}
	codes := make([]string, len(record.Quality.Issues))
	for i, issue := range record.Quality.Issues {
		codes[i] = issue.Code
	}
	good := evidence.TestInput{MethodCode: "GB/T3543", Replicates: 4, GerminationRate: 90, PurityRate: 99, MoistureRate: 11, EvidenceDigest: evidence.DigestText("复测")}
	record, err = service.SubmitRemediation(RemediateCommand{BatchID: "B-001", OriginalTestID: "T-1", RetestID: "T-2", IssueCodes: codes, Explanation: "重新清选并平衡含水率", ExpectedVersion: record.Batch.Version, Actor: "接收员", Role: "receiver", IdempotencyKey: "fix-key-00001", Retest: good})
	if err != nil {
		t.Fatal(err)
	}
	if record.Batch.Status != StatusReadyReview {
		t.Fatalf("整改后状态错误: %s %#v", record.Batch.Status, record.Quality.Issues)
	}
	record, err = service.Review(ReviewCommand{BatchID: "B-001", Decision: "APPROVE", Comment: "证据完整", ExpectedVersion: record.Batch.Version, Actor: "复核员", Role: "reviewer", IdempotencyKey: "review-key-01"})
	if err != nil {
		t.Fatal(err)
	}
	record, credential, err := service.Freeze(FreezeCommand{BatchID: "B-001", ExpectedVersion: record.Batch.Version, Actor: "管理员", Role: "administrator", IdempotencyKey: "freeze-key-01"})
	if err != nil {
		t.Fatal(err)
	}
	if record.Batch.Status != StatusFrozen || !service.VerifyCredential(credential.CredentialID, credential.SnapshotDigest).Valid {
		t.Fatal("冻结凭据无效")
	}
	recovered, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := recovered.GetBatch("B-001")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Batch.Status != StatusFrozen || len(reloaded.Timeline) != 5 {
		t.Fatalf("恢复投影错误: %#v", reloaded)
	}
}

func TestVersionConflictAndIndependentReviewer(t *testing.T) {
	service, _ := newTestService(t)
	record := createTestBatch(t, service)
	good := evidence.TestInput{MethodCode: "GB/T3543", Replicates: 4, GerminationRate: 90, PurityRate: 99, MoistureRate: 11, EvidenceDigest: evidence.DigestText("证据")}
	_, err := service.RecordTest(RecordTestCommand{BatchID: "B-001", TestID: "T", ExpectedVersion: 99, Actor: "检测员", Role: "tester", IdempotencyKey: "conflict-key", Test: good})
	if err == nil {
		t.Fatal("应报告版本冲突")
	}
	record, err = service.RecordTest(RecordTestCommand{BatchID: "B-001", TestID: "T", ExpectedVersion: record.Batch.Version, Actor: "检测员", Role: "tester", IdempotencyKey: "valid-key-001", Test: good})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Review(ReviewCommand{BatchID: "B-001", Decision: "APPROVE", ExpectedVersion: record.Batch.Version, Actor: "接收员", Role: "reviewer", IdempotencyKey: "review-bad-01"})
	if err == nil {
		t.Fatal("创建人不应复核自己的批次")
	}
}

func TestEvidencePreviewAndCredentialRevocation(t *testing.T) {
	service, store := newTestService(t)
	record := createTestBatch(t, service)
	good := evidence.TestInput{MethodCode: "GB/T3543", Replicates: 4, GerminationRate: 90, PurityRate: 99, MoistureRate: 11, EvidenceDigest: evidence.DigestText("证据")}
	var err error
	record, err = service.RecordTest(RecordTestCommand{BatchID: "B-001", TestID: "T-1", ExpectedVersion: record.Batch.Version, Actor: "检测员", Role: "tester", IdempotencyKey: "preview-test", Test: good})
	if err != nil {
		t.Fatal(err)
	}
	record, err = service.Review(ReviewCommand{BatchID: "B-001", Decision: "APPROVE", ExpectedVersion: record.Batch.Version, Actor: "复核员", Role: "reviewer", IdempotencyKey: "preview-review"})
	if err != nil {
		t.Fatal(err)
	}
	record, credential, err := service.Freeze(FreezeCommand{BatchID: "B-001", ExpectedVersion: record.Batch.Version, Actor: "管理员", Role: "administrator", IdempotencyKey: "preview-freeze"})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.EvidencePreview("B-001")
	if err != nil || preview.ManifestDigest != credential.ManifestDigest || !preview.AllowReview {
		t.Fatalf("预览不一致: %#v %v", preview, err)
	}
	version := record.Batch.Version
	revoked, err := service.RevokeCredential(RevokeCredentialCommand{CredentialID: credential.CredentialID, Reason: "证据复核失效", Actor: "管理员", Role: "administrator", IdempotencyKey: "revoke-key-001"})
	if err != nil || revoked.Status != "REVOKED" {
		t.Fatalf("撤销失败: %#v %v", revoked, err)
	}
	retried, err := service.RevokeCredential(RevokeCredentialCommand{CredentialID: credential.CredentialID, Reason: "其他原因", Actor: "管理员", Role: "administrator", IdempotencyKey: "revoke-key-001"})
	if err != nil || retried.RevocationReason != "证据复核失效" {
		t.Fatalf("幂等重试失败: %#v %v", retried, err)
	}
	current, _ := service.GetBatch("B-001")
	if current.Batch.Version != version {
		t.Fatalf("撤销不应改变批次版本: %d", current.Batch.Version)
	}
	recovered, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	verification := recovered.VerifyCredential(credential.CredentialID, credential.SnapshotDigest)
	if verification.Valid || verification.Status != "REVOKED" || verification.Credential.RevokedBy != "管理员" {
		t.Fatalf("重放后的撤销状态错误: %#v", verification)
	}
}
