// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package automations

import (
	"slices"

	qbt "github.com/autobrr/go-qbittorrent"
)

const filePriorityDoNotDownload = 0

const maxSkippedFileNames = 10

type skipSmallFilesPlan struct {
	indices []int
	names   []string

	bytes int64

	allBelow bool
}

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

func skipSmallFilesPlansFor(states map[string]*torrentDesiredState, evalCtx *EvalContext, deletions *deletionSelection) map[string]skipSmallFilesPlan {
	plans := make(map[string]skipSmallFilesPlan)
	if evalCtx == nil || deletions == nil {
		return plans
	}
	for hash, state := range states {
		if state == nil || state.skipSmallFiles == nil {
			continue
		}
		if _, deleting := deletions.byHash[hash]; deleting || deletions.isClaimed(hash) {
			continue
		}
		plan, hasFileList := planSkipSmallFiles(evalCtx.TorrentFilesByHash[hash], state.skipSmallFiles.MaxSizeBytes)
		if (hasFileList && len(plan.indices) > 0) || plan.allBelow {
			plans[hash] = plan
		}
	}
	return plans
}

func skipSmallFilesRulesFor(plans map[string]skipSmallFilesPlan, states map[string]*torrentDesiredState) map[string]ruleRef {
	rules := make(map[string]ruleRef, len(plans))
	for hash := range plans {
		if state := states[hash]; state != nil {
			rules[hash] = state.skipSmallFilesRule
		}
	}
	return rules
}

func buildSkipSmallFilesRunItems(hashes []string, plans map[string]skipSmallFilesPlan, torrentByHash map[string]*qbt.Torrent, scope *ruleScope) []ActivityRunTorrent {
	items := make([]ActivityRunTorrent, 0, len(hashes))
	for _, hash := range hashes {
		plan, ok := plans[hash]
		if !ok {
			continue
		}
		item := buildRunItemFromHash(hash, torrentByHash, scope)
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
