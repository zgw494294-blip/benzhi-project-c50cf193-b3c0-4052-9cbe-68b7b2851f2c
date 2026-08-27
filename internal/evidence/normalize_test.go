package evidence

import (
	"testing"
	"time"
)

func TestNormalizeAndLinkRetest(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	input := TestInput{MethodCode: " gb/t3543 ", Replicates: 4, GerminationRate: 90.123, PurityRate: 99.2, MoistureRate: 11, EvidenceDigest: DigestText("证据"), Operator: "检测员", ObservedAt: now.Add(-time.Hour)}
	original, err := NormalizeTest("B-1", "T-1", input, now)
	if err != nil {
		t.Fatal(err)
	}
	if original.MethodCode != "GB/T3543" || original.GerminationRate != 90.12 {
		t.Fatalf("规范化结果错误: %#v", original)
	}
	input.SupersedesTestID = "T-1"
	input.ObservedAt = now
	retest, err := NormalizeTest("B-1", "T-2", input, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLink(retest, []TestRun{original}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLink(retest, []TestRun{original, retest}); err == nil {
		t.Fatal("应拒绝重复检测编号")
	}
}

func TestNormalizeRejectsInvalidDigest(t *testing.T) {
	_, err := NormalizeTest("B", "T", TestInput{MethodCode: "ISTA", Replicates: 4, GerminationRate: 90, PurityRate: 99, MoistureRate: 10, EvidenceDigest: "not-a-digest", Operator: "x"}, time.Now())
	if err == nil {
		t.Fatal("应拒绝无效证据摘要")
	}
}
