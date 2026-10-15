// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/autobrr/qui/internal/dbinterface"
)

// Delete plan run statuses. One active run per instance.
const (
	DeletePlanStatusPending      = "pending"
	DeletePlanStatusPlanning     = "planning"
	DeletePlanStatusPreviewReady = "preview_ready"
	DeletePlanStatusDeleting     = "deleting"
	DeletePlanStatusCompleted    = "completed"
	DeletePlanStatusFailed       = "failed"
	DeletePlanStatusCanceled     = "canceled"
)

// Delete plan row removal outcomes. A row is pending until the deletion reaches it.
const (
	DeletePlanRowStatusPending = "pending"
	DeletePlanRowStatusDeleted = "deleted"
	DeletePlanRowStatusSkipped = "skipped"
	DeletePlanRowStatusFailed  = "failed"
)

// ErrDeletePlanAlreadyActive is returned when a run is created while one is active.
var ErrDeletePlanAlreadyActive = errors.New("a delete plan is already active for this instance")

// DeletePlanRun is a delete plan run and, once built, its frozen rows.
type DeletePlanRun struct {
	ID               int64     `json:"id"`
	InstanceID       int       `json:"instanceId"`
	Status           string    `json:"status"`
	TargetBytes      int64     `json:"targetBytes"`
	Keys             []KeyRule `json:"keys"`
	RemoveDeadGroups bool      `json:"removeDeadGroups"`
	WaitingForSync   bool      `json:"waitingForSync"`
	PlannedBytes     int64     `json:"plannedBytes"`
	DeadBytes        int64     `json:"deadBytes"`
	OvershootBytes   int64     `json:"overshootBytes"`
	// ShortfallBytes is targetBytes - plannedBytes when positive, else 0. It is
	// derived on read, so an untick and a refill keep it consistent with the rows.
	ShortfallBytes int64  `json:"shortfallBytes"`
	TargetReached  bool   `json:"targetReached"`
	RemovedBytes   int64  `json:"removedBytes"`
	ErrorMessage   string `json:"errorMessage"`
	// WarningMessage carries the failure summary of a completed run that lost some
	// chunks. ErrorMessage stays empty on a completed run.
	WarningMessage string     `json:"warningMessage"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	CompletedAt    *time.Time `json:"completedAt"`
	// Groups holds the non-excluded groups in plan order, included and candidates
	// alike; ExcludedGroups holds the groups dropped by a protection. Both are
	// filled by GetRun.
	Groups         []*DeletePlanRow `json:"groups"`
	ExcludedGroups []*DeletePlanRow `json:"excludedGroups"`
}

// DeletePlanRow is one frozen content group in a delete plan.
type DeletePlanRow struct {
	ID          int64  `json:"id"`
	RunID       int64  `json:"runId"`
	ContentPath string `json:"contentPath"`
	SizeBytes   int64  `json:"sizeBytes"`
	MemberCount int    `json:"memberCount"`
	Dead        bool   `json:"dead"`
	Included    bool   `json:"included"`
	// Unticked marks a group the user vetoed. A refill never re-includes it.
	Unticked      bool               `json:"unticked"`
	Excluded      bool               `json:"excluded"`
	ExcludeReason string             `json:"excludeReason"`
	DecidingKey   string             `json:"decidingKey"`
	Members       []DeletePlanMember `json:"members"`
	// Removal outcome, filled by the deletion. RowStatus is pending until then.
	RowStatus string `json:"rowStatus"`
	RowReason string `json:"rowReason"`
	RowError  string `json:"rowError"`
}

// DeletePlanMember is one torrent in a content group.
type DeletePlanMember struct {
	Hash      string   `json:"hash"`
	Name      string   `json:"name"`
	SizeBytes int64    `json:"sizeBytes"`
	Progress  float64  `json:"progress"`
	State     string   `json:"state"`
	Tags      []string `json:"tags"`
	SavePath  string   `json:"savePath"`
}

// DeletePlanStore handles database operations for delete plans.
type DeletePlanStore struct {
	db dbinterface.Querier
}

// NewDeletePlanStore creates a new DeletePlanStore.
func NewDeletePlanStore(db dbinterface.Querier) *DeletePlanStore {
	return &DeletePlanStore{db: db}
}

// CreateRunIfNoActive atomically checks for an active run and creates a pending
// one if none exists. The active set is pending, planning, preview_ready and
// deleting, so a second plan waits until the first is confirmed or canceled.
func (s *DeletePlanStore) CreateRunIfNoActive(ctx context.Context, instanceID int, targetBytes int64, keysJSON string, removeDeadGroups bool) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin delete plan run: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO delete_plan_runs (instance_id, status, target_bytes, keys_json, remove_dead_groups)
		SELECT ?, 'pending', ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM delete_plan_runs
			WHERE instance_id = ?
			  AND status IN ('pending', 'planning', 'preview_ready', 'deleting')
		)
		RETURNING id
	`, instanceID, targetBytes, keysJSON, boolToInt(removeDeadGroups), instanceID).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrDeletePlanAlreadyActive
		}
		return 0, fmt.Errorf("insert delete plan run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit delete plan run: %w", err)
	}
	return id, nil
}

