// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package contentgroup

import (
	"fmt"
	"strings"

	qbt "github.com/autobrr/go-qbittorrent"
)

// NestedPathReason names the ADR 0017 guard in the exclusion reason a preview shows.
const NestedPathReason = "content path nests with another torrent outside the group"

// NestedPathExclusion returns the reason a content group must be excluded because a
// torrent outside the group has a Content Path that nests with the group's Content Path.
// It returns "" when the group is safe.
//
// A group holds the torrents at one Content Path. Nested means one path is a strict
// ancestor of the other on path-segment boundaries, so /data/Pack does not nest with
// /data/Package. Two nested paths land in different buckets, so the file-overlap check
// never compares them, and a delete with files removes the files of the path.
func (idx Index) NestedPathExclusion(group []qbt.Torrent) string {
	groupPath := normalizedGroupPath(group)
	if groupPath == "" {
		return ""
	}
	conflict := idx[groupPath].nested
	if conflict == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s and %s", NestedPathReason, groupPath, conflict)
}

// nestedPaths maps every Content Path that nests with another torrent's Content Path to
// the conflicting path. It walks each path's ancestors and looks them up, so it finds
// both nesting directions in one pass. Several conflicts on one path resolve to the
// lexicographically smallest, so the reason does not depend on map order.
func nestedPaths(byPath map[string][]qbt.Torrent) map[string]string {
	nested := make(map[string]string)
	for path := range byPath {
		for _, ancestor := range ancestorPaths(path) {
			if len(byPath[ancestor]) == 0 {
				continue
			}
			recordConflict(nested, path, ancestor)
			recordConflict(nested, ancestor, path)
		}
	}
	return nested
}

// recordConflict keeps the lexicographically smallest conflict for path.
func recordConflict(nested map[string]string, path, conflict string) {
	if existing, ok := nested[path]; !ok || conflict < existing {
		nested[path] = conflict
	}
}

// ancestorPaths returns the strict ancestors of a normalized path, cut at path
// separators. A Windows drive (d:/data) and a UNC share (//server/share) cut the same way.
func ancestorPaths(path string) []string {
	ancestors := make([]string, 0, strings.Count(path, "/"))
	for i := 1; i < len(path); i++ {
		if path[i] == '/' {
			ancestors = append(ancestors, path[:i])
		}
	}
	return ancestors
}

// normalizedGroupPath returns the Content Path every member shares, or "" when the group
// has none.
func normalizedGroupPath(group []qbt.Torrent) string {
	for _, torrent := range group {
		if p := NormalizePath(torrent.ContentPath); p != "" {
			return p
		}
	}
	return ""
}
