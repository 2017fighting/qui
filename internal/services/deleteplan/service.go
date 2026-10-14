// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package deleteplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/rs/zerolog/log"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/qbittorrent"
	"github.com/autobrr/qui/internal/services/activity"
	"github.com/autobrr/qui/internal/services/contentgroup"
)

// healthChecker is the subset of *qbittorrent.Client methods needed for readiness checks.
type healthChecker interface {
	IsHealthy() bool
	GetLastSyncUpdate() time.Time
}

// syncReader is the slice of the sync manager a delete plan reads. ADR 0005.
type syncReader interface {
	GetAllTorrents(ctx context.Context, instanceID int) ([]qbt.Torrent, error)
	GetTorrentFilesBatch(ctx context.Context, instanceID int, hashes []string) (map[string]qbt.TorrentFiles, error)
}

type clientReadiness interface {
	Client(ctx context.Context, instanceID int) (healthChecker, error)
}

type instanceReader interface {
	Get(ctx context.Context, id int) (*models.Instance, error)
}

// syncClients adapts the sync manager's concrete *Client to healthChecker.
type syncClients struct{ sm *qbittorrent.SyncManager }

func (c syncClients) Client(ctx context.Context, instanceID int) (healthChecker, error) {
	client, err := c.sm.GetClient(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// PlanRequest is a request to build a plan.
type PlanRequest struct {
	TargetBytes      int64
	Keys             []models.KeyRule
	RemoveDeadGroups bool
}

// Service builds delete plans in the background for one instance at a time.
type Service struct {
	cfg               Config
	store             *models.DeletePlanStore
	sync              syncReader
	clients           clientReadiness
	instances         instanceReader
	activityPublisher activity.Publisher

	// In-memory cancel handles keyed by runID.
	cancelFuncs map[int64]context.CancelFunc
	cancelMu    sync.Mutex
}

// NewService creates a new delete plan service. The concrete sync manager is
// accepted here only; the service reads it through narrow interfaces.
func NewService(cfg Config, instanceStore *models.InstanceStore, store *models.DeletePlanStore, syncManager *qbittorrent.SyncManager) *Service {
	defaults := DefaultConfig()
	if cfg.SyncPollInterval <= 0 {
		cfg.SyncPollInterval = defaults.SyncPollInterval
	}
	if cfg.StuckRunThreshold <= 0 {
		cfg.StuckRunThreshold = defaults.StuckRunThreshold
	}
	return &Service{
		cfg:               cfg,
		store:             store,
		sync:              syncManager,
		clients:           syncClients{sm: syncManager},
		instances:         instanceStore,
		activityPublisher: activity.NopPublisher{},
		cancelFuncs:       make(map[int64]context.CancelFunc),
	}
}

// SetActivityPublisher wires the server-event hub so run status transitions are
// pushed to connected clients. Safe to call once at startup.
func (s *Service) SetActivityPublisher(publisher activity.Publisher) {
	if s == nil || publisher == nil {
		return
	}
	s.activityPublisher = publisher
}

func (s *Service) emitRun(instanceID int, runID int64) {
	if s == nil || s.activityPublisher == nil {
		return
	}
	s.activityPublisher.Publish(activity.Event{
		Kind:       activity.KindDeletePlanRun,
		InstanceID: instanceID,
		ResourceID: strconv.FormatInt(runID, 10),
	})
}

// Start recovers runs left pending or planning by a restart. A plan is manual,
// so there is no scheduler to start.
func (s *Service) Start(ctx context.Context) {
	if s == nil {
		return
	}
	statuses := []string{models.DeletePlanStatusPending, models.DeletePlanStatusPlanning}
	if err := s.store.MarkStuckRunsFailed(ctx, s.cfg.StuckRunThreshold, statuses); err != nil {
		log.Error().Err(err).Msg("deleteplan: failed to recover stuck runs")
	}
}

// TriggerPlan creates a run and builds its plan in the background. It returns
// ErrPlanAlreadyActive when the instance already has an active run.
func (s *Service) TriggerPlan(ctx context.Context, instanceID int, req PlanRequest) (int64, error) {
	if req.TargetBytes <= 0 {
		return 0, ErrInvalidTarget
	}
	if _, err := s.instances.Get(ctx, instanceID); err != nil {
		return 0, err
	}

	keys := req.Keys
	if len(keys) == 0 {
		keys = DefaultKeys()
	}
	keysJSON, err := json.Marshal(keys)
	if err != nil {
		return 0, fmt.Errorf("marshal delete plan keys: %w", err)
	}

	runID, err := s.store.CreateRunIfNoActive(ctx, instanceID, req.TargetBytes, string(keysJSON), req.RemoveDeadGroups)
	if errors.Is(err, models.ErrDeletePlanAlreadyActive) {
		return 0, ErrPlanAlreadyActive
	}
	if err != nil {
		return 0, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.cancelMu.Lock()
	s.cancelFuncs[runID] = cancel
	s.cancelMu.Unlock()

	s.emitRun(instanceID, runID)

	go func() {
		defer func() {
			s.cancelMu.Lock()
			delete(s.cancelFuncs, runID)
			s.cancelMu.Unlock()
		}()
		s.executePlan(runCtx, instanceID, runID)
	}()

	return runID, nil
}

// CancelRun cancels a pending, planning, or preview_ready run.
func (s *Service) CancelRun(ctx context.Context, runID int64) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return ErrPlanRunNotFound
	}

	switch run.Status {
	case models.DeletePlanStatusPending, models.DeletePlanStatusPlanning, models.DeletePlanStatusPreviewReady:
		s.cancelMu.Lock()
		if cancel, ok := s.cancelFuncs[runID]; ok {
			cancel()
		}
		s.cancelMu.Unlock()

		if err := s.store.UpdateRunStatus(ctx, runID, models.DeletePlanStatusCanceled); err != nil {
			return err
		}
		s.emitRun(run.InstanceID, runID)
		return nil
	case models.DeletePlanStatusDeleting:
		return ErrCannotCancelDuringDeletion
	case models.DeletePlanStatusCompleted, models.DeletePlanStatusFailed, models.DeletePlanStatusCanceled:
		return fmt.Errorf("%w: %s", ErrPlanRunAlreadyFinished, run.Status)
	default:
		return fmt.Errorf("%w: %s", ErrPlanRunInvalidStatus, run.Status)
	}
}

func (s *Service) executePlan(ctx context.Context, instanceID int, runID int64) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		log.Error().Err(err).Int64("run", runID).Msg("deleteplan: failed to load run")
		return
	}
	if run == nil || run.Status != models.DeletePlanStatusPending {
		return
	}

	if err := s.waitForFreshSync(ctx, instanceID); err != nil {
		// Canceled while waiting: CancelRun already set the final status.
		return
	}

	started, err := s.store.StartRunPlanning(ctx, runID)
	if err != nil {
		log.Error().Err(err).Int64("run", runID).Msg("deleteplan: failed to start planning")
		return
	}
	if !started {
		// Canceled while waiting for sync data.
		return
	}
	s.emitRun(instanceID, runID)

	plan, err := s.buildPlan(ctx, instanceID, run)
	if err != nil {
		s.failRun(ctx, instanceID, runID, err.Error())
		return
	}

	if err := s.store.InsertRows(ctx, runID, planRows(plan)); err != nil {
		s.failRun(ctx, instanceID, runID, err.Error())
		return
	}

	frozen, err := s.store.UpdateRunPreview(ctx, runID, plan.PlannedBytes, plan.DeadBytes, plan.OvershootBytes, plan.TargetReached)
	if err != nil {
		log.Error().Err(err).Int64("run", runID).Msg("deleteplan: failed to freeze the plan")
		return
	}
	if frozen {
		s.emitRun(instanceID, runID)
	}
}