const deletePlanRunColumns = `id, instance_id, status, target_bytes, keys_json, remove_dead_groups,
	planned_bytes, dead_bytes, overshoot_bytes, target_reached, removed_bytes,
	error_message, warning_message, created_at, updated_at, completed_at`

// GetRun retrieves a run with its frozen rows.
func (s *DeletePlanStore) GetRun(ctx context.Context, runID int64) (*DeletePlanRun, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+deletePlanRunColumns+` FROM delete_plan_runs WHERE id = ?`, runID)
	run, err := scanDeletePlanRun(row)
	if err != nil || run == nil {
		return run, err
	}
	if err := s.loadRows(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

// GetRunByInstance retrieves a run with its frozen rows, scoped to an instance,
// so a handler never leaks another instance's run.
func (s *DeletePlanStore) GetRunByInstance(ctx context.Context, instanceID int, runID int64) (*DeletePlanRun, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+deletePlanRunColumns+` FROM delete_plan_runs WHERE id = ? AND instance_id = ?`, runID, instanceID)
	run, err := scanDeletePlanRun(row)
	if err != nil || run == nil {
		return run, err
	}
	if err := s.loadRows(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

// ListRuns lists recent runs for an instance, newest first, without rows.
func (s *DeletePlanStore) ListRuns(ctx context.Context, instanceID, limit int) ([]*DeletePlanRun, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+deletePlanRunColumns+`
		FROM delete_plan_runs
		WHERE instance_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, instanceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query delete plan runs: %w", err)
	}
	defer rows.Close()

	var runs []*DeletePlanRun
	for rows.Next() {
		run, err := scanDeletePlanRunRows(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate delete plan runs: %w", err)
	}
	return runs, nil
}

func (s *DeletePlanStore) loadRows(ctx context.Context, run *DeletePlanRun) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, content_path, size_bytes, member_count, dead, included,
		       unticked, excluded, exclude_reason, deciding_key, members, row_status, row_reason, row_error
		FROM delete_plan_rows
		WHERE run_id = ?
		ORDER BY id
	`, run.ID)
	if err != nil {
		return fmt.Errorf("query delete plan rows: %w", err)
	}
	defer rows.Close()

	run.Groups = []*DeletePlanRow{}
	run.ExcludedGroups = []*DeletePlanRow{}
	for rows.Next() {
		planRow, err := scanDeletePlanRow(rows)
		if err != nil {
			return err
		}
		if planRow.Excluded {
			run.ExcludedGroups = append(run.ExcludedGroups, planRow)
			continue
		}
		run.Groups = append(run.Groups, planRow)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate delete plan rows: %w", err)
	}
	return nil
}

// UpdateRowSelection replaces a preview_ready run's unticked set and its selected
// rows in one transaction: every non-excluded row is cleared, then the unticked
// rows are vetoed and the included rows are selected. A candidate is neither, so
// it stays out of the plan until a refill selects it. It reports whether the run
// was still preview_ready, so a racing confirm or cancel leaves the rows untouched.
func (s *DeletePlanStore) UpdateRowSelection(ctx context.Context, runID int64, unticked, included []int64, plannedBytes, deadBytes, overshootBytes int64, targetReached bool) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin delete plan row selection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE delete_plan_runs
		SET planned_bytes = ?, dead_bytes = ?, overshoot_bytes = ?, target_reached = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'preview_ready'
	`, plannedBytes, deadBytes, overshootBytes, boolToInt(targetReached), runID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE delete_plan_rows SET included = 0, unticked = 0 WHERE run_id = ? AND excluded = 0
	`, runID); err != nil {
		return false, err
	}

	if err := setRowFlag(ctx, tx, runID, "unticked", unticked); err != nil {
		return false, err
	}
	if err := setRowFlag(ctx, tx, runID, "included", included); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit delete plan row selection: %w", err)
	}
	return true, nil
}

