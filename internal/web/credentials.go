package web

import (
	"net/http"

	"seedvault/internal/workflow"
)

func (s *Server) HandleGetCredential(w http.ResponseWriter, r *http.Request) {
	credential, err := s.workflow.GetCredential(r.PathValue("credentialID"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, credential)
}

func (s *Server) HandleVerifyCredential(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CredentialID string `json:"credential_id"`
		Digest       string `json:"digest"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	result := s.workflow.VerifyCredential(input.CredentialID, input.Digest)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, result)
}

func (s *Server) HandleRevokeCredential(w http.ResponseWriter, r *http.Request) {
	var command workflow.RevokeCredentialCommand
	if !decodeJSON(w, r, &command) {
		return
	}
	command.CredentialID = r.PathValue("credentialID")
	command.IdempotencyKey = requestKey(r, command.IdempotencyKey)
	credential, err := s.workflow.RevokeCredential(command)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, credential)
}
