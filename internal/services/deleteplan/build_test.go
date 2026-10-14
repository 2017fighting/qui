// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package deleteplan

import (
	"testing"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/services/contentgroup"
)

// The plan corpus runs one scenario per row. A row gives the instance's
// torrents, the file lists the overlap check reads, the target, and the
// verdict: the included groups in plan order, the excluded groups with their
// reasons, and the totals. reason states why the verdict is the one it is.
type planCorpusRow struct {
	name string

	torrents         []qbt.Torrent
	files            map[string]qbt.TorrentFiles
	targetBytes      int64
	removeDeadGroups bool

	// verdict
	wantIncluded      []string
	wantExcluded      map[string]string
	wantPlannedBytes  int64
	wantDeadBytes     int64
	wantOvershoot     int64
	wantTargetReached bool

	reason string
}

func torrent(hash, contentPath string, size int64, progress float64, state qbt.TorrentState, tags string) qbt.Torrent {
	return qbt.Torrent{
		Hash:        hash,
		Name:        hash,
		ContentPath: contentPath,
		SavePath:    "/data",
		Size:        size,
		Progress:    progress,
		State:       state,
		Tags:        tags,
	}
}

// fileList names files under /data, the SavePath of every fixture torrent.
func fileList(names ...string) qbt.TorrentFiles {
	files := make(qbt.TorrentFiles, 0, len(names))
	for _, name := range names {
		files = append(files, qbt.TorrentFile{Name: name, Size: 100})
	}
	return files
}

func TestBuildPlan(t *testing.T) {
	rows := []planCorpusRow{
		{
			name: "one torrent is one group",
			torrents: []qbt.Torrent{
				torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, ""),
			},
			targetBytes:       100,
			wantIncluded:      []string{"/data/A"},
			wantPlannedBytes:  100,
			wantTargetReached: true,
			reason:            "a single torrent is its own content group",
		},
		{
			name: "same content path and same files form one group",
			torrents: []qbt.Torrent{
				torrent("a", "/data/Pack", 300, 1, qbt.TorrentStateUploading, ""),
				torrent("b", "/data/Pack", 200, 1, qbt.TorrentStateUploading, ""),
			},
			files: map[string]qbt.TorrentFiles{
				"a": fileList("Pack/one.mkv"),
				"b": fileList("Pack/one.mkv"),
			},
			targetBytes:       300,
			wantIncluded:      []string{"/data/Pack"},
			wantPlannedBytes:  300,
			wantTargetReached: true,
			reason:            "equal Content Paths and a full file overlap are one group; bytes count once as the largest member",
		},
		{
			name: "a member with no file overlap is dropped from the group",
			torrents: []qbt.Torrent{
				torrent("a", "/data/Pack", 300, 1, qbt.TorrentStateUploading, ""),
				torrent("b", "/data/Pack", 200, 1, qbt.TorrentStateUploading, ""),
			},
			files: map[string]qbt.TorrentFiles{
				"a": fileList("Pack/one.mkv"),
				"b": fileList("Pack/other.mkv"),
			},
			targetBytes:       300,
			wantIncluded:      []string{"/data/Pack"},
			wantPlannedBytes:  300,
			wantTargetReached: true,
			reason:            "the same directory with different content is not a copy, so the group is the trigger alone",
		},
		{
			name: "the plan stops at the first group that reaches the target",
			torrents: []qbt.Torrent{
				torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, ""),
				torrent("b", "/data/B", 200, 1, qbt.TorrentStateUploading, ""),
				torrent("c", "/data/C", 300, 1, qbt.TorrentStateUploading, ""),
			},
			targetBytes:       250,
			wantIncluded:      []string{"/data/A", "/data/B"},
			wantPlannedBytes:  300,
			wantOvershoot:     50,
			wantTargetReached: true,
			reason:            "groups are ranked smallest first, so A and B reach 300 and C is never reached",
		},
		{
			name: "the plan reports when the target cannot be reached",
			torrents: []qbt.Torrent{
				torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, ""),
				torrent("b", "/data/B", 200, 1, qbt.TorrentStateUploading, ""),
			},
			targetBytes:       1000,
			wantIncluded:      []string{"/data/A", "/data/B"},
			wantPlannedBytes:  300,
			wantTargetReached: false,
			reason:            "every ranked group is included and the target is still unmet",
		},
		{
			name: "dead groups leave the target arithmetic",
			torrents: []qbt.Torrent{
				torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, ""),
				torrent("b", "/data/B", 500, 1, qbt.TorrentStateStoppedUp, ""),
			},
			targetBytes:       200,
			removeDeadGroups:  true,
			wantIncluded:      []string{"/data/A", "/data/B"},
			wantPlannedBytes:  100,
			wantDeadBytes:     500,
			wantTargetReached: false,
			reason:            "a stopped group is dead, so its bytes do not count toward the target",
		},
		{
			name: "dead groups are omitted when the switch is off",
			torrents: []qbt.Torrent{
				torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, ""),
				torrent("b", "/data/B", 500, 1, qbt.TorrentStateStoppedUp, ""),
			},
			targetBytes:       100,
			wantIncluded:      []string{"/data/A"},
			wantPlannedBytes:  100,
			wantTargetReached: true,
			reason:            "removeDeadGroups is false, so the dead group is not in the plan",
		},
		{
			name: "missingFiles counts as not seeding",
			torrents: []qbt.Torrent{
				torrent("a", "/data/A", 100, 1, qbt.TorrentStateMissingFiles, ""),
			},
			targetBytes:       100,
			removeDeadGroups:  true,
			wantIncluded:      []string{"/data/A"},
			wantDeadBytes:     100,
			wantTargetReached: false,
			reason:            "a torrent with no files on disk cannot promise bytes, so its group is dead",
		},
		{
			name: "a member still downloading protects the group",
			torrents: []qbt.Torrent{
				torrent("a", "/data/A", 100, 0.5, qbt.TorrentStateDownloading, ""),
			},
			targetBytes:  100,
			wantExcluded: map[string]string{"/data/A": ReasonIncomplete},
			reason:       "an incomplete member protects the whole group",
		},
		{
			name: "the delete protection tag protects the group",
			torrents: []qbt.Torrent{
				torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, "cross-seed,no_delete"),
			},
			targetBytes:  100,
			wantExcluded: map[string]string{"/data/A": ReasonNoDeleteTag},
			reason:       "any member carrying no_delete takes the group out of the plan",
		},
		{
			name: "a partial file overlap excludes the group",
			torrents: []qbt.Torrent{
				torrent("a", "/data/Pack", 200, 1, qbt.TorrentStateUploading, ""),
				torrent("b", "/data/Pack", 200, 1, qbt.TorrentStateUploading, ""),
			},
			files: map[string]qbt.TorrentFiles{
				"a": fileList("Pack/one.mkv", "Pack/two.mkv"),
				"b": fileList("Pack/one.mkv", "Pack/three.mkv"),
			},
			targetBytes:  200,
			wantExcluded: map[string]string{"/data/Pack": ReasonPartialOverlap},
			reason:       "a shared path with a partial overlap would break the other torrent",
		},
		{
			name: "an unreadable file list excludes the group",
			torrents: []qbt.Torrent{
				torrent("a", "/data/Pack", 200, 1, qbt.TorrentStateUploading, ""),
				torrent("b", "/data/Pack", 200, 1, qbt.TorrentStateUploading, ""),
			},
			files: map[string]qbt.TorrentFiles{
				"a": fileList("Pack/one.mkv"),
			},
			targetBytes:  200,
			wantExcluded: map[string]string{"/data/Pack": ReasonUnreadableFiles},
			reason:       "a file list that cannot be read drops the group instead of guessing",
		},
		{
			name: "a nested content path excludes both groups",
			torrents: []qbt.Torrent{
				torrent("a", "/data/Pack", 200, 1, qbt.TorrentStateUploading, ""),
				torrent("b", "/data/Pack/extra.mkv", 100, 1, qbt.TorrentStateUploading, ""),
			},
			targetBytes: 200,
			wantExcluded: map[string]string{
				"/data/Pack":           ReasonNestedPath,
				"/data/Pack/extra.mkv": ReasonNestedPath,
			},
			reason: "a single-file torrent inside a pack directory would lose its data",
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			idx := contentgroup.BuildIndex(row.torrents)
			plan := BuildPlan(row.torrents, idx, DefaultKeys(), row.targetBytes, ProtectionRules{
				DeleteTag:        DeleteProtectionTag,
				RemoveDeadGroups: row.removeDeadGroups,
				FilesByHash:      row.files,
			})

			require.Equal(t, row.wantIncluded, contentPaths(plan.Groups), row.reason)
			require.Equal(t, row.wantExcluded, excludedReasons(plan.ExcludedGroups), row.reason)
			require.Equal(t, row.wantPlannedBytes, plan.PlannedBytes, row.reason)
			require.Equal(t, row.wantDeadBytes, plan.DeadBytes, row.reason)
			require.Equal(t, row.wantOvershoot, plan.OvershootBytes, row.reason)
			require.Equal(t, row.wantTargetReached, plan.TargetReached, row.reason)
		})
	}
}

