package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"seedvault/internal/evidence"
)

type selfcheckEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func runSelfcheck(server *http.Server, listener net.Listener) error {
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	baseURL := "http://" + listener.Addr().String()
	client := &http.Client{Timeout: 5 * time.Second}
	var batch struct {
		Batch struct {
			BatchID string `json:"batch_id"`
			Version int    `json:"version"`
			Status  string `json:"status"`
		} `json:"batch"`
		Quality struct {
			Issues []struct {
				Code string `json:"code"`
			} `json:"issues"`
		} `json:"quality"`
	}
	if err := postSelfcheck(client, baseURL+"/api/batches", map[string]any{"batch_id": "SELF-CHECK-001", "species_name": "小麦", "source_region": "甘肃省张掖市", "harvest_date": "2025-08-18", "sample_count": 400, "storage_condition": "低温干燥", "actor": "接收员-自检", "role": "receiver", "idempotency_key": "self-create-001"}, &batch); err != nil {
		return err
	}
	badTest := map[string]any{"test_id": "SELF-TEST-001", "expected_version": batch.Batch.Version, "actor": "检测员-自检", "role": "tester", "idempotency_key": "self-test-001", "test": map[string]any{"method_code": "GB/T3543", "replicates": 4, "germination_rate": 70, "purity_rate": 96, "moisture_rate": 14, "contamination_flag": false, "evidence_digest": evidence.DigestText("自检原始证据")}}
	if err := postSelfcheck(client, baseURL+"/api/batches/SELF-CHECK-001/tests", badTest, &batch); err != nil {
		return err
	}
	if batch.Batch.Status != "REMEDIATION_REQUIRED" || len(batch.Quality.Issues) == 0 {
		return fmt.Errorf("低质量检测未进入整改状态")
	}
	issueCodes := make([]string, 0, len(batch.Quality.Issues))
	for _, issue := range batch.Quality.Issues {
		issueCodes = append(issueCodes, issue.Code)
	}
	retest := map[string]any{"original_test_id": "SELF-TEST-001", "retest_id": "SELF-TEST-002", "issue_codes": issueCodes, "explanation": "完成清选与含水率平衡后使用替代样本复测", "expected_version": batch.Batch.Version, "actor": "接收员-自检", "role": "receiver", "idempotency_key": "self-remediation-001", "retest": map[string]any{"method_code": "GB/T3543", "replicates": 4, "germination_rate": 91, "purity_rate": 99, "moisture_rate": 11, "contamination_flag": false, "evidence_digest": evidence.DigestText("自检复测证据")}}
	if err := postSelfcheck(client, baseURL+"/api/batches/SELF-CHECK-001/remediations", retest, &batch); err != nil {
		return err
	}
	if batch.Batch.Status != "READY_REVIEW" {
		return fmt.Errorf("整改通过后状态为 %s", batch.Batch.Status)
	}
	review := map[string]any{"decision": "APPROVE", "comment": "自检独立复核通过", "expected_version": batch.Batch.Version, "actor": "复核员-自检", "role": "reviewer", "idempotency_key": "self-review-001"}
	if err := postSelfcheck(client, baseURL+"/api/batches/SELF-CHECK-001/reviews", review, &batch); err != nil {
		return err
	}
	var frozen struct {
		Batch struct {
			Batch struct {
				Status string `json:"status"`
			} `json:"batch"`
		} `json:"batch"`
		Credential struct {
			CredentialID   string `json:"credential_id"`
			SnapshotDigest string `json:"snapshot_digest"`
		} `json:"credential"`
	}
	freeze := map[string]any{"expected_version": batch.Batch.Version, "actor": "管理员-自检", "role": "administrator", "idempotency_key": "self-freeze-001"}
	if err := postSelfcheck(client, baseURL+"/api/batches/SELF-CHECK-001/freeze", freeze, &frozen); err != nil {
		return err
	}
	if frozen.Batch.Batch.Status != "FROZEN" || frozen.Credential.CredentialID == "" {
		return fmt.Errorf("冻结或凭据签发未完成")
	}
	var verification struct {
		Valid  bool   `json:"valid"`
		Status string `json:"status"`
	}
	if err := postSelfcheck(client, baseURL+"/api/credentials/verify", map[string]any{"credential_id": frozen.Credential.CredentialID, "digest": frozen.Credential.SnapshotDigest}, &verification); err != nil {
		return err
	}
	if !verification.Valid || verification.Status != "VALID" {
		return fmt.Errorf("凭据验证失败")
	}
	context, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(context); err != nil {
		return err
	}
	select {
	case err := <-done:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case <-time.After(time.Second):
		return fmt.Errorf("自检 HTTP 服务未按时结束")
	}
	return nil
}

func postSelfcheck(client *http.Client, url string, payload any, target any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return err
	}
	var envelope selfcheckEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("解析自检响应: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if envelope.Error != nil {
			return fmt.Errorf("%s: %s", url, envelope.Error.Message)
		}
		return fmt.Errorf("%s: HTTP %d", url, response.StatusCode)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return fmt.Errorf("解析自检数据: %w", err)
	}
	return nil
}
