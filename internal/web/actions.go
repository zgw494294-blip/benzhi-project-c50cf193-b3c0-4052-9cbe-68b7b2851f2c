package web

import (
	"net/http"

	"seedvault/internal/workflow"
)

func (s *Server) HandleRecordTest(w http.ResponseWriter, r *http.Request) {
	var command workflow.RecordTestCommand
	if !decodeJSON(w, r, &command) {
		return
	}
	command.BatchID = r.PathValue("batchID")
	command.IdempotencyKey = requestKey(r, command.IdempotencyKey)
	record, err := s.workflow.RecordTest(command)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) HandleRemediation(w http.ResponseWriter, r *http.Request) {
	var command workflow.RemediateCommand
	if !decodeJSON(w, r, &command) {
		return
	}
	command.BatchID = r.PathValue("batchID")
	command.IdempotencyKey = requestKey(r, command.IdempotencyKey)
	record, err := s.workflow.SubmitRemediation(command)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) HandleReview(w http.ResponseWriter, r *http.Request) {
	var command workflow.ReviewCommand
	if !decodeJSON(w, r, &command) {
		return
	}
	command.BatchID = r.PathValue("batchID")
	command.IdempotencyKey = requestKey(r, command.IdempotencyKey)
	record, err := s.workflow.Review(command)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) HandleFreeze(w http.ResponseWriter, r *http.Request) {
	var command workflow.FreezeCommand
	if !decodeJSON(w, r, &command) {
		return
	}
	command.BatchID = r.PathValue("batchID")
	command.IdempotencyKey = requestKey(r, command.IdempotencyKey)
	record, credential, err := s.workflow.Freeze(command)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"batch": record, "credential": credential})
}
