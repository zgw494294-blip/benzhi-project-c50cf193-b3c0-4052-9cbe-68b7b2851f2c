package evidence

import "time"

// TestRun 是一次完整的批次检测观察。四类指标放在同一记录中，确保它们共享样本与证据上下文。
type TestRun struct {
	TestID            string    `json:"test_id"`
	BatchID           string    `json:"batch_id"`
	MethodCode        string    `json:"method_code"`
	Replicates        int       `json:"replicates"`
	GerminationRate   float64   `json:"germination_rate"`
	PurityRate        float64   `json:"purity_rate"`
	MoistureRate      float64   `json:"moisture_rate"`
	ContaminationFlag bool      `json:"contamination_flag"`
	ContaminationNote string    `json:"contamination_note,omitempty"`
	EvidenceDigest    string    `json:"evidence_digest"`
	Operator          string    `json:"operator"`
	ObservedAt        time.Time `json:"observed_at"`
	SupersedesTestID  string    `json:"supersedes_test_id,omitempty"`
}

// TestInput 是未规范化的录入值。
type TestInput struct {
	MethodCode        string    `json:"method_code"`
	Replicates        int       `json:"replicates"`
	GerminationRate   float64   `json:"germination_rate"`
	PurityRate        float64   `json:"purity_rate"`
	MoistureRate      float64   `json:"moisture_rate"`
	ContaminationFlag bool      `json:"contamination_flag"`
	ContaminationNote string    `json:"contamination_note"`
	EvidenceDigest    string    `json:"evidence_digest"`
	Operator          string    `json:"operator"`
	ObservedAt        time.Time `json:"observed_at"`
	SupersedesTestID  string    `json:"supersedes_test_id"`
}

// Remediation 把问题说明、原测和替代复测绑定在一起。
type Remediation struct {
	RemediationID string    `json:"remediation_id"`
	BatchID       string    `json:"batch_id"`
	IssueCodes    []string  `json:"issue_codes"`
	Explanation   string    `json:"explanation"`
	OriginalTest  string    `json:"original_test_id"`
	RetestID      string    `json:"retest_id"`
	SubmittedBy   string    `json:"submitted_by"`
	SubmittedAt   time.Time `json:"submitted_at"`
}

// ManifestItem 是冻结前交给复核员的稳定证据索引。
type ManifestItem struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Digest    string `json:"digest"`
	Relation  string `json:"relation,omitempty"`
	Summary   string `json:"summary"`
	Timestamp string `json:"timestamp"`
}
