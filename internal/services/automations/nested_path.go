// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package automations

import (
	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/services/contentgroup"
)

// nestedPathExcludedHashes returns the hashes whose Content Path nests with another
// torrent's Content Path. ADR 0017: a delete that expands to a content group must not
// remove files another torrent needs, and two nested paths never share a group, so the
// file-overlap check cannot catch them.
func nestedPathExcludedHashes(torrents []*qbt.Torrent) map[string]struct{} {
	values := make([]qbt.Torrent, 0, len(torrents))
	for _, torrent := range torrents {
		if torrent != nil {
			values = append(values, *torrent)
		}
	}
	idx := contentgroup.BuildIndex(values)
	var excluded map[string]struct{}
	for i := range values {
		if idx.NestedPathExclusion(idx.Group(values[i])) == "" {
			continue
		}
		if excluded == nil {
			excluded = make(map[string]struct{})
		}
		excluded[values[i].Hash] = struct{}{}
	}
	return excluded
}

// prepareNestedPathExclusion records which torrents a group delete must not expand to.
// It runs wherever an eval context meets the full torrent list.
func prepareNestedPathExclusion(evalCtx *EvalContext, torrents []*qbt.Torrent) {
	if evalCtx == nil {
		return
	}
	evalCtx.nestedPathExcluded = nestedPathExcludedHashes(torrents)
}

// nestedPathExcluded reports whether the torrent's Content Path nests with another
// torrent's, so a group delete would remove files that other torrent needs.
func nestedPathExcluded(evalCtx *EvalContext, hash string) bool {
	if evalCtx == nil {
		return false
	}
	_, excluded := evalCtx.nestedPathExcluded[hash]
	return excluded
}
