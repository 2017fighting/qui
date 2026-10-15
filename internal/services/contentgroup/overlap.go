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

// MinFileOverlapPercent is the minimum percentage of file overlap required
// to consider two torrents as sharing the same files.
// 90% tolerates small differences (extra NFO/sample/metadata files) while preventing
// accidental grouping of unrelated torrents that happen to share the same path.
const MinFileOverlapPercent = 90

// OverlapUnknown is returned when the overlap between two file lists cannot be
// computed, for example when one of the lists is empty or has no bytes.
const OverlapUnknown = -1

// ResolvedTorrentFilePath resolves a torrent-internal file name against the torrent's
// SavePath, so two torrents with different layouts can still meet on the same on-disk path.
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

// FileOverlapIndex records one torrent's files by their resolved on-disk path.
type FileOverlapIndex struct {
	sizeByPath map[string]int64
	totalBytes int64
}

// BuildFileOverlapIndex indexes a torrent's file list by resolved on-disk path.
func BuildFileOverlapIndex(torrent qbt.Torrent, files qbt.TorrentFiles) FileOverlapIndex {
	index := FileOverlapIndex{sizeByPath: make(map[string]int64, len(files))}
	for _, file := range files {
		index.sizeByPath[ResolvedTorrentFilePath(torrent, file.Name)] = file.Size
		index.totalBytes += file.Size
	}
	return index
}

// FileOverlapPercent returns how much of the smaller torrent, in bytes, resolves
// to the same on-disk paths. Returns OverlapUnknown when comparison is not possible.
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

// FileFetcher returns the file lists for the given hashes.
type FileFetcher func(hashes []string) (map[string]qbt.TorrentFiles, error)

// cachedFileFetcher serves file lists that the caller already loaded.
func cachedFileFetcher(filesByHash map[string]qbt.TorrentFiles) FileFetcher {
	return func(_ []string) (map[string]qbt.TorrentFiles, error) {
		return filesByHash, nil
	}
}

// VerifiedMembers returns the members of a shared-ContentPath group that hold the
// same data as the trigger and are therefore safe to delete with it.
//
// A shared ContentPath only proves the torrents write into the same directory: unrelated
// packs whose payload folder carries the same name (a bare "Season 2" root) all report the
// same path. Members that share no file are dropped, the trigger is still deleted. Partial
// overlap or unreadable files abort the group, because the delete would break data another
// torrent needs.
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
			// Same directory, different content: not a copy of the trigger.
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

// ShouldPreserveCrossSeedFiles reports whether deleting the torrent with files would
// break a verified partner in its Content Path bucket. An unreadable file list counts as
// "preserve": the caller keeps the files instead of guessing.
func ShouldPreserveCrossSeedFiles(torrent qbt.Torrent, idx Index, filesByHash map[string]qbt.TorrentFiles) bool {
	verifiedHashes, ok := VerifiedMembers(
		torrent,
		idx.Group(torrent),
		nil,
		cachedFileFetcher(filesByHash),
	)
	return !ok || len(verifiedHashes) > 1
}

// BatchFileFetcher returns the file lists for the given hashes, honoring ctx.
type BatchFileFetcher func(ctx context.Context, hashes []string) (map[string]qbt.TorrentFiles, error)

// VerifyFileOverlap reports whether torrent1 and torrent2 share at least minOverlapPercent
// of the smaller torrent's files. A zero or negative percent falls back to
// MinFileOverlapPercent. It returns an error when the overlap cannot be computed, so the
// caller can fail closed.
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

// KeyContentPath is the group key that makes a group's Content Path ambiguous when it
// equals the torrent's SavePath: the bucket may then hold unrelated torrents.
const KeyContentPath = "contentPath"

// AmbiguityPolicy values answer an ambiguous Content Path. They match the values
// models.GroupDefinition.AmbiguousPolicy accepts.
const (
	AmbiguityVerifyOverlap = "verify_overlap"
	AmbiguitySkip          = "skip"
)

// GroupIndex is the key-based group index an ambiguity check reads.
type GroupIndex interface {
	IsAmbiguousForHash(hash string) bool
	MembersForHash(hash string) []string
}

// ShouldExpandAmbiguousGroup reports whether a group may expand for triggerHash when the
// trigger's Content Path is ambiguous (ContentPath == SavePath).
//
// def supplies the policy. A nil def, a definition without KeyContentPath, a
// non-ambiguous trigger, or AmbiguitySkip all decide without reading files. Otherwise
// every member must share at least def.MinFileOverlapPercent of its files with the trigger,
// or the whole group is skipped.
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
