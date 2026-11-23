// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package contentgroup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/rs/zerolog/log"

	"github.com/autobrr/qui/internal/models"
)

const MinFileOverlapPercent = 90

const OverlapUnknown = -1

func ResolvedTorrentFilePath(torrent qbt.Torrent, fileName string) string {
	savePath := NormalizePath(torrent.SavePath)
	relativePath := strings.TrimPrefix(NormalizePath(fileName), "/")
	if savePath == "" {
		return relativePath
	}
	if relativePath == "" {
		return savePath
	}
	return savePath + "/" + relativePath
}

type FileOverlapIndex struct {
	sizeByPath map[string]int64
	totalBytes int64
}

func BuildFileOverlapIndex(torrent qbt.Torrent, files qbt.TorrentFiles) FileOverlapIndex {
	index := FileOverlapIndex{sizeByPath: make(map[string]int64, len(files))}
	for _, file := range files {
		index.sizeByPath[ResolvedTorrentFilePath(torrent, file.Name)] = file.Size
		index.totalBytes += file.Size
	}
	return index
}

func FileOverlapPercent(triggerFiles FileOverlapIndex, candidate qbt.Torrent, candidateFiles qbt.TorrentFiles) int64 {
	if len(triggerFiles.sizeByPath) == 0 || len(candidateFiles) == 0 {
		return OverlapUnknown
	}

	var candidateBytes, matchedBytes int64
	for _, file := range candidateFiles {
		candidateBytes += file.Size
		if triggerSize, exists := triggerFiles.sizeByPath[ResolvedTorrentFilePath(candidate, file.Name)]; exists {
			matchedBytes += min(triggerSize, file.Size)
		}
	}

	smallerBytes := min(triggerFiles.totalBytes, candidateBytes)
	if smallerBytes <= 0 {
		return OverlapUnknown
	}
	return (matchedBytes * 100) / smallerBytes
}

type FileFetcher func(hashes []string) (map[string]qbt.TorrentFiles, error)

func cachedFileFetcher(filesByHash map[string]qbt.TorrentFiles) FileFetcher {
	return func(_ []string) (map[string]qbt.TorrentFiles, error) {
		return filesByHash, nil
	}
}

func VerifiedMembers(
	trigger qbt.Torrent,
	group []qbt.Torrent,
	alreadyIncludedHashes map[string]struct{},
	fetchFiles FileFetcher,
) ([]string, bool) {
	candidates := make([]qbt.Torrent, 0, len(group))
	for _, other := range group {
		if other.Hash == trigger.Hash {
			continue
		}
		if _, processed := alreadyIncludedHashes[other.Hash]; processed {
			continue
		}
		candidates = append(candidates, other)
	}
	if len(candidates) == 0 {
		return []string{trigger.Hash}, true
	}

	hashes := make([]string, 0, len(candidates)+1)
	hashes = append(hashes, trigger.Hash)
	for _, candidate := range candidates {
		hashes = append(hashes, candidate.Hash)
	}
	filesByHash, err := fetchFiles(hashes)
	if err != nil {
		log.Warn().Err(err).Str("hash", trigger.Hash).
			Msg("contentgroup: skipping cross-seed group, could not read its files")
		return nil, false
	}

	triggerFiles := BuildFileOverlapIndex(trigger, filesByHash[trigger.Hash])
	verified := []string{trigger.Hash}
	for _, candidate := range candidates {
		percent := FileOverlapPercent(triggerFiles, candidate, filesByHash[candidate.Hash])
		switch {
		case percent >= MinFileOverlapPercent:
			verified = append(verified, candidate.Hash)
		case percent == 0:

			log.Debug().Str("hash", trigger.Hash).Str("otherHash", candidate.Hash).
				Str("contentPath", trigger.ContentPath).
				Msg("contentgroup: excluding torrent that shares the content path but no files")
		default:
			log.Warn().Str("hash", trigger.Hash).Str("otherHash", candidate.Hash).Int64("overlapPercent", percent).
				Msg("contentgroup: skipping entire group due to partial file overlap")
			return nil, false
		}
	}
	return verified, true
}

func ShouldPreserveCrossSeedFiles(torrent qbt.Torrent, idx Index, filesByHash map[string]qbt.TorrentFiles) bool {
	verifiedHashes, ok := VerifiedMembers(
		torrent,
		idx.Group(torrent),
		nil,
		cachedFileFetcher(filesByHash),
	)
	return !ok || len(verifiedHashes) > 1
}

type BatchFileFetcher func(ctx context.Context, hashes []string) (map[string]qbt.TorrentFiles, error)

func VerifyFileOverlap(ctx context.Context, fetch BatchFileFetcher, torrent1, torrent2 qbt.Torrent, minOverlapPercent int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if fetch == nil {
		return false, errors.New("no file fetcher")
	}

	filesByHash, err := fetch(ctx, []string{torrent1.Hash, torrent2.Hash})
	if err != nil {
		return false, fmt.Errorf("failed to fetch files: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return false, err
	}

	if minOverlapPercent <= 0 {
		minOverlapPercent = MinFileOverlapPercent
	}
	percent := FileOverlapPercent(BuildFileOverlapIndex(torrent1, filesByHash[torrent1.Hash]), torrent2, filesByHash[torrent2.Hash])
	if percent == OverlapUnknown {
		return false, errors.New("cannot compute overlap: missing or zero-size file lists")
	}
	return percent >= int64(minOverlapPercent), nil
}

const KeyContentPath = "contentPath"

const (
	AmbiguityVerifyOverlap = "verify_overlap"
	AmbiguitySkip          = "skip"
)

type GroupIndex interface {
	IsAmbiguousForHash(hash string) bool
	MembersForHash(hash string) []string
}

func ShouldExpandAmbiguousGroup(
	ctx context.Context,
	fetch BatchFileFetcher,
	def *models.GroupDefinition,
	idx GroupIndex,
	triggerHash string,
	torrentByHash map[string]qbt.Torrent,
) bool {
	if idx == nil || triggerHash == "" {
		return true
	}
	if !idx.IsAmbiguousForHash(triggerHash) {
		return true
	}
	if def == nil || !slices.Contains(def.Keys, KeyContentPath) {
		return true
	}

	policy := strings.TrimSpace(def.AmbiguousPolicy)
	if policy == "" {
		policy = AmbiguityVerifyOverlap
	}
	if policy == AmbiguitySkip {
		return false
	}

	if fetch == nil {
		return false
	}
	triggerTorrent, ok := torrentByHash[triggerHash]
	if !ok {
		return false
	}

	minPercent := def.MinFileOverlapPercent
	if minPercent <= 0 {
		minPercent = MinFileOverlapPercent
	}

	for _, otherHash := range idx.MembersForHash(triggerHash) {
		if otherHash == triggerHash {
			continue
		}
		otherTorrent, ok := torrentByHash[otherHash]
		if !ok {
			return false
		}
		hasOverlap, err := VerifyFileOverlap(ctx, fetch, triggerTorrent, otherTorrent, minPercent)
		if err != nil || !hasOverlap {
			return false
		}
	}
	return true
}
