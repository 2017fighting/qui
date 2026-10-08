// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package automations

import (
	"slices"

	qbt "github.com/autobrr/go-qbittorrent"
)

// filePriorityDoNotDownload is qBittorrent's download priority 0: the file stays in
// the torrent's file list and is never fetched.
const filePriorityDoNotDownload = 0

// maxSkippedFileNames bounds the file names one activity-run row carries, so a
// torrent with thousands of small files does not bloat the in-memory run store.
const maxSkippedFileNames = 10

// skipSmallFilesPlan is what the size threshold changes in one torrent's file list.
type skipSmallFilesPlan struct {
	// indices and names are the files to set to "Do not download", in file order.
	indices []int
	names   []string
	// bytes is the total size of those files.
	bytes int64
	// allBelow marks a file list where every file is under the threshold. Such a
	// torrent is left alone: skipping all of them would leave it with no wanted
	// bytes, so it would never download anything again.
	allBelow bool
}

// planSkipSmallFiles picks the files the Skip small files action sets to priority 0:
// every file under maxSizeBytes that is not already set to "Do not download".
// hasFileList is false when there is nothing to plan against, which is a torrent
// whose metadata has not downloaded (no file list) or one whose every file is under
// the threshold; that second case is the plan's allBelow.
func planSkipSmallFiles(files qbt.TorrentFiles, maxSizeBytes int64) (plan skipSmallFilesPlan, hasFileList bool) {
	if len(files) == 0 {
		return skipSmallFilesPlan{}, false
	}

	below := 0
	for _, file := range files {
		if file.Size >= maxSizeBytes {
			continue
		}
		below++
		if file.Priority == 0 {
			// Already a skipped file: setting it again would only churn the client.
			continue
		}
		plan.indices = append(plan.indices, file.Index)
		plan.names = append(plan.names, file.Name)
		plan.bytes += file.Size
	}

	if below == len(files) {
		return skipSmallFilesPlan{allBelow: true}, false
	}
	return plan, true
}

// skipSmallFilesPlansFor reports, for every torrent that matched a Skip small files
// action, the files to set to "Do not download". A torrent marked for deletion keeps
// its file priorities; the delete removes it anyway. A torrent whose every file is
// under the threshold lands in the plan as allBelow and is left alone there.
func skipSmallFilesPlansFor(states map[string]*torrentDesiredState, evalCtx *EvalContext) map[string]skipSmallFilesPlan {
	plans := make(map[string]skipSmallFilesPlan)
	if evalCtx == nil {
		return plans
	}
	for hash, state := range states {
		if state == nil || state.shouldDelete || state.skipSmallFiles == nil {
			continue
		}
		plan, hasFileList := planSkipSmallFiles(evalCtx.TorrentFilesByHash[hash], state.skipSmallFiles.MaxSizeBytes)
		if (hasFileList && len(plan.indices) > 0) || plan.allBelow {
			plans[hash] = plan
		}
	}
	return plans
}

// buildSkipSmallFilesRunItems reports the per-torrent detail of a Skip small files
// pass for the given torrents: the files a torrent would lose from its download set,
// or the reason it was left alone. The caller decides which torrents the detail
// covers, so a live pass can leave out the ones it left alone.
func (s *Service) buildSkipSmallFilesRunItems(hashes []string, plans map[string]skipSmallFilesPlan, torrentByHash map[string]qbt.Torrent) []ActivityRunTorrent {
	items := make([]ActivityRunTorrent, 0, len(hashes))
	for _, hash := range hashes {
		plan, ok := plans[hash]
		if !ok {
			continue
		}
		item := buildRunItemFromHash(hash, torrentByHash, s.syncManager)
		if plan.allBelow {
			item.SkippedAllBelow = true
			items = append(items, item)
			continue
		}
		item.SkippedFiles = slices.Clone(plan.names[:min(len(plan.names), maxSkippedFileNames)])
		item.SkippedFileCount = len(plan.names)
		item.SkippedBytes = plan.bytes
		items = append(items, item)
	}
	sortActivityRunItems(items)
	return items
}
