package quality

import (
	"testing"

	"seedvault/internal/evidence"
)

func TestEvaluateThresholds(t *testing.T) {
	run := evidence.TestRun{TestID: "T-1", MethodCode: "GB/T3543", Replicates: 4, GerminationRate: 86, PurityRate: 99, MoistureRate: 12, EvidenceDigest: evidence.DigestText("e")}
	result := Evaluate("小麦", 400, run)
	if !result.Passed {
		t.Fatalf("合格检测不应被阻断: %#v", result.Issues)
	}
	run.GerminationRate = 70
	run.ContaminationFlag = true
	result = Evaluate("小麦", 300, run)
	if result.Passed || len(result.Issues) < 3 {
		t.Fatalf("不合格检测问题不足: %#v", result.Issues)
	}
}

func TestCompareRetestRejectsRegression(t *testing.T) {
	original := evidence.TestRun{TestID: "T-1", GerminationRate: 90, PurityRate: 99, MoistureRate: 10}
	retest := evidence.TestRun{TestID: "T-2", SupersedesTestID: "T-1", MethodCode: "GB/T3543", Replicates: 4, GerminationRate: 86, PurityRate: 98, MoistureRate: 11, EvidenceDigest: evidence.DigestText("e")}
	result := CompareRetest("小麦", 400, original, retest)
	if result.Passed {
		t.Fatal("关键指标倒退时不应通过")
	}
}
