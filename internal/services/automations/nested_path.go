// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package automations

import (
	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/services/contentgroup"
)

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

func prepareNestedPathExclusion(evalCtx *EvalContext, torrents []*qbt.Torrent) {
	if evalCtx == nil {
		return
	}
	evalCtx.nestedPathExcluded = nestedPathExcludedHashes(torrents)
}

func nestedPathExcluded(evalCtx *EvalContext, hash string) bool {
	if evalCtx == nil {
		return false
	}
	_, excluded := evalCtx.nestedPathExcluded[hash]
	return excluded
}
