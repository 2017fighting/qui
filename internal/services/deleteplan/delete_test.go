// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package deleteplan

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/models"
)

// bulkCall is one BulkAction the deletion path made.
type bulkCall struct {
	hashes []string
	action string
}

// bulkRecorder collects the bulk delete calls from the deletion goroutine.
type bulkRecorder struct {
	mu    sync.Mutex
	calls []bulkCall
}

func (r *bulkRecorder) record(hashes []string, action string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, bulkCall{hashes: slices.Clone(hashes), action: action})
}

func (r *bulkRecorder) snapshot() []bulkCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

// freezePreviewRun builds a preview_ready run with the given rows.
func freezePreviewRun(t *testing.T, store *models.DeletePlanStore, instanceID int, rows []*models.DeletePlanRow) int64 {
	t.Helper()
	ctx := t.Context()
	runID, err := store.CreateRunIfNoActive(ctx, instanceID, 1000, "[]", true)
	require.NoError(t, err)
	started, err := store.StartRunPlanning(ctx, runID)
	require.NoError(t, err)
	require.True(t, started)
	require.NoError(t, store.InsertRows(ctx, runID, rows))
	frozen, err := store.UpdateRunPreview(ctx, runID, 1000, 0, 0, true)
	require.NoError(t, err)
	require.True(t, frozen)
	return runID
}

func singleMemberRow(path, hash string, size int64) *models.DeletePlanRow {
	return &models.DeletePlanRow{
		ContentPath: path,
		SizeBytes:   size,
		MemberCount: 1,
		Included:    true,
		Members: []models.DeletePlanMember{
			{Hash: hash, Name: hash, SizeBytes: size, Progress: 1, State: "uploading"},
		},
	}
}

func waitForRunStatus(t *testing.T, store *models.DeletePlanStore, runID int64, status string) *models.DeletePlanRun {
	t.Helper()
	require.Eventually(t, func() bool {
		run, err := store.GetRun(t.Context(), runID)
		return err == nil && run != nil && run.Status == status
	}, 2*time.Second, 5*time.Millisecond, "run did not reach %s", status)

	run, err := store.GetRun(t.Context(), runID)
	require.NoError(t, err)
	return run
}

func TestService_ConfirmRemovesTheGroupsWithFiles(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now())
	svc, store, fake, instanceID := newServiceFixture(t, client)

	runID := freezePreviewRun(t, store, instanceID, []*models.DeletePlanRow{
		singleMemberRow("/data/A", "a", 100),
		singleMemberRow("/data/B", "b", 200),
	})
	fake.getAllTorrents = func(_ context.Context, _ int) ([]qbt.Torrent, error) {
		return []qbt.Torrent{
			torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, ""),
			torrent("b", "/data/B", 200, 1, qbt.TorrentStateUploading, ""),
		}, nil
	}
	recorder := &bulkRecorder{}
	fake.bulkAction = func(_ context.Context, _ int, hashes []string, action string) error {
		recorder.record(hashes, action)
		return nil
	}

	require.NoError(t, svc.ConfirmRun(t.Context(), runID))
	run := waitForRunStatus(t, store, runID, models.DeletePlanStatusCompleted)

	require.EqualValues(t, 300, run.RemovedBytes)
	require.Empty(t, run.ErrorMessage)
	require.Empty(t, run.WarningMessage)

	calls := recorder.snapshot()
	require.Len(t, calls, 1)
	require.Equal(t, []string{"a", "b"}, calls[0].hashes)
	require.Equal(t, "deleteWithFiles", calls[0].action)

	for _, row := range run.Groups {
		require.Equal(t, models.DeletePlanRowStatusDeleted, row.RowStatus)
	}
}

func TestService_ConfirmChunksTheHashesByFifty(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now())
	svc, store, fake, instanceID := newServiceFixture(t, client)

	const count = 120
	rows := make([]*models.DeletePlanRow, 0, count)
	torrents := make([]qbt.Torrent, 0, count)
	for i := range count {
		hash := fmt.Sprintf("h%03d", i)
		rows = append(rows, singleMemberRow("/data/"+hash, hash, 10))
		torrents = append(torrents, torrent(hash, "/data/"+hash, 10, 1, qbt.TorrentStateUploading, ""))
	}
	runID := freezePreviewRun(t, store, instanceID, rows)
	fake.getAllTorrents = func(_ context.Context, _ int) ([]qbt.Torrent, error) {
		return torrents, nil
	}
	recorder := &bulkRecorder{}
	fake.bulkAction = func(_ context.Context, _ int, hashes []string, action string) error {
		recorder.record(hashes, action)
		return nil
	}

	require.NoError(t, svc.ConfirmRun(t.Context(), runID))
	run := waitForRunStatus(t, store, runID, models.DeletePlanStatusCompleted)

	calls := recorder.snapshot()
	require.Len(t, calls, 3)
	require.Len(t, calls[0].hashes, 50)
	require.Len(t, calls[1].hashes, 50)
	require.Len(t, calls[2].hashes, 20)
	for _, call := range calls {
		require.Equal(t, "deleteWithFiles", call.action)
	}
	require.EqualValues(t, count*10, run.RemovedBytes)
}