// TestBuildPlan_DefaultDeadGroupsOn pins the default the request falls back to.
func TestBuildPlan_DefaultDeadGroupsOn(t *testing.T) {
	torrents := []qbt.Torrent{
		torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, ""),
		torrent("b", "/data/B", 500, 1, qbt.TorrentStateStoppedUp, ""),
	}
	idx := contentgroup.BuildIndex(torrents)
	plan := BuildPlan(torrents, idx, DefaultKeys(), 200, ProtectionRules{
		DeleteTag:        DeleteProtectionTag,
		RemoveDeadGroups: true,
	})

	require.Equal(t, []string{"/data/A", "/data/B"}, contentPaths(plan.Groups))
	require.Equal(t, int64(500), plan.DeadBytes)
}

// TestBuildPlan_DecidingKey names the key that decided the first group's place.
func TestBuildPlan_DecidingKey(t *testing.T) {
	torrents := []qbt.Torrent{
		torrent("a", "/data/A", 100, 1, qbt.TorrentStateUploading, ""),
		torrent("b", "/data/B", 200, 1, qbt.TorrentStateUploading, ""),
	}
	idx := contentgroup.BuildIndex(torrents)
	plan := BuildPlan(torrents, idx, DefaultKeys(), 100, ProtectionRules{DeleteTag: DeleteProtectionTag})

	require.Len(t, plan.Groups, 1)
	require.Equal(t, models.FieldGroupSize, plan.Groups[0].DecidingKey)
}

func contentPaths(groups []PlanGroup) []string {
	if len(groups) == 0 {
		return nil
	}
	paths := make([]string, 0, len(groups))
	for _, group := range groups {
		paths = append(paths, group.ContentPath)
	}
	return paths
}

func excludedReasons(groups []PlanGroup) map[string]string {
	if len(groups) == 0 {
		return nil
	}
	reasons := make(map[string]string, len(groups))
	for _, group := range groups {
		reasons[group.ContentPath] = group.ExcludeReason
	}
	return reasons
}
