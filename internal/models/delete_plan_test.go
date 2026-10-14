// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package models_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/database"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

func newDeletePlanFixture(t *testing.T, newDB func(testing.TB, string) *database.DB) (*models.InstanceStore, *models.DeletePlanStore, int) {
	t.Helper()
	ctx := t.Context()
	db := newDB(t, "delete-plan")
	instances, err := models.NewInstanceStore(db, []byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	instance, err := instances.Create(ctx, "test", "http://example.invalid", "user", "pass", nil, nil, false, nil)
	require.NoError(t, err)
	return instances, models.NewDeletePlanStore(db), instance.ID
}

func TestDeletePlanStore_OneActiveRunPerInstance(t *testing.T) {
	t.Parallel()
	runOneActiveRunRule(t, testdb.NewMigratedSQLite)
}

func TestDeletePlanStore_OneActiveRunPerInstancePostgresIntegration(t *testing.T) {
	t.Parallel()
	runOneActiveRunRule(t, testdb.NewMigratedPostgres)
}

func runOneActiveRunRule(t *testing.T, newDB func(testing.TB, string) *database.DB) {
	t.Helper()
	ctx := t.Context()
	_, store, instanceID := newDeletePlanFixture(t, newDB)

	runID, err := store.CreateRunIfNoActive(ctx, instanceID, 1000, "[]", true)
	require.NoError(t, err)
	require.Positive(t, runID)

	_, err = store.CreateRunIfNoActive(ctx, instanceID, 2000, "[]", true)
	require.ErrorIs(t, err, models.ErrDeletePlanAlreadyActive)

	// A canceled run no longer blocks a new plan.
	require.NoError(t, store.UpdateRunStatus(ctx, runID, models.DeletePlanStatusCanceled))
	secondID, err := store.CreateRunIfNoActive(ctx, instanceID, 2000, "[]", true)
	require.NoError(t, err)
	require.NotEqual(t, runID, secondID)
}

func TestDeletePlanStore_RoundTripsAFrozenPlan(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	_, store, instanceID := newDeletePlanFixture(t, testdb.NewMigratedSQLite)

	keysJSON := `[{"field":"GROUP_SIZE","enabled":true,"direction":"ASC"}]`
	runID, err := store.CreateRunIfNoActive(ctx, instanceID, 5000, keysJSON, true)
	require.NoError(t, err)

	run, err := store.GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, models.DeletePlanStatusPending, run.Status)
	require.True(t, run.WaitingForSync, "a pending run waits for fresh sync data")
	require.Equal(t, []models.KeyRule{{Field: models.FieldGroupSize, Enabled: true, Direction: models.SortDirectionASC}}, run.Keys)

	started, err := store.StartRunPlanning(ctx, runID)
	require.NoError(t, err)
	require.True(t, started)

	rows := []*models.DeletePlanRow{
		{
			ContentPath: "/data/Pack",
			SizeBytes:   500,
			MemberCount: 2,
			Included:    true,
			DecidingKey: string(models.FieldGroupSize),
			Members: []models.DeletePlanMember{
				{Hash: "a", Name: "Pack", SizeBytes: 500, Progress: 1, State: "uploading", Tags: []string{"cross-seed"}, SavePath: "/data"},
				{Hash: "b", Name: "Pack", SizeBytes: 400, Progress: 1, State: "uploading", Tags: []string{}, SavePath: "/data"},
			},
		},
		{
			ContentPath:   "/data/Protected",
			SizeBytes:     900,
			MemberCount:   1,
			Excluded:      true,
			ExcludeReason: "incomplete",
			Members: []models.DeletePlanMember{
				{Hash: "c", Name: "Protected", SizeBytes: 900, Progress: 0.5, State: "downloading", Tags: []string{}, SavePath: "/data"},
			},
		},
	}
	require.NoError(t, store.InsertRows(ctx, runID, rows))

	frozen, err := store.UpdateRunPreview(ctx, runID, 500, 0, 0, false)
	require.NoError(t, err)
	require.True(t, frozen)

	run, err = store.GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, models.DeletePlanStatusPreviewReady, run.Status)
	require.False(t, run.WaitingForSync)
	require.EqualValues(t, 500, run.PlannedBytes)
	require.False(t, run.TargetReached)
	require.Len(t, run.Groups, 1)
	require.Equal(t, "/data/Pack", run.Groups[0].ContentPath)
	require.Equal(t, 2, run.Groups[0].MemberCount)
	require.Equal(t, []string{"cross-seed"}, run.Groups[0].Members[0].Tags)
	require.Len(t, run.ExcludedGroups, 1)
	require.Equal(t, "incomplete", run.ExcludedGroups[0].ExcludeReason)

	// A second freeze cannot fire once the run left planning.
	frozen, err = store.UpdateRunPreview(ctx, runID, 1, 1, 1, true)
	require.NoError(t, err)
	require.False(t, frozen)
}