func TestService_AFailedChunkDoesNotStopTheRest(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now())
	svc, store, fake, instanceID := newServiceFixture(t, client)

	const count = 120
	rows := make([]*models.DeletePlanRow, 0, count)
	torrents := make([]qbt.Torrent, 0, count)
	for i := range count {
		hash := fmt.Sprintf("h%03d", i)
		rows = append(rows, singleMemberRow("/data/"+hash, hash, 10))
		torrents = append(torrents, torrent(hash, "/data/"+hash, 10, 1, qbt.TorrentStateUploading, ""))
	}
	runID := freezePreviewRun(t, store, instanceID, rows)
	fake.getAllTorrents = func(_ context.Context, _ int) ([]qbt.Torrent, error) {
		return torrents, nil
	}
	recorder := &bulkRecorder{}
	var attempts atomic.Int32
	fake.bulkAction = func(_ context.Context, _ int, hashes []string, action string) error {
		recorder.record(hashes, action)
		if attempts.Add(1) == 2 {
			return errors.New("chunk failed")
		}
		return nil
	}

	require.NoError(t, svc.ConfirmRun(t.Context(), runID))
	run := waitForRunStatus(t, store, runID, models.DeletePlanStatusCompleted)

	require.Len(t, recorder.snapshot(), 3, "the third chunk still runs after the second fails")
	require.Contains(t, run.WarningMessage, "chunk failed")
	require.Empty(t, run.ErrorMessage)
	require.EqualValues(t, 700, run.RemovedBytes, "only the first and third chunks removed bytes")

	for i, row := range run.Groups {
		if i >= 50 && i < 100 {
			require.Equal(t, models.DeletePlanRowStatusFailed, row.RowStatus, "row %d", i)
			require.Contains(t, row.RowError, "chunk failed")
			continue
		}
		require.Equal(t, models.DeletePlanRowStatusDeleted, row.RowStatus, "row %d", i)
	}
}

func TestService_SkipsAChangedGroupWhole(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now())
	svc, store, fake, instanceID := newServiceFixture(t, client)

	runID := freezePreviewRun(t, store, instanceID, []*models.DeletePlanRow{
		singleMemberRow("/data/A", "a", 100),
		{
			ContentPath: "/data/B",
			SizeBytes:   200,
			MemberCount: 2,
			Included:    true,
			Members: []models.DeletePlanMember{
				{Hash: "b", Name: "b", SizeBytes: 200, Progress: 1, State: "uploading"},
				{Hash: "c", Name: "c", SizeBytes: 150, Progress: 1, State: "uploading"},
			},
		},
	})
	// c left the group since the plan was built: B must not be removed.
	fake.getAllTorrents = func(_ context.Context, _ int) ([]qbt.Torrent, error) {
		return []qbt.Torrent{
			torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, ""),
			torrent("b", "/data/B", 200, 1, qbt.TorrentStateUploading, ""),
		}, nil
	}
	recorder := &bulkRecorder{}
	fake.bulkAction = func(_ context.Context, _ int, hashes []string, action string) error {
		recorder.record(hashes, action)
		return nil
	}

	require.NoError(t, svc.ConfirmRun(t.Context(), runID))
	run := waitForRunStatus(t, store, runID, models.DeletePlanStatusCompleted)

	require.Equal(t, models.DeletePlanRowStatusSkipped, run.Groups[1].RowStatus)
	require.Equal(t, ReasonMembersChanged, run.Groups[1].RowReason)
	require.EqualValues(t, 100, run.RemovedBytes, "the changed group's bytes are not counted")

	calls := recorder.snapshot()
	require.Len(t, calls, 1)
	require.Equal(t, []string{"a"}, calls[0].hashes)
}

func TestService_SkipsANewlyProtectedGroupWhole(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now())
	svc, store, fake, instanceID := newServiceFixture(t, client)

	runID := freezePreviewRun(t, store, instanceID, []*models.DeletePlanRow{
		singleMemberRow("/data/A", "a", 100),
	})
	// a is downloading again since the plan was built.
	fake.getAllTorrents = func(_ context.Context, _ int) ([]qbt.Torrent, error) {
		return []qbt.Torrent{
			torrent("a", "/data/A", 100, 0.5, qbt.TorrentStateDownloading, ""),
		}, nil
	}
	recorder := &bulkRecorder{}
	fake.bulkAction = func(_ context.Context, _ int, hashes []string, action string) error {
		recorder.record(hashes, action)
		return nil
	}

	require.NoError(t, svc.ConfirmRun(t.Context(), runID))
	run := waitForRunStatus(t, store, runID, models.DeletePlanStatusCompleted)

	require.Equal(t, models.DeletePlanRowStatusSkipped, run.Groups[0].RowStatus)
	require.Equal(t, ReasonIncomplete, run.Groups[0].RowReason)
	require.Empty(t, recorder.snapshot(), "a skipped group sends no hashes")
	require.Zero(t, run.RemovedBytes)
}

func TestService_ConfirmRejectsANonPreviewRun(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now())
	svc, store, _, instanceID := newServiceFixture(t, client)

	runID, err := store.CreateRunIfNoActive(t.Context(), instanceID, 100, "[]", true)
	require.NoError(t, err)

	require.ErrorIs(t, svc.ConfirmRun(t.Context(), runID), ErrPlanRunInvalidStatus)
}

func TestService_CannotCancelDuringDeletion(t *testing.T) {
	t.Parallel()
	client := newFakeClient(time.Now())
	svc, store, fake, instanceID := newServiceFixture(t, client)

	runID := freezePreviewRun(t, store, instanceID, []*models.DeletePlanRow{
		singleMemberRow("/data/A", "a", 100),
	})
	fake.getAllTorrents = func(_ context.Context, _ int) ([]qbt.Torrent, error) {
		return []qbt.Torrent{torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, "")}, nil
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	fake.bulkAction = func(_ context.Context, _ int, _ []string, _ string) error {
		close(entered)
		<-release
		return nil
	}

	require.NoError(t, svc.ConfirmRun(t.Context(), runID))
	<-entered

	require.ErrorIs(t, svc.CancelRun(t.Context(), runID), ErrCannotCancelDuringDeletion)

	close(release)
	waitForRunStatus(t, store, runID, models.DeletePlanStatusCompleted)
}
