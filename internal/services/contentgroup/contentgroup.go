// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package contentgroup

import (
	"slices"
	"strings"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/pkg/stringutils"
)

var pathComparisonNormalizer = stringutils.NewNormalizer(5*time.Minute, normalizePathInner)

func NormalizePath(p string) string {
	return pathComparisonNormalizer.Normalize(p)
}

func normalizePathInner(p string) string {
	if p == "" {
		return ""
	}

	p = strings.ToLower(p)

	p = strings.ReplaceAll(p, "\\", "/")

	p = strings.TrimSuffix(p, "/")
	return p
}

type Index map[string]pathBucket

type pathBucket struct {
	torrents []qbt.Torrent
	nested   string
}

func BuildIndex(torrents []qbt.Torrent) Index {
	byPath := make(map[string][]qbt.Torrent, len(torrents)/2)
	for i := range torrents {
		p := NormalizePath(torrents[i].ContentPath)
		if p == "" {
			continue
		}
		byPath[p] = append(byPath[p], torrents[i])
	}

	nested := nestedPaths(byPath)
	idx := make(Index, len(byPath))
	for path, group := range byPath {
		idx[path] = pathBucket{torrents: group, nested: nested[path]}
	}
	return idx
}

func (idx Index) Group(target qbt.Torrent) []qbt.Torrent {
	p := NormalizePath(target.ContentPath)
	if p == "" {
		return nil
	}
	return idx[p].torrents
}

func (idx Index) GroupByPath(path string) []qbt.Torrent {
	return idx[NormalizePath(path)].torrents
}

func (idx Index) SharedHashes() []string {
	var hashes []string
	for _, bucket := range idx {
		if len(bucket.torrents) < 2 {
			continue
		}
		for _, torrent := range bucket.torrents {
			hashes = append(hashes, torrent.Hash)
		}
	}
	slices.Sort(hashes)
	return hashes
}