// setRowFlag sets one boolean row flag on the given non-excluded rows.
func setRowFlag(ctx context.Context, tx dbinterface.TxQuerier, runID int64, column string, rowIDs []int64) error {
	if len(rowIDs) == 0 {
		return nil
	}

	var placeholders strings.Builder
	args := make([]any, 0, len(rowIDs)+1)
	args = append(args, runID)
	for i, rowID := range rowIDs {
		if i > 0 {
			placeholders.WriteString(", ")
		}
		placeholders.WriteString("?")
		args = append(args, rowID)
	}

	_, err := tx.ExecContext(ctx, `
		UPDATE delete_plan_rows SET `+column+` = 1
		WHERE run_id = ? AND excluded = 0 AND id IN (`+placeholders.String()+`)
	`, args...)
	return err
}

// StartRunPlanning moves a run from pending to planning, and reports whether it did.
func (s *DeletePlanStore) StartRunPlanning(ctx context.Context, runID int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE delete_plan_runs SET status = 'planning', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'pending'
	`, runID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateRunPreview freezes the plan on a planning run, and reports whether it did.
func (s *DeletePlanStore) UpdateRunPreview(ctx context.Context, runID, plannedBytes, deadBytes, overshootBytes int64, targetReached bool) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE delete_plan_runs
		SET status = 'preview_ready', planned_bytes = ?, dead_bytes = ?, overshoot_bytes = ?,
		    target_reached = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'planning'
	`, plannedBytes, deadBytes, overshootBytes, boolToInt(targetReached), runID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateRunCompleted marks a deleting run completed with the bytes it removed and,
// when some chunks failed, the failure summary. ErrorMessage stays empty.
func (s *DeletePlanStore) UpdateRunCompleted(ctx context.Context, runID, removedBytes int64, warningMessage string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE delete_plan_runs
		SET status = 'completed', removed_bytes = ?, warning_message = ?, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'deleting'
	`, removedBytes, warningMessage, runID)
	return err
}

// StartRunDeletion moves a run from preview_ready to deleting, and reports whether it did.
func (s *DeletePlanStore) StartRunDeletion(ctx context.Context, runID int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE delete_plan_runs SET status = 'deleting', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'preview_ready'
	`, runID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateRunStatus updates the status of a run.
func (s *DeletePlanStore) UpdateRunStatus(ctx context.Context, runID int64, status string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE delete_plan_runs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, status, runID)
	return err
}

// CancelRunIfNotDeleting cancels a run that has not started deleting, and reports
// whether it did. A run that reached deleting keeps that status, so cancel cannot
// stop a deletion that already began.
func (s *DeletePlanStore) CancelRunIfNotDeleting(ctx context.Context, runID int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE delete_plan_runs SET status = 'canceled', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status IN ('pending', 'planning', 'preview_ready')
	`, runID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateRunFailed marks a pending, planning, or deleting run failed, so a canceled
// run keeps its status.
func (s *DeletePlanStore) UpdateRunFailed(ctx context.Context, runID int64, errorMessage string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE delete_plan_runs
		SET status = 'failed', error_message = ?, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status IN ('pending', 'planning', 'deleting')
	`, errorMessage, runID)
	return err
}

// MarkDeletingRunsFailed marks runs left deleting by a restart as failed, so an
// interrupted deletion does not stay in a non-terminal state.
func (s *DeletePlanStore) MarkDeletingRunsFailed(ctx context.Context, errorMessage string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE delete_plan_runs
		SET status = 'failed', error_message = ?, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE status = 'deleting'
	`, errorMessage)
	return err
}

// UpdateRowOutcome records one row's removal outcome. reason is set on a skip,
// errorMessage on a failed chunk.
func (s *DeletePlanStore) UpdateRowOutcome(ctx context.Context, rowID int64, status, reason, errorMessage string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE delete_plan_rows SET row_status = ?, row_reason = ?, row_error = ? WHERE id = ?
	`, status, reason, errorMessage, rowID)
	return err
}

