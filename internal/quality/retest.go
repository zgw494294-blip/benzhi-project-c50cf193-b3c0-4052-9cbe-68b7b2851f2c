package quality

import (
	"fmt"
	"sort"

	"seedvault/internal/evidence"
)

// RetestComparison exposes the metric deltas and deterministic regression codes
// used by the freeze preview and audit views.
type RetestComparison struct {
	GerminationDelta    float64
	PurityDelta         float64
	MoistureDelta       float64
	ContaminationBefore bool
	ContaminationAfter  bool
	RegressionCodes     []string
	Passed              bool
}

func CompareRetestMetrics(original, replacement evidence.TestRun) RetestComparison {
	result := RetestComparison{
		GerminationDelta:    replacement.GerminationRate - original.GerminationRate,
		PurityDelta:         replacement.PurityRate - original.PurityRate,
		MoistureDelta:       replacement.MoistureRate - original.MoistureRate,
		ContaminationBefore: original.ContaminationFlag,
		ContaminationAfter:  replacement.ContaminationFlag,
		Passed:              true,
	}
	if replacement.SupersedesTestID != original.TestID {
		result.RegressionCodes = append(result.RegressionCodes, "RETEST_LINK_INVALID")
	}
	if replacement.GerminationRate < original.GerminationRate {
		result.RegressionCodes = append(result.RegressionCodes, "RETEST_GERMINATION_REGRESSED")
	}
	if replacement.PurityRate < original.PurityRate {
		result.RegressionCodes = append(result.RegressionCodes, "RETEST_PURITY_REGRESSED")
	}
	if replacement.MoistureRate > original.MoistureRate {
		result.RegressionCodes = append(result.RegressionCodes, "RETEST_MOISTURE_REGRESSED")
	}
	if replacement.ContaminationFlag {
		result.RegressionCodes = append(result.RegressionCodes, "RETEST_CONTAMINATION_REMAINS")
	}
	sort.Strings(result.RegressionCodes)
	result.Passed = len(result.RegressionCodes) == 0
	return result
}

// CompareRetest 先执行完整规则，再确认替代记录确实引用原测且没有让关键指标倒退。
func CompareRetest(species string, sampleCount int, original, replacement evidence.TestRun) Result {
	result := Evaluate(species, sampleCount, replacement)
	appendIssue := func(code, field, message string) {
		result.Passed = false
		result.Checks = append(result.Checks, Check{Code: code, Field: field, Passed: false, Message: message, TestID: replacement.TestID})
		result.Issues = append(result.Issues, Issue{Code: code, Field: field, Message: message, Remediation: message, TestID: replacement.TestID})
	}
	if replacement.SupersedesTestID != original.TestID {
		appendIssue("RETEST_LINK_INVALID", "supersedes_test_id", "替代复测未关联指定原测")
	}
	if replacement.GerminationRate < original.GerminationRate {
		appendIssue("RETEST_GERMINATION_REGRESSED", "germination_rate", fmt.Sprintf("复测活力 %.2f%% 低于原测 %.2f%%", replacement.GerminationRate, original.GerminationRate))
	}
	if replacement.PurityRate < original.PurityRate {
		appendIssue("RETEST_PURITY_REGRESSED", "purity_rate", fmt.Sprintf("复测纯度 %.2f%% 低于原测 %.2f%%", replacement.PurityRate, original.PurityRate))
	}
	if replacement.MoistureRate > original.MoistureRate {
		appendIssue("RETEST_MOISTURE_REGRESSED", "moisture_rate", fmt.Sprintf("复测含水率 %.2f%% 高于原测 %.2f%%", replacement.MoistureRate, original.MoistureRate))
	}
	if replacement.ContaminationFlag {
		appendIssue("RETEST_CONTAMINATION_REMAINS", "contamination_flag", "替代复测仍观察到污染")
	}
	return result
}
