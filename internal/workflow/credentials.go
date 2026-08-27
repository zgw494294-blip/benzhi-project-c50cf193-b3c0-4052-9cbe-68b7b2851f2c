package workflow

import (
	"strings"
)

func (s *Service) RevokeCredential(command RevokeCredentialCommand) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := requireRole(command.Role, "administrator"); err != nil {
		return Credential{}, err
	}
	if err := requireKey(command.IdempotencyKey); err != nil {
		return Credential{}, err
	}
	command.CredentialID = strings.TrimSpace(command.CredentialID)
	command.Reason = strings.TrimSpace(command.Reason)
	command.Actor = strings.TrimSpace(command.Actor)
	if command.CredentialID == "" {
		return Credential{}, invalid("credential_id", "凭据编号不能为空")
	}
	if command.Reason == "" {
		return Credential{}, invalid("reason", "撤销原因不能为空")
	}
	if command.Actor == "" {
		return Credential{}, invalid("actor", "操作人不能为空")
	}
	credential, ok := s.state.Credentials[command.CredentialID]
	if !ok {
		return Credential{}, &DomainError{Code: CodeNotFound, Message: "凭据不存在"}
	}
	if existingBatchID, exists := s.state.Idempotency[command.IdempotencyKey]; exists {
		if existingBatchID != credential.BatchID || credential.RevocationKey != command.IdempotencyKey {
			return Credential{}, &DomainError{Code: CodeConflict, Message: "幂等键已用于另一个操作"}
		}
		return *credential, nil
	}
	if credential.Status != "VALID" {
		return Credential{}, stateError(Status(credential.Status), "撤销入库凭据")
	}
	batch, ok := s.state.Batches[credential.BatchID]
	if !ok {
		return Credential{}, &DomainError{Code: CodePersistence, Message: "凭据关联批次不存在"}
	}
	updated := *credential
	now := s.now().UTC()
	updated.Status = "REVOKED"
	updated.RevokedAt = &now
	updated.RevokedBy = command.Actor
	updated.RevocationReason = command.Reason
	updated.RevocationKey = strings.TrimSpace(command.IdempotencyKey)
	if err := s.commitRecord("credential.revoked", command.Actor, command.IdempotencyKey, batch, &updated); err != nil {
		return Credential{}, err
	}
	return updated, nil
}
