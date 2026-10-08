// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package automations

import (
	"context"
	"fmt"
	"slices"

	qbt "github.com/autobrr/go-qbittorrent"
)

// loadTorrentFiles fetches the file list of every given torrent in one batch.
// Torrents whose metadata has not downloaded get no entry.
func (s *Service) loadTorrentFiles(ctx context.Context, instanceID int, torrents []qbt.Torrent) (map[string]qbt.TorrentFiles, error) {
	hashes := make([]string, 0, len(torrents))
	for _, t := range torrents {
		hashes = append(hashes, t.Hash)
	}
	filesByHash, err := s.filesReader.GetTorrentFilesBatch(ctx, instanceID, hashes)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch torrent files: %w", err)
	}
	return filesByHash, nil
}

func buildSkippedFilesResult(filesByHash map[string]qbt.TorrentFiles) map[string]bool {
	result := make(map[string]bool, len(filesByHash))
	for hash, files := range filesByHash {
		if len(files) == 0 {
			continue // No metadata yet
		}
		result[hash] = slices.ContainsFunc(files, func(f qbt.TorrentFile) bool { return f.Priority == 0 })
	}
	return result
}
