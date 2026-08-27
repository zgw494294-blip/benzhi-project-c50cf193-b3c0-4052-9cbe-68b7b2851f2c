package workflow

import (
	"errors"
	"fmt"
	"strings"

	"seedvault/internal/evidence"
)

// validateProjection 在加载、重放和提交边界验证跨聚合引用。
func validateProjection(state projection) error {
	if state.Batches == nil || state.Credentials == nil || state.Idempotency == nil {
		return errors.New("工作流投影缺少必要索引")
	}
	credentialByBatch := make(map[string]*Credential, len(state.Credentials))
	for credentialID, credential := range state.Credentials {
		if credential == nil {
			return fmt.Errorf("凭据索引 %s 指向空记录", credentialID)
		}
		if credentialID != credential.CredentialID {
			return fmt.Errorf("凭据索引键 %s 与记录编号 %s 不一致", credentialID, credential.CredentialID)
		}
		if credential.Status != "VALID" && credential.Status != "REVOKED" {
			return fmt.Errorf("凭据 %s 状态 %s 无效", credentialID, credential.Status)
		}
		if credential.Status == "REVOKED" && (strings.TrimSpace(credential.RevokedBy) == "" || strings.TrimSpace(credential.RevocationReason) == "" || credential.RevokedAt == nil || strings.TrimSpace(credential.RevocationKey) == "") {
			return fmt.Errorf("凭据 %s 撤销信息不完整", credentialID)
		}
		if len(credential.SnapshotDigest) != 64 || len(credential.ManifestDigest) != 64 {
			return fmt.Errorf("凭据 %s 的冻结摘要不完整", credentialID)
		}
		if existing := credentialByBatch[credential.BatchID]; existing != nil {
			return fmt.Errorf("批次 %s 存在多张入库凭据", credential.BatchID)
		}
		credentialByBatch[credential.BatchID] = credential
	}
	for batchID, record := range state.Batches {
		if record == nil {
			return fmt.Errorf("批次索引 %s 指向空记录", batchID)
		}
		if batchID != record.Batch.BatchID {
			return fmt.Errorf("批次索引键 %s 与聚合编号 %s 不一致", batchID, record.Batch.BatchID)
		}
		if err := validateRecord(record); err != nil {
			return fmt.Errorf("批次 %s 不变量失败: %w", batchID, err)
		}
		credential := credentialByBatch[batchID]
		if record.Batch.Status == StatusFrozen && credential == nil {
			return errors.New("已冻结批次缺少入库凭据")
		}
		if record.Batch.Status != StatusFrozen && credential != nil {
			return errors.New("未冻结批次不能持有入库凭据")
		}
		if credential != nil {
			lastReview := record.Reviews[len(record.Reviews)-1]
			if lastReview.CredentialID != credential.CredentialID || lastReview.SnapshotDigest != credential.SnapshotDigest {
				return errors.New("复核决定与签发凭据摘要不一致")
			}
		}
	}
	for key, batchID := range state.Idempotency {
		if strings.TrimSpace(key) == "" {
			return errors.New("投影包含空幂等键")
		}
		if _, exists := state.Batches[batchID]; !exists {
			return fmt.Errorf("幂等键 %s 引用了不存在的批次 %s", key, batchID)
		}
	}
	return nil
}

func validateRecord(record *BatchRecord) error {
	batch := record.Batch
	if strings.TrimSpace(batch.BatchID) == "" || strings.TrimSpace(batch.SpeciesName) == "" {
		return errors.New("批次身份字段不完整")
	}
	if batch.Version < 1 {
		return errors.New("批次版本必须为正数")
	}
	if batch.SampleCount < 1 || strings.TrimSpace(batch.CreatedBy) == "" || batch.CreatedAt.IsZero() {
		return errors.New("批次建档字段不完整")
	}
	if !knownStatus(batch.Status) {
		return fmt.Errorf("未知状态 %s", batch.Status)
	}
	if err := validateTests(record); err != nil {
		return err
	}
	if err := validateRemediations(record); err != nil {
		return err
	}
	if err := validateReviews(record); err != nil {
		return err
	}
	if err := validateTimeline(record.Timeline); err != nil {
		return err
	}
	return validateStatusContent(record)
}

func knownStatus(status Status) bool {
	switch status {
	case StatusDraft, StatusTesting, StatusRemediationRequired, StatusReadyReview,
		StatusReviewReturned, StatusReviewApproved, StatusFrozen:
		return true
	default:
		return false
	}
}

