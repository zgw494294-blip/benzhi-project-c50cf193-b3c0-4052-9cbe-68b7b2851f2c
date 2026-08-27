package quality

import (
	"fmt"
	"sort"
	"strings"

	"seedvault/internal/evidence"
)

// Issue 是稳定且可定位的问题项。
type Issue struct {
	Code        string `json:"code"`
	Field       string `json:"field"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
	TestID      string `json:"test_id,omitempty"`
}

// Check 是一条可显示的通过项或问题项。
type Check struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
	TestID  string `json:"test_id,omitempty"`
}

// Result 是某一时点的确定性规则结论。
type Result struct {
	ProfileCode string  `json:"profile_code"`
	Passed      bool    `json:"passed"`
	Checks      []Check `json:"checks"`
	Issues      []Issue `json:"issues"`
}

// Evaluate 检查样本量、检测方案、阈值、污染与证据。
func Evaluate(species string, sampleCount int, run evidence.TestRun) Result {
	profile, found := ProfileForSpecies(species)
	if !found {
		issue := Issue{Code: "PROFILE_NOT_FOUND", Field: "species_name", Message: "当前物种没有可执行的检测方案", Remediation: "请选择已配置物种或补充经批准的物种方案", TestID: run.TestID}
		return Result{Passed: false, Checks: []Check{{Code: issue.Code, Field: issue.Field, Passed: false, Message: issue.Message, TestID: run.TestID}}, Issues: []Issue{issue}}
	}
	builder := resultBuilder{profileCode: profile.Code, testID: run.TestID}
	builder.minimum("SAMPLE_COUNT_LOW", "sample_count", sampleCount, profile.MinimumSamples, "分装样本数满足方案", "增加具有代表性的分装样本")
	builder.minimum("REPLICATES_LOW", "replicates", run.Replicates, profile.MinimumReplicates, "检测重复次数满足方案", "按方案补足独立重复检测")
	builder.allowed("METHOD_UNSUPPORTED", "method_code", run.MethodCode, profile.AllowedMethods, "检测方法在方案允许范围内", "使用方案列出的标准检测方法")
	builder.rateMinimum("GERMINATION_LOW", "germination_rate", run.GerminationRate, profile.MinimumGermination, "活力达到最低阈值", "改善样本处理并提交替代复测")
	builder.rateMinimum("PURITY_LOW", "purity_rate", run.PurityRate, profile.MinimumPurity, "纯度达到最低阈值", "重新清选样本并提交替代复测")
	builder.rateMaximum("MOISTURE_HIGH", "moisture_rate", run.MoistureRate, profile.MaximumMoisture, "含水率未超过上限", "按规范干燥平衡后提交替代复测")
	builder.boolean("CONTAMINATION_FOUND", "contamination_flag", !run.ContaminationFlag, "未观察到污染", "隔离受污染样本并重新取样检测")
	digestOK := len(run.EvidenceDigest) == 64 && strings.ToLower(run.EvidenceDigest) == run.EvidenceDigest
	builder.boolean("EVIDENCE_INVALID", "evidence_digest", digestOK, "证据摘要格式完整", "重新上传证据并记录其 SHA-256 摘要")
	return builder.finish()
}

type resultBuilder struct {
	profileCode string
	testID      string
	checks      []Check
	issues      []Issue
}

func (b *resultBuilder) add(code, field string, passed bool, okMessage, fix string) {
	message := okMessage
	if !passed {
		message = fix
		b.issues = append(b.issues, Issue{Code: code, Field: field, Message: message, Remediation: fix, TestID: b.testID})
	}
	b.checks = append(b.checks, Check{Code: code, Field: field, Passed: passed, Message: message, TestID: b.testID})
}

func (b *resultBuilder) minimum(code, field string, actual, minimum int, ok, fix string) {
	b.add(code, field, actual >= minimum, fmt.Sprintf("%s（%d ≥ %d）", ok, actual, minimum), fmt.Sprintf("%s；当前 %d，至少 %d", fix, actual, minimum))
}

func (b *resultBuilder) rateMinimum(code, field string, actual, minimum float64, ok, fix string) {
	b.add(code, field, actual >= minimum, fmt.Sprintf("%s（%.2f%% ≥ %.2f%%）", ok, actual, minimum), fmt.Sprintf("%s；当前 %.2f%%，最低 %.2f%%", fix, actual, minimum))
}

func (b *resultBuilder) rateMaximum(code, field string, actual, maximum float64, ok, fix string) {
	b.add(code, field, actual <= maximum, fmt.Sprintf("%s（%.2f%% ≤ %.2f%%）", ok, actual, maximum), fmt.Sprintf("%s；当前 %.2f%%，上限 %.2f%%", fix, actual, maximum))
}

func (b *resultBuilder) allowed(code, field, actual string, values []string, ok, fix string) {
	passed := false
	for _, value := range values {
		if strings.EqualFold(actual, value) {
			passed = true
			break
		}
	}
	b.add(code, field, passed, ok, fix+"；允许值 "+strings.Join(values, ", "))
}

func (b *resultBuilder) boolean(code, field string, passed bool, ok, fix string) {
	b.add(code, field, passed, ok, fix)
}

func (b *resultBuilder) finish() Result {
	sort.SliceStable(b.issues, func(i, j int) bool {
		if b.issues[i].Code == b.issues[j].Code {
			return b.issues[i].Field < b.issues[j].Field
		}
		return b.issues[i].Code < b.issues[j].Code
	})
	return Result{ProfileCode: b.profileCode, Passed: len(b.issues) == 0, Checks: b.checks, Issues: b.issues}
}
