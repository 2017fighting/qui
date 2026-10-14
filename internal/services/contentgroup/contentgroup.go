// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package contentgroup decides whether two local torrents hold the same data.
// ADR 0017: a content group needs an equal normalized Content Path and a verified
// file overlap, so a delete that removes a group's files cannot break another torrent.
package contentgroup

import (
	"slices"
	"strings"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/pkg/stringutils"
)

var pathComparisonNormalizer = stringutils.NewNormalizer(5*time.Minute, normalizePathInner)

// NormalizePath standardizes a file path for comparison.
// Keep this consistent with cross-seed's path normalization.
func NormalizePath(p string) string {
	return pathComparisonNormalizer.Normalize(p)
}

func normalizePathInner(p string) string {
	if p == "" {
		return ""
	}
	// Lowercase for case-insensitive comparison
	p = strings.ToLower(p)
	// Normalize path separators (Windows backslashes to forward slashes)
	p = strings.ReplaceAll(p, "\\", "/")
	// Remove trailing slash
	p = strings.TrimSuffix(p, "/")
	return p
}

// Index maps normalized Content Paths to the torrents at that path.
// Build once with BuildIndex, then look up in O(1).
type Index map[string][]qbt.Torrent

// BuildIndex buckets torrents by normalized Content Path. A torrent with an empty
// Content Path is left out.
func BuildIndex(torrents []qbt.Torrent) Index {
	idx := make(Index, len(torrents)/2)
	for i := range torrents {
		p := NormalizePath(torrents[i].ContentPath)
		if p == "" {
			continue
		}
		idx[p] = append(idx[p], torrents[i])
	}
	return idx
}

// Group returns every torrent at the target's Content Path, including the target.
// It returns nil when the target's Content Path is empty.
func (idx Index) Group(target qbt.Torrent) []qbt.Torrent {
	p := NormalizePath(target.ContentPath)
	if p == "" {
		return nil
	}
	return idx[p]
}

// SharedHashes returns the sorted hashes of torrents in buckets with more than one
// member, the only buckets where a file-overlap check can find a partner.
func (idx Index) SharedHashes() []string {
	var hashes []string
	for _, group := range idx {
		if len(group) < 2 {
			continue
		}
		for _, torrent := range group {
			hashes = append(hashes, torrent.Hash)
		}
	}
	slices.Sort(hashes)
	return hashes
}