func validateTests(record *BatchRecord) error {
	seen := make(map[string]bool, len(record.Tests))
	for index, test := range record.Tests {
		if test.BatchID != record.Batch.BatchID {
			return fmt.Errorf("检测 %s 属于其他批次", test.TestID)
		}
		if strings.TrimSpace(test.TestID) == "" || seen[test.TestID] {
			return fmt.Errorf("检测编号为空或重复: %s", test.TestID)
		}
		if len(test.EvidenceDigest) != 64 || strings.TrimSpace(test.Operator) == "" || test.ObservedAt.IsZero() {
			return fmt.Errorf("检测 %s 证据字段不完整", test.TestID)
		}
		if test.SupersedesTestID != "" {
			originalIndex := -1
			for candidate := 0; candidate < index; candidate++ {
				if record.Tests[candidate].TestID == test.SupersedesTestID {
					originalIndex = candidate
					break
				}
			}
			if originalIndex < 0 {
				return fmt.Errorf("复测 %s 未引用更早的原测", test.TestID)
			}
			if !test.ObservedAt.After(record.Tests[originalIndex].ObservedAt) {
				return fmt.Errorf("复测 %s 的观察时间不晚于原测", test.TestID)
			}
		}
		seen[test.TestID] = true
	}
	return nil
}

func validateRemediations(record *BatchRecord) error {
	tests := make(map[string]evidence.TestRun, len(record.Tests))
	for _, test := range record.Tests {
		tests[test.TestID] = test
	}
	seen := make(map[string]bool, len(record.Remediations))
	for _, remediation := range record.Remediations {
		if remediation.BatchID != record.Batch.BatchID || strings.TrimSpace(remediation.RemediationID) == "" {
			return errors.New("整改记录身份字段不一致")
		}
		if seen[remediation.RemediationID] {
			return fmt.Errorf("整改编号 %s 重复", remediation.RemediationID)
		}
		original, originalOK := tests[remediation.OriginalTest]
		retest, retestOK := tests[remediation.RetestID]
		if !originalOK || !retestOK || retest.SupersedesTestID != original.TestID {
			return fmt.Errorf("整改 %s 的检测关联不完整", remediation.RemediationID)
		}
		if len(remediation.IssueCodes) == 0 || strings.TrimSpace(remediation.Explanation) == "" {
			return fmt.Errorf("整改 %s 缺少问题引用或说明", remediation.RemediationID)
		}
		seen[remediation.RemediationID] = true
	}
	return nil
}

func validateReviews(record *BatchRecord) error {
	seen := make(map[string]bool, len(record.Reviews))
	for index, review := range record.Reviews {
		if review.BatchID != record.Batch.BatchID || strings.TrimSpace(review.Reviewer) == "" {
			return errors.New("复核记录身份字段不完整")
		}
		if strings.TrimSpace(review.ReviewID) == "" || seen[review.ReviewID] {
			return fmt.Errorf("复核编号为空或重复: %s", review.ReviewID)
		}
		if review.Decision != "APPROVE" && review.Decision != "RETURN" {
			return fmt.Errorf("复核 %s 决定无效", review.ReviewID)
		}
		if review.Reviewer == record.Batch.CreatedBy {
			return fmt.Errorf("复核 %s 未满足独立性", review.ReviewID)
		}
		for _, test := range record.Tests {
			if review.Reviewer == test.Operator {
				return fmt.Errorf("复核员 %s 参与过检测", review.Reviewer)
			}
		}
		if review.CredentialID != "" && index != len(record.Reviews)-1 {
			return errors.New("只有最终复核决定可以关联入库凭据")
		}
		seen[review.ReviewID] = true
	}
	return nil
}

func validateTimeline(timeline []AuditEntry) error {
	var sequence uint64
	seen := make(map[string]bool, len(timeline))
	for _, entry := range timeline {
		if entry.Sequence <= sequence {
			return errors.New("批次审计序号未严格递增")
		}
		if entry.EventID == "" || seen[entry.EventID] || len(entry.Hash) != 64 {
			return errors.New("批次审计事件身份或摘要无效")
		}
		sequence = entry.Sequence
		seen[entry.EventID] = true
	}
	return nil
}

func validateStatusContent(record *BatchRecord) error {
	switch record.Batch.Status {
	case StatusDraft:
		if len(record.Tests) != 0 {
			return errors.New("草稿批次不能已有检测")
		}
	case StatusRemediationRequired:
		if len(record.Tests) == 0 || record.Quality.Passed || len(record.Quality.Issues) == 0 {
			return errors.New("待整改状态必须有未通过的质量问题")
		}
	case StatusReadyReview:
		if len(record.Tests) == 0 || !record.Quality.Passed {
			return errors.New("待复核状态必须已有通过的质量结论")
		}
	case StatusReviewReturned:
		if len(record.Reviews) == 0 || record.Reviews[len(record.Reviews)-1].Decision != "RETURN" {
			return errors.New("复核退回状态缺少退回决定")
		}
	case StatusReviewApproved:
		if len(record.Reviews) == 0 || record.Reviews[len(record.Reviews)-1].Decision != "APPROVE" || !record.Quality.Passed {
			return errors.New("复核通过状态缺少通过决定或合格结论")
		}
	case StatusFrozen:
		if len(record.Manifest) == 0 || len(record.Reviews) == 0 || record.Reviews[len(record.Reviews)-1].CredentialID == "" {
			return errors.New("冻结状态缺少证据清单或签发信息")
		}
	}
	return nil
}
