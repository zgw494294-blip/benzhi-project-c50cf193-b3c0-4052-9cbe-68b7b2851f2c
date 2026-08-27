package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// BuildManifest 汇总检测与整改，输出按类型和编号排序的稳定清单。
func BuildManifest(tests []TestRun, remediations []Remediation) []ManifestItem {
	items := make([]ManifestItem, 0, len(tests)+len(remediations))
	for _, run := range tests {
		relation := "原始检测"
		if run.SupersedesTestID != "" {
			relation = "替代:" + run.SupersedesTestID
		}
		items = append(items, ManifestItem{
			Kind: "test", ID: run.TestID, Digest: run.EvidenceDigest, Relation: relation,
			Summary:   fmt.Sprintf("%s；发芽率 %.2f%%；纯度 %.2f%%；含水率 %.2f%%；污染=%t", run.MethodCode, run.GerminationRate, run.PurityRate, run.MoistureRate, run.ContaminationFlag),
			Timestamp: run.ObservedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	for _, remediation := range remediations {
		payload, _ := json.Marshal(struct {
			Issues      []string `json:"issues"`
			Explanation string   `json:"explanation"`
			Original    string   `json:"original"`
			Retest      string   `json:"retest"`
		}{remediation.IssueCodes, remediation.Explanation, remediation.OriginalTest, remediation.RetestID})
		sum := sha256.Sum256(payload)
		items = append(items, ManifestItem{
			Kind: "remediation", ID: remediation.RemediationID, Digest: hex.EncodeToString(sum[:]),
			Relation:  remediation.OriginalTest + "->" + remediation.RetestID,
			Summary:   remediation.Explanation,
			Timestamp: remediation.SubmittedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind == items[j].Kind {
			return items[i].ID < items[j].ID
		}
		return items[i].Kind < items[j].Kind
	})
	return items
}

// ManifestDigest 对清单使用规范 JSON 计算摘要。
func ManifestDigest(items []ManifestItem) string {
	data, _ := json.Marshal(items)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
