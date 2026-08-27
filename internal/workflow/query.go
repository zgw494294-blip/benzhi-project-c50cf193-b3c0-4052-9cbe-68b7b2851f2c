package workflow

import (
	"sort"
	"strings"
	"time"

	"seedvault/internal/evidence"
	"seedvault/internal/quality"
)

const maxBatchPageSize = 100

func (s *Service) QueryBatches(query BatchListQuery) (BatchListResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if query.Page < 1 {
		return BatchListResult{}, invalid("page", "页码必须大于零")
	}
	if query.Page > 1000000 {
		return BatchListResult{}, invalid("page", "页码超出允许范围")
	}
	if query.PageSize < 1 || query.PageSize > maxBatchPageSize {
		return BatchListResult{}, invalid("page_size", "页大小必须在 1 到 100 之间")
	}
	from, to, err := parseHarvestRange(query.HarvestFrom, query.HarvestTo)
	if err != nil {
		return BatchListResult{}, err
	}
	status := query.Status
	if status != "" {
		status = Status(strings.ToUpper(strings.TrimSpace(string(status))))
		if !knownStatus(status) {
			return BatchListResult{}, invalid("status", "未知批次状态")
		}
	}
	items := make([]BatchRecord, 0, len(s.state.Batches))
	counts := make(map[string]int, 7)
	for _, bucket := range []Status{StatusDraft, StatusTesting, StatusRemediationRequired, StatusReadyReview, StatusReviewReturned, StatusReviewApproved, StatusFrozen} {
		counts[string(bucket)] = 0
	}
	for _, record := range s.state.Batches {
		batch := record.Batch
		if query.Species != "" && !strings.EqualFold(strings.TrimSpace(query.Species), batch.SpeciesName) {
			continue
		}
		if query.SourceRegion != "" && !strings.Contains(strings.ToLower(batch.SourceRegion), strings.ToLower(strings.TrimSpace(query.SourceRegion))) {
			continue
		}
		if status != "" && batch.Status != status {
			continue
		}
		harvest, _ := time.Parse("2006-01-02", batch.HarvestDate)
		if from != nil && harvest.Before(*from) {
			continue
		}
		if to != nil && harvest.After(*to) {
			continue
		}
		counts[string(batch.Status)]++
		items = append(items, cloneBatch(record))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Batch.CreatedAt.Equal(items[j].Batch.CreatedAt) {
			return items[i].Batch.BatchID < items[j].Batch.BatchID
		}
		return items[i].Batch.CreatedAt.After(items[j].Batch.CreatedAt)
	})
	total := len(items)
	start := (query.Page - 1) * query.PageSize
	if start > total {
		start = total
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	pageItems := items[start:end]
	return BatchListResult{Batches: pageItems, Items: pageItems, Total: total, Page: query.Page, PageSize: query.PageSize, StatusCounts: counts, PendingCounts: counts}, nil
}

func parseHarvestRange(fromValue, toValue string) (*time.Time, *time.Time, error) {
	var from, to *time.Time
	parse := func(value, field string) (*time.Time, error) {
		if strings.TrimSpace(value) == "" {
			return nil, nil
		}
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
		if err != nil {
			return nil, invalid(field, "日期必须是 YYYY-MM-DD")
		}
		return &parsed, nil
	}
	var err error
	if from, err = parse(fromValue, "harvest_from"); err != nil {
		return nil, nil, err
	}
	if to, err = parse(toValue, "harvest_to"); err != nil {
		return nil, nil, err
	}
	if from != nil && to != nil && from.After(*to) {
		return nil, nil, invalid("harvest_from", "采收日期起点不能晚于终点")
	}
	return from, to, nil
}

func (s *Service) EvidencePreview(batchID string) (EvidencePreview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.state.Batches[strings.TrimSpace(batchID)]
	if !ok {
		return EvidencePreview{}, &DomainError{Code: CodeNotFound, Message: "批次不存在"}
	}
	manifest := evidence.BuildManifest(record.Tests, record.Remediations)
	digest := evidence.ManifestDigest(manifest)
	if record.Batch.Status == StatusFrozen {
		credential := s.credentialForBatch(record.Batch.BatchID)
		if credential.ManifestDigest != digest {
			return EvidencePreview{}, &DomainError{Code: CodePersistence, Message: "冻结凭据的证据清单摘要不一致"}
		}
	}
	comparisons := make([]EvidenceComparison, 0, len(record.Remediations))
	allPassed := true
	for _, remediation := range record.Remediations {
		original, originalOK := findTest(record.Tests, remediation.OriginalTest)
		retest, retestOK := findTest(record.Tests, remediation.RetestID)
		if !originalOK || !retestOK {
			allPassed = false
			continue
		}
		comparison := quality.CompareRetestMetrics(original, retest)
		if !comparison.Passed {
			allPassed = false
		}
		comparisons = append(comparisons, EvidenceComparison{RemediationID: remediation.RemediationID, OriginalTestID: original.TestID, RetestID: retest.TestID, GerminationDelta: comparison.GerminationDelta, PurityDelta: comparison.PurityDelta, MoistureDelta: comparison.MoistureDelta, ContaminationBefore: comparison.ContaminationBefore, ContaminationAfter: comparison.ContaminationAfter, RegressionCodes: comparison.RegressionCodes, Passed: comparison.Passed})
	}
	return EvidencePreview{BatchID: record.Batch.BatchID, Status: record.Batch.Status, Manifest: manifest, ManifestDigest: digest, Comparisons: comparisons, AllowReview: record.Quality.Passed && allPassed && (record.Batch.Status == StatusReadyReview || record.Batch.Status == StatusReviewApproved || record.Batch.Status == StatusFrozen), Blocked: !(record.Quality.Passed && allPassed)}, nil
}