// waitForFreshSync blocks until the instance has sync data no older than
// MaxSyncAge or the run is canceled. A pending run waits rather than ranking a
// stale torrent list.
func (s *Service) waitForFreshSync(ctx context.Context, instanceID int) error {
	for {
		if s.syncIsFresh(ctx, instanceID) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.cfg.SyncPollInterval):
		}
	}
}

func (s *Service) syncIsFresh(ctx context.Context, instanceID int) bool {
	client, err := s.clients.Client(ctx, instanceID)
	if err != nil {
		return false
	}
	if !client.IsHealthy() {
		return false
	}
	lastSync := client.GetLastSyncUpdate()
	return !lastSync.IsZero() && time.Since(lastSync) <= MaxSyncAge
}

func (s *Service) buildPlan(ctx context.Context, instanceID int, run *models.DeletePlanRun) (Plan, error) {
	torrents, err := s.sync.GetAllTorrents(ctx, instanceID)
	if err != nil {
		return Plan{}, fmt.Errorf("read torrents: %w", err)
	}

	idx := contentgroup.BuildIndex(torrents)

	var filesByHash map[string]qbt.TorrentFiles
	if shared := idx.SharedHashes(); len(shared) > 0 {
		filesCtx := qbittorrent.WithForceFilesRefresh(ctx)
		filesByHash, err = s.sync.GetTorrentFilesBatch(filesCtx, instanceID, shared)
		if err != nil {
			return Plan{}, fmt.Errorf("read torrent files: %w", err)
		}
	}

	return BuildPlan(torrents, idx, run.Keys, run.TargetBytes, ProtectionRules{
		DeleteTag:        DeleteProtectionTag,
		RemoveDeadGroups: run.RemoveDeadGroups,
		FilesByHash:      filesByHash,
	}), nil
}

func (s *Service) failRun(ctx context.Context, instanceID int, runID int64, message string) {
	if err := s.store.UpdateRunFailed(ctx, runID, message); err != nil {
		log.Error().Err(err).Int64("run", runID).Msg("deleteplan: failed to mark run failed")
		return
	}
	s.emitRun(instanceID, runID)
}

func planRows(plan Plan) []*models.DeletePlanRow {
	rows := make([]*models.DeletePlanRow, 0, len(plan.Groups)+len(plan.ExcludedGroups))
	for _, group := range plan.Groups {
		rows = append(rows, planRow(group))
	}
	for _, group := range plan.ExcludedGroups {
		rows = append(rows, planRow(group))
	}
	return rows
}

func planRow(group PlanGroup) *models.DeletePlanRow {
	members := make([]models.DeletePlanMember, 0, len(group.Members))
	for _, member := range group.Members {
		members = append(members, models.DeletePlanMember{
			Hash:      member.Hash,
			Name:      member.Name,
			SizeBytes: member.Size,
			Progress:  member.Progress,
			State:     string(member.State),
			Tags:      splitTags(member.Tags),
			SavePath:  member.SavePath,
		})
	}
	return &models.DeletePlanRow{
		ContentPath:   group.ContentPath,
		SizeBytes:     group.SizeBytes,
		MemberCount:   len(group.Members),
		Dead:          group.Dead,
		Included:      group.Included,
		Excluded:      group.Excluded,
		ExcludeReason: group.ExcludeReason,
		DecidingKey:   string(group.DecidingKey),
		Members:       members,
	}
}

func splitTags(tags string) []string {
	if strings.TrimSpace(tags) == "" {
		return []string{}
	}
	parts := strings.Split(tags, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
