package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// NormalizeTest 校验并规范化检测输入，不悄悄修正超出物理范围的数据。
func NormalizeTest(batchID, testID string, in TestInput, now time.Time) (TestRun, error) {
	batchID = strings.TrimSpace(batchID)
	testID = strings.TrimSpace(testID)
	if batchID == "" || testID == "" {
		return TestRun{}, errors.New("batch_id 和 test_id 不能为空")
	}
	method := strings.ToUpper(strings.TrimSpace(in.MethodCode))
	if method == "" {
		return TestRun{}, errors.New("method_code 不能为空")
	}
	operator := strings.TrimSpace(in.Operator)
	if operator == "" {
		return TestRun{}, errors.New("operator 不能为空")
	}
	if in.Replicates < 1 || in.Replicates > 100 {
		return TestRun{}, errors.New("replicates 必须在 1 到 100 之间")
	}
	values := []struct {
		name  string
		value float64
	}{
		{"germination_rate", in.GerminationRate},
		{"purity_rate", in.PurityRate},
		{"moisture_rate", in.MoistureRate},
	}
	for _, item := range values {
		if math.IsNaN(item.value) || math.IsInf(item.value, 0) || item.value < 0 || item.value > 100 {
			return TestRun{}, fmt.Errorf("%s 必须是 0 到 100 的有限数值", item.name)
		}
	}
	digest := strings.ToLower(strings.TrimSpace(in.EvidenceDigest))
	if !digestPattern.MatchString(digest) {
		return TestRun{}, errors.New("evidence_digest 必须是 64 位小写 SHA-256")
	}
	note := strings.TrimSpace(in.ContaminationNote)
	if in.ContaminationFlag && note == "" {
		return TestRun{}, errors.New("存在污染时必须填写 contamination_note")
	}
	observed := in.ObservedAt.UTC()
	if observed.IsZero() {
		observed = now.UTC()
	}
	if observed.After(now.Add(5 * time.Minute)) {
		return TestRun{}, errors.New("observed_at 不能晚于当前时间")
	}
	return TestRun{
		TestID: testID, BatchID: batchID, MethodCode: method,
		Replicates: in.Replicates, GerminationRate: round(in.GerminationRate),
		PurityRate: round(in.PurityRate), MoistureRate: round(in.MoistureRate),
		ContaminationFlag: in.ContaminationFlag, ContaminationNote: note,
		EvidenceDigest: digest, Operator: operator, ObservedAt: observed,
		SupersedesTestID: strings.TrimSpace(in.SupersedesTestID),
	}, nil
}

func round(value float64) float64 { return math.Round(value*100) / 100 }

// ValidateLink 阻止孤立、自引用、重复替代和跨批次复测。
func ValidateLink(candidate TestRun, existing []TestRun) error {
	seenID := make(map[string]TestRun, len(existing))
	superseded := make(map[string]bool, len(existing))
	for _, run := range existing {
		if _, duplicate := seenID[run.TestID]; duplicate {
			return fmt.Errorf("已有重复检测编号 %s", run.TestID)
		}
		seenID[run.TestID] = run
		if run.SupersedesTestID != "" {
			superseded[run.SupersedesTestID] = true
		}
	}
	if _, duplicate := seenID[candidate.TestID]; duplicate {
		return fmt.Errorf("检测编号 %s 已存在", candidate.TestID)
	}
	if candidate.SupersedesTestID == "" {
		return nil
	}
	if candidate.SupersedesTestID == candidate.TestID {
		return errors.New("替代复测不能引用自身")
	}
	original, ok := seenID[candidate.SupersedesTestID]
	if !ok {
		return errors.New("supersedes_test_id 指向的原测不存在")
	}
	if original.BatchID != candidate.BatchID {
		return errors.New("替代复测不能跨批次关联")
	}
	if superseded[original.TestID] {
		return errors.New("同一原测只能有一条替代复测")
	}
	if !candidate.ObservedAt.After(original.ObservedAt) {
		return errors.New("替代复测时间必须晚于原测")
	}
	return nil
}

// DigestText 生成前端示例与领域清单使用的稳定摘要。
func DigestText(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}