func TestDeletePlanStore_GetRunByInstanceScopesToTheInstance(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	instances, store, instanceID := newDeletePlanFixture(t, testdb.NewMigratedSQLite)

	other, err := instances.Create(ctx, "other", "http://example.invalid", "user", "pass", nil, nil, false, nil)
	require.NoError(t, err)

	runID, err := store.CreateRunIfNoActive(ctx, instanceID, 1000, "[]", true)
	require.NoError(t, err)

	scoped, err := store.GetRunByInstance(ctx, instanceID, runID)
	require.NoError(t, err)
	require.NotNil(t, scoped)

	scoped, err = store.GetRunByInstance(ctx, other.ID, runID)
	require.NoError(t, err)
	require.Nil(t, scoped)
}

// freezePreviewRun builds a plan run, inserts rows, and freezes it preview_ready.
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

func TestDeletePlanStore_DeletionStateTransitions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	_, store, instanceID := newDeletePlanFixture(t, testdb.NewMigratedSQLite)

	runID := freezePreviewRun(t, store, instanceID, []*models.DeletePlanRow{
		{
			ContentPath: "/data/A",
			SizeBytes:   500,
			MemberCount: 1,
			Included:    true,
			Members: []models.DeletePlanMember{
				{Hash: "a", SizeBytes: 500, Progress: 1, State: "uploading"},
			},
		},
	})

	deleting, err := store.StartRunDeletion(ctx, runID)
	require.NoError(t, err)
	require.True(t, deleting)

	// A second start cannot fire once the run is deleting.
	deleting, err = store.StartRunDeletion(ctx, runID)
	require.NoError(t, err)
	require.False(t, deleting)

	// Cancel is refused once deleting, so a deletion that began is not stopped.
	canceled, err := store.CancelRunIfNotDeleting(ctx, runID)
	require.NoError(t, err)
	require.False(t, canceled)

	// Rows start pending.
	run, err := store.GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, models.DeletePlanStatusDeleting, run.Status)
	require.Len(t, run.Groups, 1)
	require.Equal(t, models.DeletePlanRowStatusPending, run.Groups[0].RowStatus)

	require.NoError(t, store.UpdateRowOutcome(ctx, run.Groups[0].ID, models.DeletePlanRowStatusSkipped, "members_changed", ""))

	// A completed run carries removedBytes and, when chunks failed, a warning.
	require.NoError(t, store.UpdateRunCompleted(ctx, runID, 400, "one chunk failed"))

	run, err = store.GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, models.DeletePlanStatusCompleted, run.Status)
	require.EqualValues(t, 400, run.RemovedBytes)
	require.Equal(t, "one chunk failed", run.WarningMessage)
	require.Empty(t, run.ErrorMessage)
	require.NotNil(t, run.CompletedAt)
	require.Equal(t, models.DeletePlanRowStatusSkipped, run.Groups[0].RowStatus)
	require.Equal(t, "members_changed", run.Groups[0].RowReason)
}

func TestDeletePlanStore_CancelBeforeDeletion(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	_, store, instanceID := newDeletePlanFixture(t, testdb.NewMigratedSQLite)

	runID, err := store.CreateRunIfNoActive(ctx, instanceID, 1000, "[]", true)
	require.NoError(t, err)

	canceled, err := store.CancelRunIfNotDeleting(ctx, runID)
	require.NoError(t, err)
	require.True(t, canceled)

	run, err := store.GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, models.DeletePlanStatusCanceled, run.Status)
}

func TestDeletePlanStore_FailsADeletingRun(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	_, store, instanceID := newDeletePlanFixture(t, testdb.NewMigratedSQLite)

	runID := freezePreviewRun(t, store, instanceID, nil)
	_, err := store.StartRunDeletion(ctx, runID)
	require.NoError(t, err)

	require.NoError(t, store.UpdateRunFailed(ctx, runID, "read torrents: boom"))

	run, err := store.GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, models.DeletePlanStatusFailed, run.Status)
	require.Equal(t, "read torrents: boom", run.ErrorMessage)
	require.NotNil(t, run.CompletedAt)
}

func TestDeletePlanStore_MarkDeletingRunsFailed(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	_, store, instanceID := newDeletePlanFixture(t, testdb.NewMigratedSQLite)

	runID := freezePreviewRun(t, store, instanceID, nil)
	_, err := store.StartRunDeletion(ctx, runID)
	require.NoError(t, err)

	require.NoError(t, store.MarkDeletingRunsFailed(ctx, "Marked failed after restart"))

	run, err := store.GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, models.DeletePlanStatusFailed, run.Status)
	require.Equal(t, "Marked failed after restart", run.ErrorMessage)
}
