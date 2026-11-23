// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package contentgroup

import (
	"fmt"
	"strings"

	qbt "github.com/autobrr/go-qbittorrent"
)

const NestedPathReason = "content path nests with another torrent outside the group"

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

func recordConflict(nested map[string]string, path, conflict string) {
	if existing, ok := nested[path]; !ok || conflict < existing {
		nested[path] = conflict
	}
}

func ancestorPaths(path string) []string {
	ancestors := make([]string, 0, strings.Count(path, "/"))
	for i := 1; i < len(path); i++ {
		if path[i] == '/' {
			ancestors = append(ancestors, path[:i])
		}
	}
	return ancestors
}

func normalizedGroupPath(group []qbt.Torrent) string {
	for _, torrent := range group {
		if p := NormalizePath(torrent.ContentPath); p != "" {
			return p
		}
	}
	return ""
}