// MarkStuckRunsFailed marks pending/planning runs older than the threshold as
// failed, so a restart does not leave them stuck.
func (s *DeletePlanStore) MarkStuckRunsFailed(ctx context.Context, threshold time.Duration, statuses []string) error {
	// created_at is written by CURRENT_TIMESTAMP, so compare in the same UTC format.
	cutoff := time.Now().Add(-threshold).UTC().Format(time.DateTime)

	var placeholders strings.Builder
	args := make([]any, 0, len(statuses)+1)
	args = append(args, cutoff)
	for i, status := range statuses {
		if i > 0 {
			placeholders.WriteString(", ")
		}
		placeholders.WriteString("?")
		args = append(args, status)
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE delete_plan_runs
		SET status = 'failed', error_message = 'Marked failed after restart', completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE created_at < ? AND status IN (`+placeholders.String()+`)
	`, args...)
	return err
}

// InsertRows inserts the frozen plan rows in batches of 100.
func (s *DeletePlanStore) InsertRows(ctx context.Context, runID int64, rows []*DeletePlanRow) error {
	if len(rows) == 0 {
		return nil
	}

	const batchSize = 100
	for i := 0; i < len(rows); i += batchSize {
		end := min(i+batchSize, len(rows))
		batch := rows[i:end]

		var query strings.Builder
		query.WriteString(`INSERT INTO delete_plan_rows
			(run_id, content_path, size_bytes, member_count, dead, included, unticked, excluded, exclude_reason, deciding_key, members)
			VALUES `)
		args := make([]any, 0, len(batch)*11)
		for j, planRow := range batch {
			if j > 0 {
				query.WriteString(", ")
			}
			query.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
			membersJSON, err := json.Marshal(planRow.Members)
			if err != nil {
				return fmt.Errorf("marshal delete plan row members: %w", err)
			}
			args = append(args, runID, planRow.ContentPath, planRow.SizeBytes, planRow.MemberCount,
				boolToInt(planRow.Dead), boolToInt(planRow.Included), boolToInt(planRow.Unticked), boolToInt(planRow.Excluded),
				planRow.ExcludeReason, planRow.DecidingKey, string(membersJSON))
		}

		if _, err := s.db.ExecContext(ctx, query.String(), args...); err != nil {
			return fmt.Errorf("insert delete plan rows: %w", err)
		}
	}

	return nil
}

func scanDeletePlanRun(row *sql.Row) (*DeletePlanRun, error) {
	run, err := scanDeletePlanRunScanner(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

func scanDeletePlanRunRows(rows *sql.Rows) (*DeletePlanRun, error) {
	return scanDeletePlanRunScanner(rows.Scan)
}

func scanDeletePlanRunScanner(scan func(dest ...any) error) (*DeletePlanRun, error) {
	var run DeletePlanRun
	var keysJSON sql.NullString
	var removeDeadGroups, targetReached int
	var errorMessage sql.NullString
	var completedAt sql.NullTime

	if err := scan(
		&run.ID,
		&run.InstanceID,
		&run.Status,
		&run.TargetBytes,
		&keysJSON,
		&removeDeadGroups,
		&run.PlannedBytes,
		&run.DeadBytes,
		&run.OvershootBytes,
		&targetReached,
		&run.RemovedBytes,
		&errorMessage,
		&run.WarningMessage,
		&run.CreatedAt,
		&run.UpdatedAt,
		&completedAt,
	); err != nil {
		return nil, err
	}

	if keysJSON.Valid && keysJSON.String != "" {
		if err := json.Unmarshal([]byte(keysJSON.String), &run.Keys); err != nil {
			return nil, fmt.Errorf("unmarshal delete plan keys: %w", err)
		}
	}
	if run.Keys == nil {
		run.Keys = []KeyRule{}
	}
	run.RemoveDeadGroups = SQLiteIntToBool(removeDeadGroups)
	run.TargetReached = SQLiteIntToBool(targetReached)
	run.ShortfallBytes = max(run.TargetBytes-run.PlannedBytes, 0)
	if errorMessage.Valid {
		run.ErrorMessage = errorMessage.String
	}
	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}
	// A run waits for fresh sync data only while it is pending.
	run.WaitingForSync = run.Status == DeletePlanStatusPending
	run.Groups = []*DeletePlanRow{}
	run.ExcludedGroups = []*DeletePlanRow{}
	return &run, nil
}

func scanDeletePlanRow(rows *sql.Rows) (*DeletePlanRow, error) {
	var planRow DeletePlanRow
	var dead, included, unticked, excluded int
	var membersJSON sql.NullString

	if err := rows.Scan(
		&planRow.ID,
		&planRow.RunID,
		&planRow.ContentPath,
		&planRow.SizeBytes,
		&planRow.MemberCount,
		&dead,
		&included,
		&unticked,
		&excluded,
		&planRow.ExcludeReason,
		&planRow.DecidingKey,
		&membersJSON,
		&planRow.RowStatus,
		&planRow.RowReason,
		&planRow.RowError,
	); err != nil {
		return nil, fmt.Errorf("scan delete plan row: %w", err)
	}

	planRow.Dead = SQLiteIntToBool(dead)
	planRow.Included = SQLiteIntToBool(included)
	planRow.Unticked = SQLiteIntToBool(unticked)
	planRow.Excluded = SQLiteIntToBool(excluded)
	if membersJSON.Valid && membersJSON.String != "" {
		if err := json.Unmarshal([]byte(membersJSON.String), &planRow.Members); err != nil {
			return nil, fmt.Errorf("unmarshal delete plan members: %w", err)
		}
	}
	if planRow.Members == nil {
		planRow.Members = []DeletePlanMember{}
	}
	return &planRow, nil
}
