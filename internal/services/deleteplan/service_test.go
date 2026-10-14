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
