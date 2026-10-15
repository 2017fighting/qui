// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package handlers

import (
	"context"
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
		r.Patch("/runs/{runID}/rows", handler.UpdateRows)
		r.Post("/runs/{runID}/refill", handler.RefillRun)
		r.Post("/runs/{runID}/confirm", handler.ConfirmRun)
		r.Post("/runs/{runID}/cancel", handler.CancelRun)
	})
	return router
}

// fakeDeletePlanService drives the confirm handler without a sync manager.
type fakeDeletePlanService struct {
	confirmErr error
	confirmed  int64
}

func (f *fakeDeletePlanService) TriggerPlan(context.Context, int, deleteplan.PlanRequest) (int64, error) {
	return 0, nil
}

func (f *fakeDeletePlanService) CancelRun(context.Context, int64) error { return nil }

func (f *fakeDeletePlanService) ConfirmRun(_ context.Context, runID int64) error {
	f.confirmed = runID
	return f.confirmErr
}

func (f *fakeDeletePlanService) UpdateRunRows(context.Context, int64, []int64) error { return nil }

func (f *fakeDeletePlanService) RefillRun(context.Context, int64) error { return nil }

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

func TestDeletePlan_ConfirmRunReturns202(t *testing.T) {
	db := testdb.NewMigratedSQLite(t, "delete-plan-confirm")
	instances, err := models.NewInstanceStore(db, []byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	instance, err := instances.Create(t.Context(), "test", "http://example.invalid", "", "", nil, nil, false, nil)
	require.NoError(t, err)
	store := models.NewDeletePlanStore(db)
	runID := seedDeletePlanRun(t, store, instance.ID, models.DeletePlanStatusPreviewReady)

	service := &fakeDeletePlanService{}
	router := newDeletePlanRouter(NewDeletePlanHandler(store, service))

	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d/confirm", instance.ID, runID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, nil))
	require.Equal(t, http.StatusAccepted, resp.Code, resp.Body.String())
	require.Equal(t, runID, service.confirmed)

	var run models.DeletePlanRun
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &run))
	require.Equal(t, models.DeletePlanStatusPreviewReady, run.Status)
}

func TestDeletePlan_ConfirmRunConflictsWhenNotPreviewReady(t *testing.T) {
	_, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-confirm-conflict")
	runID := seedDeletePlanRun(t, store, instance.ID, models.DeletePlanStatusPending)

	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d/confirm", instance.ID, runID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, nil))
	require.Equal(t, http.StatusConflict, resp.Code, resp.Body.String())
}

func TestDeletePlan_ConfirmRunScopesToTheInstance(t *testing.T) {
	instances, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-confirm-scope")
	runID := seedDeletePlanRun(t, store, instance.ID, models.DeletePlanStatusPreviewReady)

	other, err := instances.Create(t.Context(), "other", "http://example.invalid", "", "", nil, nil, false, nil)
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d/confirm", other.ID, runID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, nil))
	require.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())
}

// seedUntickableRun freezes a plan with a candidate beyond the target: rows A
// (100) and B (200) are included against a 250-byte target, planned 300,
// overshoot 50, and C (300) is the next candidate in plan order.
func seedUntickableRun(t *testing.T, store *models.DeletePlanStore, instanceID int) int64 {
	t.Helper()
	ctx := t.Context()
	runID, err := store.CreateRunIfNoActive(ctx, instanceID, 250, "[]", true)
	require.NoError(t, err)
	started, err := store.StartRunPlanning(ctx, runID)
	require.NoError(t, err)
	require.True(t, started)
	require.NoError(t, store.InsertRows(ctx, runID, []*models.DeletePlanRow{
		{
			ContentPath: "/data/A",
			SizeBytes:   100,
			MemberCount: 1,
			Included:    true,
			Members:     []models.DeletePlanMember{{Hash: "a", SizeBytes: 100, Progress: 1, State: "uploading"}},
		},
		{
			ContentPath: "/data/B",
			SizeBytes:   200,
			MemberCount: 1,
			Included:    true,
			Members:     []models.DeletePlanMember{{Hash: "b", SizeBytes: 200, Progress: 1, State: "uploading"}},
		},
		{
			ContentPath: "/data/C",
			SizeBytes:   300,
			MemberCount: 1,
			Members:     []models.DeletePlanMember{{Hash: "c", SizeBytes: 300, Progress: 1, State: "uploading"}},
		},
	}))
	frozen, err := store.UpdateRunPreview(ctx, runID, 300, 0, 50, true)
	require.NoError(t, err)
	require.True(t, frozen)
	return runID
}

