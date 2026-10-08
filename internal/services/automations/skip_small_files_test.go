// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package automations

import (
	"testing"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"
)

const mib = 1024 * 1024

func TestPlanSkipSmallFiles(t *testing.T) {
	tests := []struct {
		name         string
		files        qbt.TorrentFiles
		maxSizeBytes int64
		wantApply    bool
		wantAllBelow bool
		wantIndices  []int
		wantNames    []string
		wantBytes    int64
	}{
		{
			name: "selects only files under the threshold that are still wanted",
			files: qbt.TorrentFiles{
				{Index: 0, Name: "feature.mkv", Size: 4000 * mib, Priority: 1},
				{Index: 1, Name: "sample.mkv", Size: 40 * mib, Priority: 1},
				{Index: 2, Name: "cover.jpg", Size: 2 * mib, Priority: 6},
				{Index: 3, Name: "already-skipped.nfo", Size: 1 * mib, Priority: 0},
			},
			maxSizeBytes: 500 * mib,
			wantApply:    true,
			wantIndices:  []int{1, 2},
			wantNames:    []string{"sample.mkv", "cover.jpg"},
			wantBytes:    42 * mib,
		},
		{
			name: "a file exactly at the threshold stays wanted",
			files: qbt.TorrentFiles{
				{Index: 0, Name: "exact.mkv", Size: 500 * mib, Priority: 1},
				{Index: 1, Name: "under.mkv", Size: 500*mib - 1, Priority: 1},
			},
			maxSizeBytes: 500 * mib,
			wantApply:    true,
			wantIndices:  []int{1},
			wantNames:    []string{"under.mkv"},
			wantBytes:    500*mib - 1,
		},
		{
			name: "nothing to do when every small file is already skipped",
			files: qbt.TorrentFiles{
				{Index: 0, Name: "feature.mkv", Size: 4000 * mib, Priority: 1},
				{Index: 1, Name: "sample.mkv", Size: 40 * mib, Priority: 0},
			},
			maxSizeBytes: 500 * mib,
			wantApply:    true,
			wantIndices:  nil,
			wantNames:    nil,
			wantBytes:    0,
		},
		{
			name: "every file below the threshold leaves the torrent alone",
			files: qbt.TorrentFiles{
				{Index: 0, Name: "part1.mkv", Size: 300 * mib, Priority: 1},
				{Index: 1, Name: "part2.mkv", Size: 200 * mib, Priority: 1},
			},
			maxSizeBytes: 500 * mib,
			wantApply:    false,
			wantAllBelow: true,
			wantIndices:  nil,
			wantNames:    nil,
			wantBytes:    0,
		},
		{
			name: "a single file below the threshold leaves the torrent alone",
			files: qbt.TorrentFiles{
				{Index: 0, Name: "only.mkv", Size: 10 * mib, Priority: 1},
			},
			maxSizeBytes: 500 * mib,
			wantApply:    false,
			wantAllBelow: true,
		},
		{
			name: "every file below the threshold and already skipped is still a guard case",
			files: qbt.TorrentFiles{
				{Index: 0, Name: "part1.mkv", Size: 300 * mib, Priority: 0},
				{Index: 1, Name: "part2.mkv", Size: 200 * mib, Priority: 0},
			},
			maxSizeBytes: 500 * mib,
			wantApply:    false,
			wantAllBelow: true,
		},
		{
			name:         "an empty file list has nothing to plan",
			files:        qbt.TorrentFiles{},
			maxSizeBytes: 500 * mib,
			wantApply:    false,
			wantAllBelow: false,
		},
		{
			name: "a zero threshold selects nothing",
			files: qbt.TorrentFiles{
				{Index: 0, Name: "feature.mkv", Size: 4000 * mib, Priority: 1},
			},
			maxSizeBytes: 0,
			wantApply:    true,
			wantAllBelow: false,
			wantIndices:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, apply := planSkipSmallFiles(tt.files, tt.maxSizeBytes)
			require.Equal(t, tt.wantApply, apply)
			require.Equal(t, tt.wantAllBelow, plan.allBelow)
			require.Equal(t, tt.wantIndices, plan.indices)
			require.Equal(t, tt.wantNames, plan.names)
			require.Equal(t, tt.wantBytes, plan.bytes)
		})
	}
}
