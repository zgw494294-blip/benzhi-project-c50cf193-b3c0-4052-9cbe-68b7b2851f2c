package workflow

import (
	"time"

	"seedvault/internal/evidence"
	"seedvault/internal/quality"
)

type Status string

const (
	StatusDraft               Status = "DRAFT"
	StatusTesting             Status = "TESTING"
	StatusRemediationRequired Status = "REMEDIATION_REQUIRED"
	StatusReadyReview         Status = "READY_REVIEW"
	StatusReviewReturned      Status = "REVIEW_RETURNED"
	StatusReviewApproved      Status = "REVIEW_APPROVED"
	StatusFrozen              Status = "FROZEN"
)

// SeedBatch 是整条核验流程的聚合根。
type SeedBatch struct {
	BatchID          string    `json:"batch_id"`
	SpeciesName      string    `json:"species_name"`
	SourceRegion     string    `json:"source_region"`
	HarvestDate      string    `json:"harvest_date"`
	SampleCount      int       `json:"sample_count"`
	StorageCondition string    `json:"storage_condition"`
	Status           Status    `json:"status"`
	Version          int       `json:"version"`
	CreatedAt        time.Time `json:"created_at"`
	CreatedBy        string    `json:"created_by"`
}

type ReviewDecision struct {
	ReviewID       string    `json:"review_id"`
	BatchID        string    `json:"batch_id"`
	Reviewer       string    `json:"reviewer"`
	Decision       string    `json:"decision"`
	IssueRefs      []string  `json:"issue_refs"`
	Comment        string    `json:"comment"`
	SnapshotDigest string    `json:"snapshot_digest,omitempty"`
	CredentialID   string    `json:"credential_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type AuditEntry struct {
	Sequence   uint64    `json:"sequence"`
	EventID    string    `json:"event_id"`
	Type       string    `json:"type"`
	Actor      string    `json:"actor"`
	Summary    string    `json:"summary"`
	OccurredAt time.Time `json:"occurred_at"`
	Hash       string    `json:"hash"`
}

// BatchRecord 是可查询的完整批次投影。
type BatchRecord struct {
	Batch        SeedBatch               `json:"batch"`
	Tests        []evidence.TestRun      `json:"tests"`
	Remediations []evidence.Remediation  `json:"remediations"`
	Quality      quality.Result          `json:"quality"`
	Reviews      []ReviewDecision        `json:"reviews"`
	Manifest     []evidence.ManifestItem `json:"manifest,omitempty"`
	Timeline     []AuditEntry            `json:"timeline"`
}

type Credential struct {
	CredentialID     string     `json:"credential_id"`
	BatchID          string     `json:"batch_id"`
	SnapshotDigest   string     `json:"snapshot_digest"`
	ManifestDigest   string     `json:"manifest_digest"`
	IssuedBy         string     `json:"issued_by"`
	IssuedAt         time.Time  `json:"issued_at"`
	Status           string     `json:"status"`
	VerifyText       string     `json:"verify_text"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevokedBy        string     `json:"revoked_by,omitempty"`
	RevocationReason string     `json:"revocation_reason,omitempty"`
	RevocationKey    string     `json:"revocation_key,omitempty"`
}

type Verification struct {
	Valid      bool        `json:"valid"`
	Status     string      `json:"status"`
	Message    string      `json:"message"`
	Credential *Credential `json:"credential,omitempty"`
}

type CreateBatchCommand struct {
	BatchID          string `json:"batch_id"`
	SpeciesName      string `json:"species_name"`
	SourceRegion     string `json:"source_region"`
	HarvestDate      string `json:"harvest_date"`
	SampleCount      int    `json:"sample_count"`
	StorageCondition string `json:"storage_condition"`
	Actor            string `json:"actor"`
	Role             string `json:"role"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type RecordTestCommand struct {
	BatchID         string             `json:"batch_id"`
	TestID          string             `json:"test_id"`
	ExpectedVersion int                `json:"expected_version"`
	Actor           string             `json:"actor"`
	Role            string             `json:"role"`
	IdempotencyKey  string             `json:"idempotency_key"`
	Test            evidence.TestInput `json:"test"`
}

type RemediateCommand struct {
	BatchID         string             `json:"batch_id"`
	RemediationID   string             `json:"remediation_id"`
	OriginalTestID  string             `json:"original_test_id"`
	RetestID        string             `json:"retest_id"`
	IssueCodes      []string           `json:"issue_codes"`
	Explanation     string             `json:"explanation"`
	ExpectedVersion int                `json:"expected_version"`
	Actor           string             `json:"actor"`
	Role            string             `json:"role"`
	IdempotencyKey  string             `json:"idempotency_key"`
	Retest          evidence.TestInput `json:"retest"`
}

type ReviewCommand struct {
	BatchID         string   `json:"batch_id"`
	ReviewID        string   `json:"review_id"`
	Decision        string   `json:"decision"`
	IssueRefs       []string `json:"issue_refs"`
	Comment         string   `json:"comment"`
	ExpectedVersion int      `json:"expected_version"`
	Actor           string   `json:"actor"`
	Role            string   `json:"role"`
	IdempotencyKey  string   `json:"idempotency_key"`
}

type FreezeCommand struct {
	BatchID         string `json:"batch_id"`
	ExpectedVersion int    `json:"expected_version"`
	Actor           string `json:"actor"`
	Role            string `json:"role"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type RevokeCredentialCommand struct {
	CredentialID   string `json:"credential_id"`
	Reason         string `json:"reason"`
	Actor          string `json:"actor"`
	Role           string `json:"role"`
	IdempotencyKey string `json:"idempotency_key"`
}

type BatchListQuery struct {
	Species      string
	Status       Status
	SourceRegion string
	HarvestFrom  string
	HarvestTo    string
	Page         int
	PageSize     int
}

type BatchListResult struct {
	Batches       []BatchRecord  `json:"batches"`
	Items         []BatchRecord  `json:"items"`
	Total         int            `json:"total"`
	Page          int            `json:"page"`
	PageSize      int            `json:"page_size"`
	StatusCounts  map[string]int `json:"status_counts"`
	PendingCounts map[string]int `json:"pending_counts"`
}

type EvidenceComparison struct {
	RemediationID       string   `json:"remediation_id"`
	OriginalTestID      string   `json:"original_test_id"`
	RetestID            string   `json:"retest_id"`
	GerminationDelta    float64  `json:"germination_delta"`
	PurityDelta         float64  `json:"purity_delta"`
	MoistureDelta       float64  `json:"moisture_delta"`
	ContaminationBefore bool     `json:"contamination_before"`
	ContaminationAfter  bool     `json:"contamination_after"`
	RegressionCodes     []string `json:"regression_codes"`
	Passed              bool     `json:"passed"`
}

type EvidencePreview struct {
	BatchID        string                  `json:"batch_id"`
	Status         Status                  `json:"status"`
	Manifest       []evidence.ManifestItem `json:"manifest"`
	ManifestDigest string                  `json:"manifest_digest"`
	Comparisons    []EvidenceComparison    `json:"comparisons"`
	AllowReview    bool                    `json:"allow_review"`
	Blocked        bool                    `json:"blocked"`
}
