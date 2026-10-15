// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package deleteplan

import (
	"context"
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

func newServiceFixture(t *testing.T, client *fakeClient) (*Service, *models.DeletePlanStore, *fakeSync, int) {
	t.Helper()
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, "delete-plan-service")
	instances, err := models.NewInstanceStore(db, []byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	instance, err := instances.Create(ctx, "test", "http://example.invalid", "user", "pass", nil, nil, false, nil)
	require.NoError(t, err)

	store := models.NewDeletePlanStore(db)
	svc := NewService(Config{
		SyncPollInterval:  5 * time.Millisecond,
		StuckRunThreshold: time.Hour,
	}, instances, store, nil)

	fake := stubSync(svc)
	fake.getInstance = func(_ context.Context, _ int) (*models.Instance, error) {
		return instance, nil
	}
	fake.getClient = func(_ context.Context, _ int) (healthChecker, error) {
		return client, nil
	}
	return svc, store, fake, instance.ID
}

func TestService_TriggerPlanFreezesAPreview(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now())
	svc, store, fake, instanceID := newServiceFixture(t, client)
	fake.getAllTorrents = func(_ context.Context, _ int) ([]qbt.Torrent, error) {
		return []qbt.Torrent{
			torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, ""),
			torrent("b", "/data/B", 200, 1, qbt.TorrentStateUploading, ""),
		}, nil
	}

	runID, err := svc.TriggerPlan(t.Context(), instanceID, PlanRequest{TargetBytes: 150, RemoveDeadGroups: true})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		run, err := store.GetRun(t.Context(), runID)
		return err == nil && run != nil && run.Status == models.DeletePlanStatusPreviewReady
	}, 2*time.Second, 5*time.Millisecond)

	run, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.False(t, run.WaitingForSync)
	require.EqualValues(t, 300, run.PlannedBytes)
	require.EqualValues(t, 150, run.OvershootBytes)
	require.True(t, run.TargetReached)
	require.Len(t, run.Groups, 2)
	require.Len(t, run.Groups[0].Members, 1)
	require.Equal(t, "a", run.Groups[0].Members[0].Hash)
}

func TestService_PendingRunWaitsForFreshSync(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now().Add(-5 * time.Minute))
	svc, store, fake, instanceID := newServiceFixture(t, client)
	fake.getAllTorrents = func(_ context.Context, _ int) ([]qbt.Torrent, error) {
		return []qbt.Torrent{torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, "")}, nil
	}

	runID, err := svc.TriggerPlan(t.Context(), instanceID, PlanRequest{TargetBytes: 100, RemoveDeadGroups: true})
	require.NoError(t, err)

	run, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.Equal(t, models.DeletePlanStatusPending, run.Status)
	require.True(t, run.WaitingForSync)

	// Once the instance syncs, the run builds the plan.
	client.setLastSync(time.Now())
	require.Eventually(t, func() bool {
		run, err := store.GetRun(t.Context(), runID)
		return err == nil && run != nil && run.Status == models.DeletePlanStatusPreviewReady
	}, 2*time.Second, 5*time.Millisecond)
}

func TestService_SecondPlanIsRejected(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now())
	svc, store, fake, instanceID := newServiceFixture(t, client)
	fake.getAllTorrents = func(_ context.Context, _ int) ([]qbt.Torrent, error) {
		return nil, nil
	}

	first, err := svc.TriggerPlan(t.Context(), instanceID, PlanRequest{TargetBytes: 100, RemoveDeadGroups: true})
	require.NoError(t, err)

	// Wait for the first plan to finish, so the run is preview_ready and still active.
	require.Eventually(t, func() bool {
		run, err := store.GetRun(t.Context(), first)
		return err == nil && run != nil && run.Status == models.DeletePlanStatusPreviewReady
	}, 2*time.Second, 5*time.Millisecond)

	_, err = svc.TriggerPlan(t.Context(), instanceID, PlanRequest{TargetBytes: 100, RemoveDeadGroups: true})
	require.ErrorIs(t, err, ErrPlanAlreadyActive)
}

