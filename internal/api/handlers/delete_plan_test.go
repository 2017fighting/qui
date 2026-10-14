// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/services/deleteplan"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

func newDeletePlanRouter(handler *DeletePlanHandler) *chi.Mux {
	router := chi.NewRouter()
	router.Route("/api/instances/{instanceID}/delete-plan", func(r chi.Router) {
		r.Post("/runs", handler.CreateRun)
		r.Get("/runs", handler.ListRuns)
		r.Get("/runs/{runID}", handler.GetRun)
		r.Post("/runs/{runID}/cancel", handler.CancelRun)
	})
	return router
}

func newDeletePlanHandlerFixture(t *testing.T, name string) (*models.InstanceStore, *models.DeletePlanStore, *models.Instance, *DeletePlanHandler, *chi.Mux) {
	t.Helper()
	db := testdb.NewMigratedSQLite(t, name)
	instances, err := models.NewInstanceStore(db, []byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	instance, err := instances.Create(t.Context(), "test", "http://example.invalid", "", "", nil, nil, false, nil)
	require.NoError(t, err)

	store := models.NewDeletePlanStore(db)
	service := deleteplan.NewService(deleteplan.Config{
		SyncPollInterval:  time.Millisecond,
		StuckRunThreshold: time.Hour,
	}, instances, store, nil)
	handler := NewDeletePlanHandler(store, service)
	return instances, store, instance, handler, newDeletePlanRouter(handler)
}

func seedDeletePlanRun(t *testing.T, store *models.DeletePlanStore, instanceID int, status string) int64 {
	t.Helper()
	ctx := t.Context()
	runID, err := store.CreateRunIfNoActive(ctx, instanceID, 1000, "[]", true)
	require.NoError(t, err)
	if status == models.DeletePlanStatusPreviewReady {
		started, err := store.StartRunPlanning(ctx, runID)
		require.NoError(t, err)
		require.True(t, started)
		require.NoError(t, store.InsertRows(ctx, runID, []*models.DeletePlanRow{
			{
				ContentPath: "/data/Pack",
				SizeBytes:   500,
				MemberCount: 1,
				Included:    true,
				Members: []models.DeletePlanMember{
					{Hash: "a", Name: "Pack", SizeBytes: 500, Progress: 1, State: "uploading", Tags: []string{}, SavePath: "/data"},
				},
			},
			{
				ContentPath:   "/data/Protected",
				SizeBytes:     900,
				MemberCount:   1,
				Excluded:      true,
				ExcludeReason: "incomplete",
				Members: []models.DeletePlanMember{
					{Hash: "b", Name: "Protected", SizeBytes: 900, Progress: 0.5, State: "downloading", Tags: []string{}, SavePath: "/data"},
				},
			},
		}))
		frozen, err := store.UpdateRunPreview(ctx, runID, 500, 0, 0, false)
		require.NoError(t, err)
		require.True(t, frozen)
	}
	return runID
}

func TestDeletePlan_CreateRunReturns202(t *testing.T) {
	_, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-create")
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs", instance.ID)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(`{"targetBytes":10995116277760}`)))
	require.Equal(t, http.StatusAccepted, resp.Code, resp.Body.String())

	var run models.DeletePlanRun
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &run))
	require.Equal(t, instance.ID, run.InstanceID)
	require.EqualValues(t, 10995116277760, run.TargetBytes)
	require.Equal(t, models.DeletePlanStatusPending, run.Status)
	require.True(t, run.WaitingForSync)
	require.True(t, run.RemoveDeadGroups, "removeDeadGroups defaults true")
	require.Len(t, run.Keys, 4, "keys default to the server chain")
	require.NotNil(t, run.Groups)
	require.NotNil(t, run.ExcludedGroups)

	// The fixture has no sync manager, so the run waits for fresh sync. Cancel it
	// so the background plan stops before the database closes.
	cancelURL := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d/cancel", instance.ID, run.ID)
	cancelResp := httptest.NewRecorder()
	router.ServeHTTP(cancelResp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, cancelURL, nil))
	require.Equal(t, http.StatusOK, cancelResp.Code, cancelResp.Body.String())

	stored, err := store.GetRun(t.Context(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeletePlanStatusCanceled, stored.Status)
}

func TestDeletePlan_CreateRunRejectsABadTarget(t *testing.T) {
	_, _, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-bad-target")
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs", instance.ID)

	for _, body := range []string{`{"targetBytes":0}`, `{}`, `{"targetBytes":-1}`} {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(body)))
		require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
	}
}

func TestDeletePlan_SecondActiveRunConflicts(t *testing.T) {
	_, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-conflict")
	seedDeletePlanRun(t, store, instance.ID, models.DeletePlanStatusPending)

	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs", instance.ID)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(`{"targetBytes":1000}`)))
	require.Equal(t, http.StatusConflict, resp.Code, resp.Body.String())

	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Equal(t, "a delete plan is already active for this instance", body.Error)
}

func TestDeletePlan_GetRunReturnsTheFrozenBody(t *testing.T) {
	instances, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-get")
	runID := seedDeletePlanRun(t, store, instance.ID, models.DeletePlanStatusPreviewReady)

	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d", instance.ID, runID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodGet, url, nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var run models.DeletePlanRun
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &run))
	require.Equal(t, models.DeletePlanStatusPreviewReady, run.Status)
	require.Len(t, run.Groups, 1)
	require.Equal(t, "/data/Pack", run.Groups[0].ContentPath)
	require.Len(t, run.Groups[0].Members, 1)
	require.Len(t, run.ExcludedGroups, 1)
	require.Equal(t, "incomplete", run.ExcludedGroups[0].ExcludeReason)

	// Another instance never sees this run.
	other, err := instances.Create(t.Context(), "other", "http://example.invalid", "", "", nil, nil, false, nil)
	require.NoError(t, err)
	resp = httptest.NewRecorder()
	url = fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d", other.ID, runID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodGet, url, nil))
	require.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())
}

func TestDeletePlan_ListRuns(t *testing.T) {
	_, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-list")
	seedDeletePlanRun(t, store, instance.ID, models.DeletePlanStatusPreviewReady)

	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs?limit=20", instance.ID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodGet, url, nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body struct {
		Runs []models.DeletePlanRun `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Len(t, body.Runs, 1)
	require.Equal(t, models.DeletePlanStatusPreviewReady, body.Runs[0].Status)
}

func TestDeletePlan_CancelRun(t *testing.T) {
	_, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-cancel")
	runID := seedDeletePlanRun(t, store, instance.ID, models.DeletePlanStatusPreviewReady)

	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d/cancel", instance.ID, runID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var run models.DeletePlanRun
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &run))
	require.Equal(t, models.DeletePlanStatusCanceled, run.Status)

	// A finished run cannot be canceled again.
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, nil))
	require.Equal(t, http.StatusConflict, resp.Code, resp.Body.String())
}
