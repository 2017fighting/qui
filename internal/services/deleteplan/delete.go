// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package deleteplan

import (
	"context"
	"fmt"
	"strings"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/rs/zerolog/log"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/qbittorrent"
	"github.com/autobrr/qui/internal/services/contentgroup"
)

const deletionChunkSize = 50

type deleteTarget struct {
	rowID     int64
	hashes    []string
	sizeBytes int64
}

type deletionChunk struct {
	hashes []string
	rowIDs []int64
}

func (s *Service) executeDeletion(ctx context.Context, instanceID int, runID int64) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		log.Error().Err(err).Int64("run", runID).Msg("deleteplan: failed to load run for deletion")
		return
	}
	if run == nil || run.Status != models.DeletePlanStatusDeleting {
		return
	}

	if err := s.deleteGroups(ctx, instanceID, run); err != nil {
		s.failDeletion(ctx, instanceID, runID, err.Error())
	}
}

func (s *Service) deleteGroups(ctx context.Context, instanceID int, run *models.DeletePlanRun) error {
	refs, err := s.sync.GetAllTorrentRefs(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("read torrents: %w", err)
	}
	torrents := torrentValues(refs)

	idx := contentgroup.BuildIndex(torrents)

	var filesByHash map[string]qbt.TorrentFiles
	if shared := idx.SharedHashes(); len(shared) > 0 {
		filesCtx := qbittorrent.WithForceFilesRefresh(ctx)
		filesByHash, err = s.sync.GetTorrentFilesBatch(filesCtx, instanceID, shared)
		if err != nil {
			return fmt.Errorf("read torrent files: %w", err)
		}
	}

	rules := ProtectionRules{
		DeleteTag:        DeleteProtectionTag,
		RemoveDeadGroups: run.RemoveDeadGroups,
		FilesByHash:      filesByHash,
	}

	targets := make([]deleteTarget, 0, len(run.Groups))
	for _, row := range run.Groups {
		if !row.Included {
			continue
		}
		group, reason := recheckRow(row, idx, rules)
		if reason != "" {
			if err := s.store.UpdateRowOutcome(ctx, row.ID, models.DeletePlanRowStatusSkipped, reason, ""); err != nil {
				return err
			}
			continue
		}
		hashes := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			hashes = append(hashes, member.Hash)
		}
		targets = append(targets, deleteTarget{rowID: row.ID, hashes: hashes, sizeBytes: row.SizeBytes})
	}

	removedBytes, warning := s.removeTargets(ctx, instanceID, targets)
	return s.store.UpdateRunCompleted(ctx, run.ID, removedBytes, warning)
}

func (s *Service) removeTargets(ctx context.Context, instanceID int, targets []deleteTarget) (int64, string) {
	failed := make(map[int64]string)
	deleted := make(map[int64]struct{})
	var failures []string

	for _, chunk := range buildDeletionChunks(targets, deletionChunkSize) {
		if err := s.sync.BulkAction(ctx, instanceID, chunk.hashes, "deleteWithFiles"); err != nil {
			for _, rowID := range chunk.rowIDs {
				failed[rowID] = err.Error()
			}
			failures = append(failures, err.Error())
			continue
		}
		for _, rowID := range chunk.rowIDs {
			deleted[rowID] = struct{}{}
		}
	}

	var removedBytes int64
	for _, target := range targets {
		if message, bad := failed[target.rowID]; bad {
			s.recordRowOutcome(ctx, target.rowID, models.DeletePlanRowStatusFailed, "", message)
			continue
		}
		if _, ok := deleted[target.rowID]; !ok {
			continue
		}
		s.recordRowOutcome(ctx, target.rowID, models.DeletePlanRowStatusDeleted, "", "")
		removedBytes += target.sizeBytes
	}

	if len(failures) == 0 {
		return removedBytes, ""
	}
	return removedBytes, strings.Join(failures, "; ")
}

func (s *Service) recordRowOutcome(ctx context.Context, rowID int64, status, reason, errorMessage string) {
	if err := s.store.UpdateRowOutcome(ctx, rowID, status, reason, errorMessage); err != nil {
		log.Error().Err(err).Int64("row", rowID).Msg("deleteplan: failed to record row outcome")
	}
}

func (s *Service) failDeletion(ctx context.Context, instanceID int, runID int64, message string) {
	if err := s.store.UpdateRunFailed(ctx, runID, message); err != nil {
		log.Error().Err(err).Int64("run", runID).Msg("deleteplan: failed to mark deletion failed")
		return
	}
	s.emitRun(instanceID, runID)
}

func buildDeletionChunks(targets []deleteTarget, size int) []deletionChunk {
	var chunks []deletionChunk
	var current deletionChunk
	for _, target := range targets {
		for _, hash := range target.hashes {
			current.hashes = append(current.hashes, hash)
			current.rowIDs = append(current.rowIDs, target.rowID)
			if len(current.hashes) >= size {
				chunks = append(chunks, current)
				current = deletionChunk{}
			}
		}
	}
	if len(current.hashes) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

func recheckRow(row *models.DeletePlanRow, idx contentgroup.Index, rules ProtectionRules) (PlanGroup, string) {
	bucket := idx.GroupByPath(row.ContentPath)
	if len(bucket) == 0 {
		return PlanGroup{}, ReasonMembersChanged
	}
	group := buildGroup(bucket, idx, rules)
	if group.Excluded {
		return group, group.ExcludeReason
	}
	if !sameMembers(group.Members, row.Members) {
		return group, ReasonMembersChanged
	}
	return group, ""
}

func sameMembers(fresh []qbt.Torrent, frozen []models.DeletePlanMember) bool {
	if len(fresh) != len(frozen) {
		return false
	}
	hashes := make(map[string]struct{}, len(fresh))
	for _, member := range fresh {
		hashes[member.Hash] = struct{}{}
	}
	for _, member := range frozen {
		if _, ok := hashes[member.Hash]; !ok {
			return false
		}
	}
	return true
}
