package web

import (
	"net/http"
	"strconv"

	"seedvault/internal/quality"
	"seedvault/internal/workflow"
)

func (s *Server) HandleProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, quality.Profiles())
}

func (s *Server) HandleListBatches(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if len(query) == 0 {
		writeJSON(w, http.StatusOK, s.workflow.ListBatches())
		return
	}
	page, pageSize := 1, 20
	var err error
	if value := query.Get("page"); value != "" {
		page, err = strconv.Atoi(value)
		if err != nil {
			writeDomainError(w, workflowInvalid("page", "页码必须是整数"))
			return
		}
	}
	if value := query.Get("page_size"); value != "" {
		pageSize, err = strconv.Atoi(value)
		if err != nil {
			writeDomainError(w, workflowInvalid("page_size", "页大小必须是整数"))
			return
		}
	}
	harvestFrom := query.Get("harvest_from")
	if harvestFrom == "" {
		harvestFrom = query.Get("harvest_date_from")
	}
	harvestTo := query.Get("harvest_to")
	if harvestTo == "" {
		harvestTo = query.Get("harvest_date_to")
	}
	result, queryErr := s.workflow.QueryBatches(workflow.BatchListQuery{Species: query.Get("species"), Status: workflow.Status(query.Get("status")), SourceRegion: query.Get("source_region"), HarvestFrom: harvestFrom, HarvestTo: harvestTo, Page: page, PageSize: pageSize})
	if queryErr != nil {
		writeDomainError(w, queryErr)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func workflowInvalid(field, message string) error {
	return &workflow.DomainError{Code: workflow.CodeInvalid, Message: message, Fields: map[string]string{field: message}}
}

func (s *Server) HandleCreateBatch(w http.ResponseWriter, r *http.Request) {
	var command workflow.CreateBatchCommand
	if !decodeJSON(w, r, &command) {
		return
	}
	command.IdempotencyKey = requestKey(r, command.IdempotencyKey)
	record, err := s.workflow.CreateBatchContext(r.Context(), command)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) HandleGetBatch(w http.ResponseWriter, r *http.Request) {
	record, err := s.workflow.GetBatch(r.PathValue("batchID"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) HandleTimeline(w http.ResponseWriter, r *http.Request) {
	record, err := s.workflow.GetBatch(r.PathValue("batchID"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record.Timeline)
}

func (s *Server) HandleEvidencePreview(w http.ResponseWriter, r *http.Request) {
	preview, err := s.workflow.EvidencePreview(r.PathValue("batchID"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