func TestDeletePlan_UpdateRowsUnticksAndReportsTheShortfall(t *testing.T) {
	_, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-untick")
	runID := seedUntickableRun(t, store, instance.ID)

	before, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.Len(t, before.Groups, 3)
	untickedRowID := before.Groups[1].ID

	body := fmt.Sprintf(`{"unticked":[%d]}`, untickedRowID)
	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d/rows", instance.ID, runID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPatch, url, strings.NewReader(body)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var run models.DeletePlanRun
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &run))
	require.Len(t, run.Groups, 3, "an unticked row stays in the run")
	require.True(t, run.Groups[0].Included)
	require.False(t, run.Groups[1].Included)
	require.True(t, run.Groups[1].Unticked, "the veto is reported to the client")
	require.False(t, run.Groups[2].Included, "a candidate stays out of the plan")
	require.False(t, run.Groups[2].Unticked)
	require.EqualValues(t, 100, run.PlannedBytes)
	require.EqualValues(t, 0, run.OvershootBytes)
	require.EqualValues(t, 150, run.ShortfallBytes)
	require.False(t, run.TargetReached)
}

func TestDeletePlan_UpdateRowsConflictsWhenNotPreviewReady(t *testing.T) {
	_, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-untick-conflict")
	runID := seedDeletePlanRun(t, store, instance.ID, models.DeletePlanStatusPending)

	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d/rows", instance.ID, runID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPatch, url, strings.NewReader(`{"unticked":[1]}`)))
	require.Equal(t, http.StatusConflict, resp.Code, resp.Body.String())
}

func TestDeletePlan_RefillAddsTheNextCandidateInPlanOrder(t *testing.T) {
	_, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-refill")
	runID := seedUntickableRun(t, store, instance.ID)

	run, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.Len(t, run.Groups, 3)
	untickedRowID := run.Groups[1].ID

	// Veto B, so the plan is 150 bytes short of the target.
	body := fmt.Sprintf(`{"unticked":[%d]}`, untickedRowID)
	untickResp := httptest.NewRecorder()
	untickURL := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d/rows", instance.ID, runID)
	router.ServeHTTP(untickResp, httptest.NewRequestWithContext(t.Context(), http.MethodPatch, untickURL, strings.NewReader(body)))
	require.Equal(t, http.StatusOK, untickResp.Code, untickResp.Body.String())

	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d/refill", instance.ID, runID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var refilled models.DeletePlanRun
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &refilled))
	require.False(t, refilled.Groups[1].Included, "refill never re-includes the vetoed group")
	require.True(t, refilled.Groups[1].Unticked)
	require.True(t, refilled.Groups[2].Included, "refill adds the next candidate")
	require.EqualValues(t, 400, refilled.PlannedBytes)
	require.EqualValues(t, 150, refilled.OvershootBytes)
	require.EqualValues(t, 0, refilled.ShortfallBytes)
	require.True(t, refilled.TargetReached)
	// Plan order is unchanged: A stays before B, B before C.
	require.Equal(t, "/data/A", refilled.Groups[0].ContentPath)
	require.Equal(t, "/data/B", refilled.Groups[1].ContentPath)
	require.Equal(t, "/data/C", refilled.Groups[2].ContentPath)
}

func TestDeletePlan_RefillConflictsWhenNotPreviewReady(t *testing.T) {
	_, store, instance, _, router := newDeletePlanHandlerFixture(t, "delete-plan-refill-conflict")
	runID := seedDeletePlanRun(t, store, instance.ID, models.DeletePlanStatusPending)

	resp := httptest.NewRecorder()
	url := fmt.Sprintf("/api/instances/%d/delete-plan/runs/%d/refill", instance.ID, runID)
	router.ServeHTTP(resp, httptest.NewRequestWithContext(t.Context(), http.MethodPost, url, nil))
	require.Equal(t, http.StatusConflict, resp.Code, resp.Body.String())
}
