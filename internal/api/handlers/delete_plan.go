// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/services/deleteplan"
)

// DeletePlanHandler serves the instance-scoped delete plan API.
type DeletePlanHandler struct {
	store   *models.DeletePlanStore
	service *deleteplan.Service
}

// NewDeletePlanHandler creates a new DeletePlanHandler.
func NewDeletePlanHandler(store *models.DeletePlanStore, service *deleteplan.Service) *DeletePlanHandler {
	return &DeletePlanHandler{store: store, service: service}
}

// DeletePlanCreatePayload is the request body for creating a delete plan.
type DeletePlanCreatePayload struct {
	TargetBytes      *int64           `json:"targetBytes"`
	Keys             []models.KeyRule `json:"keys"`
	RemoveDeadGroups *bool            `json:"removeDeadGroups"`
}

// CreateRun creates a run and builds its plan.
func (h *DeletePlanHandler) CreateRun(w http.ResponseWriter, r *http.Request) {
	instanceID, err := parseInstanceID(w, r)
	if err != nil {
		return
	}

	var payload DeletePlanCreatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	if payload.TargetBytes == nil || *payload.TargetBytes <= 0 {
		RespondError(w, http.StatusBadRequest, "targetBytes must be greater than zero")
		return
	}
	removeDeadGroups := true
	if payload.RemoveDeadGroups != nil {
		removeDeadGroups = *payload.RemoveDeadGroups
	}

	if h.service == nil {
		RespondError(w, http.StatusServiceUnavailable, "Delete plan service not available")
		return
	}

	runID, err := h.service.TriggerPlan(r.Context(), instanceID, deleteplan.PlanRequest{
		TargetBytes:      *payload.TargetBytes,
		Keys:             payload.Keys,
		RemoveDeadGroups: removeDeadGroups,
	})
	if err != nil {
		switch {
		case errors.Is(err, deleteplan.ErrInvalidTarget):
			RespondError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, deleteplan.ErrPlanAlreadyActive):
			RespondError(w, http.StatusConflict, err.Error())
		case errors.Is(err, models.ErrInstanceNotFound):
			RespondError(w, http.StatusNotFound, "Instance not found")
		default:
			log.Error().Err(err).Int("instanceID", instanceID).Msg("deleteplan: failed to create run")
			RespondError(w, http.StatusInternalServerError, "Failed to create delete plan")
		}
		return
	}

	run, err := h.store.GetRun(r.Context(), runID)
	if err != nil || run == nil {
		log.Error().Err(err).Int64("runID", runID).Msg("deleteplan: failed to read the new run")
		RespondError(w, http.StatusInternalServerError, "Failed to read delete plan")
		return
	}

	RespondJSON(w, http.StatusAccepted, run)
}

// ListRuns lists recent runs for an instance, newest first.
func (h *DeletePlanHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	instanceID, err := parseInstanceID(w, r)
	if err != nil {
		return
	}

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	runs, err := h.store.ListRuns(r.Context(), instanceID, limit)
	if err != nil {
		log.Error().Err(err).Int("instanceID", instanceID).Msg("deleteplan: failed to list runs")
		RespondError(w, http.StatusInternalServerError, "Failed to list delete plans")
		return
	}
	if runs == nil {
		runs = []*models.DeletePlanRun{}
	}

	RespondJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

// GetRun returns a run with its frozen rows.
func (h *DeletePlanHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	instanceID, err := parseInstanceID(w, r)
	if err != nil {
		return
	}
	runID, ok := parseDeletePlanRunID(w, r)
	if !ok {
		return
	}

	run, err := h.store.GetRunByInstance(r.Context(), instanceID, runID)
	if err != nil {
		log.Error().Err(err).Int64("runID", runID).Msg("deleteplan: failed to get run")
		RespondError(w, http.StatusInternalServerError, "Failed to get delete plan")
		return
	}
	if run == nil {
		RespondError(w, http.StatusNotFound, "Run not found")
		return
	}

	RespondJSON(w, http.StatusOK, run)
}

// CancelRun cancels a run that has not started deleting.
func (h *DeletePlanHandler) CancelRun(w http.ResponseWriter, r *http.Request) {
	instanceID, err := parseInstanceID(w, r)
	if err != nil {
		return
	}
	runID, ok := parseDeletePlanRunID(w, r)
	if !ok {
		return
	}

	run, err := h.store.GetRunByInstance(r.Context(), instanceID, runID)
	if err != nil {
		log.Error().Err(err).Int64("runID", runID).Msg("deleteplan: failed to get run for cancellation")
		RespondError(w, http.StatusInternalServerError, "Failed to get delete plan")
		return
	}
	if run == nil {
		RespondError(w, http.StatusNotFound, "Run not found")
		return
	}

	if h.service == nil {
		RespondError(w, http.StatusServiceUnavailable, "Delete plan service not available")
		return
	}

	if err := h.service.CancelRun(r.Context(), runID); err != nil {
		switch {
		case errors.Is(err, deleteplan.ErrPlanRunNotFound):
			RespondError(w, http.StatusNotFound, "Run not found")
		case errors.Is(err, deleteplan.ErrCannotCancelDuringDeletion):
			RespondError(w, http.StatusConflict, err.Error())
		case errors.Is(err, deleteplan.ErrPlanRunAlreadyFinished):
			RespondError(w, http.StatusConflict, err.Error())
		case errors.Is(err, deleteplan.ErrPlanRunInvalidStatus):
			RespondError(w, http.StatusBadRequest, err.Error())
		default:
			log.Error().Err(err).Int64("runID", runID).Msg("deleteplan: failed to cancel run")
			RespondError(w, http.StatusInternalServerError, "Failed to cancel delete plan")
		}
		return
	}

	run, err = h.store.GetRun(r.Context(), runID)
	if err != nil || run == nil {
		log.Error().Err(err).Int64("runID", runID).Msg("deleteplan: failed to read the canceled run")
		RespondError(w, http.StatusInternalServerError, "Failed to read delete plan")
		return
	}

	RespondJSON(w, http.StatusOK, run)
}

func parseDeletePlanRunID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "runID"), 10, 64)
	if err != nil || runID <= 0 {
		RespondError(w, http.StatusBadRequest, "Invalid run ID")
		return 0, false
	}
	return runID, true
}