func TestService_CancelRun(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now().Add(-5 * time.Minute))
	svc, store, _, instanceID := newServiceFixture(t, client)

	runID, err := svc.TriggerPlan(t.Context(), instanceID, PlanRequest{TargetBytes: 100, RemoveDeadGroups: true})
	require.NoError(t, err)

	require.NoError(t, svc.CancelRun(t.Context(), runID))
	run, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.Equal(t, models.DeletePlanStatusCanceled, run.Status)
	require.False(t, run.WaitingForSync)

	require.ErrorIs(t, svc.CancelRun(t.Context(), runID), ErrPlanRunAlreadyFinished)
}

func TestService_RejectsAnInvalidTarget(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now())
	svc, _, _, instanceID := newServiceFixture(t, client)

	_, err := svc.TriggerPlan(t.Context(), instanceID, PlanRequest{TargetBytes: 0, RemoveDeadGroups: true})
	require.ErrorIs(t, err, ErrInvalidTarget)
}

// seedPreviewPlan freezes a plan with a candidate beyond the target: A (100) and
// B (200) are included against a 250-byte target, planned 300, overshoot 50, and
// C (300) is the next candidate in plan order.
func seedPreviewPlan(t *testing.T, store *models.DeletePlanStore, instanceID int, targetBytes int64) int64 {
	t.Helper()
	ctx := t.Context()
	runID, err := store.CreateRunIfNoActive(ctx, instanceID, targetBytes, "[]", true)
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

func TestService_UpdateRunRowsUnticksAndReportsTheShortfall(t *testing.T) {
	t.Parallel()
	svc, store, _, instanceID := newServiceFixture(t, newFakeClient(time.Now()))
	runID := seedPreviewPlan(t, store, instanceID, 250)

	run, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.Len(t, run.Groups, 3)

	require.NoError(t, svc.UpdateRunRows(t.Context(), runID, []int64{run.Groups[1].ID}))

	updated, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.True(t, updated.Groups[0].Included)
	require.False(t, updated.Groups[1].Included, "an unticked row stays in the run with included false")
	require.True(t, updated.Groups[1].Unticked)
	require.False(t, updated.Groups[2].Included, "a candidate stays out of the plan")
	require.False(t, updated.Groups[2].Unticked)
	require.EqualValues(t, 100, updated.PlannedBytes)
	require.EqualValues(t, 0, updated.OvershootBytes)
	require.EqualValues(t, 150, updated.ShortfallBytes)
	require.False(t, updated.TargetReached)
}

func TestService_RefillAddsTheNextCandidateWithoutReincludingTheVeto(t *testing.T) {
	t.Parallel()
	svc, store, _, instanceID := newServiceFixture(t, newFakeClient(time.Now()))
	runID := seedPreviewPlan(t, store, instanceID, 250)

	run, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	// Veto B: the plan is 150 bytes short of the target.
	require.NoError(t, svc.UpdateRunRows(t.Context(), runID, []int64{run.Groups[1].ID}))

	require.NoError(t, svc.RefillRun(t.Context(), runID))

	updated, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.True(t, updated.Groups[0].Included)
	require.False(t, updated.Groups[1].Included, "refill never re-includes a vetoed group")
	require.True(t, updated.Groups[1].Unticked)
	require.True(t, updated.Groups[2].Included, "refill adds the next candidate in plan order")
	require.EqualValues(t, 400, updated.PlannedBytes)
	require.EqualValues(t, 150, updated.OvershootBytes)
	require.EqualValues(t, 0, updated.ShortfallBytes)
	require.True(t, updated.TargetReached)
	// Refill flips flags only: the groups that stay keep their order.
	require.Equal(t, "/data/A", updated.Groups[0].ContentPath)
	require.Equal(t, "/data/B", updated.Groups[1].ContentPath)
	require.Equal(t, "/data/C", updated.Groups[2].ContentPath)
}

func TestService_RefillStopsAtTheTarget(t *testing.T) {
	t.Parallel()
	svc, store, _, instanceID := newServiceFixture(t, newFakeClient(time.Now()))
	runID := seedPreviewPlan(t, store, instanceID, 250)

	// The plan already reaches the target, so refill adds no candidate.
	require.NoError(t, svc.RefillRun(t.Context(), runID))

	updated, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.False(t, updated.Groups[2].Included)
	require.EqualValues(t, 300, updated.PlannedBytes)
	require.EqualValues(t, 50, updated.OvershootBytes)
}

func TestService_UpdateRunRowsReincludesARetickedRow(t *testing.T) {
	t.Parallel()
	svc, store, _, instanceID := newServiceFixture(t, newFakeClient(time.Now()))
	runID := seedPreviewPlan(t, store, instanceID, 250)

	run, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.NoError(t, svc.UpdateRunRows(t.Context(), runID, []int64{run.Groups[1].ID}))

	// Tick B back on: it leaves the unticked set and returns to the plan.
	require.NoError(t, svc.UpdateRunRows(t.Context(), runID, nil))

	updated, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	require.True(t, updated.Groups[1].Included)
	require.False(t, updated.Groups[1].Unticked)
	require.False(t, updated.Groups[2].Included, "re-ticking does not pull in a candidate")
	require.EqualValues(t, 300, updated.PlannedBytes)
}

func TestService_UpdateRunRowsRejectsAForeignRow(t *testing.T) {
	t.Parallel()
	svc, store, _, instanceID := newServiceFixture(t, newFakeClient(time.Now()))
	runID := seedPreviewPlan(t, store, instanceID, 250)

	require.ErrorIs(t, svc.UpdateRunRows(t.Context(), runID, []int64{999999}), ErrPlanRowNotInRun)
}

func TestService_UpdateRunRowsRejectsANonPreviewRun(t *testing.T) {
	t.Parallel()
	svc, store, _, instanceID := newServiceFixture(t, newFakeClient(time.Now()))
	runID, err := store.CreateRunIfNoActive(t.Context(), instanceID, 1000, "[]", true)
	require.NoError(t, err)

	require.ErrorIs(t, svc.UpdateRunRows(t.Context(), runID, nil), ErrPlanRunInvalidStatus)
	require.ErrorIs(t, svc.RefillRun(t.Context(), runID), ErrPlanRunInvalidStatus)
}

func TestSelectionTotals(t *testing.T) {
	t.Parallel()
	rows := []*models.DeletePlanRow{
		{ID: 1, SizeBytes: 100},
		{ID: 2, SizeBytes: 200},
		{ID: 3, SizeBytes: 400, Dead: true},
	}

	// Every row is selected.
	planned, dead, overshoot, reached := selectionTotals(rows, 250, idSet([]int64{1, 2, 3}))
	require.EqualValues(t, 300, planned)
	require.EqualValues(t, 400, dead, "dead bytes never count toward the target")
	require.EqualValues(t, 50, overshoot)
	require.True(t, reached)

	// Only row 2 is selected: an unselected row contributes nothing.
	planned, dead, overshoot, reached = selectionTotals(rows, 250, idSet([]int64{2}))
	require.EqualValues(t, 200, planned)
	require.EqualValues(t, 0, dead)
	require.EqualValues(t, 0, overshoot)
	require.False(t, reached)

	// Only the dead row is selected: its bytes never reach the target.
	planned, dead, overshoot, reached = selectionTotals(rows, 250, idSet([]int64{3}))
	require.EqualValues(t, 0, planned)
	require.EqualValues(t, 400, dead)
	require.EqualValues(t, 0, overshoot)
	require.False(t, reached)
}
