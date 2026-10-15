// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package contentgroup

import (
	"strings"
	"testing"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/assert"
)

// The nested-path guard (ADR 0017): a content group is excluded when a torrent outside
// the group has a Content Path that nests with a group member's Content Path. Two nested
// paths land in different buckets, so the file-overlap check never compares them, and a
// delete with files removes the files of the path.
func TestNestedPathExclusion(t *testing.T) {
	tests := []struct {
		scenario string
		group    []qbt.Torrent
		all      []qbt.Torrent
		want     string
	}{
		{
			scenario: "group path is a strict ancestor of another torrent",
			group:    []qbt.Torrent{{Hash: "pack", ContentPath: "/data/Pack"}},
			all: []qbt.Torrent{
				{Hash: "pack", ContentPath: "/data/Pack"},
				{Hash: "single", ContentPath: "/data/Pack/extra.mkv"},
			},
			want: NestedPathReason + ": /data/pack and /data/pack/extra.mkv",
		},
		{
			scenario: "another torrent is a strict ancestor of the group path",
			group:    []qbt.Torrent{{Hash: "single", ContentPath: "/data/Pack/extra.mkv"}},
			all: []qbt.Torrent{
				{Hash: "pack", ContentPath: "/data/Pack"},
				{Hash: "single", ContentPath: "/data/Pack/extra.mkv"},
			},
			want: NestedPathReason + ": /data/pack/extra.mkv and /data/pack",
		},
		{
			scenario: "sibling names sharing a string prefix do not nest",
			group:    []qbt.Torrent{{Hash: "pack", ContentPath: "/data/Pack"}},
			all: []qbt.Torrent{
				{Hash: "pack", ContentPath: "/data/Pack"},
				{Hash: "sibling", ContentPath: "/data/Package"},
			},
			want: "",
		},
		{
			scenario: "the group's own bucket is not nesting",
			group: []qbt.Torrent{
				{Hash: "a", ContentPath: "/data/Pack"},
				{Hash: "b", ContentPath: "/data/Pack"},
			},
			all: []qbt.Torrent{
				{Hash: "a", ContentPath: "/data/Pack"},
				{Hash: "b", ContentPath: "/data/Pack"},
			},
			want: "",
		},
		{
			scenario: "windows drive path nests",
			group:    []qbt.Torrent{{Hash: "pack", ContentPath: `D:\Data\Pack`}},
			all: []qbt.Torrent{
				{Hash: "pack", ContentPath: `D:\Data\Pack`},
				{Hash: "single", ContentPath: `D:\Data\Pack\extra.mkv`},
			},
			want: NestedPathReason + ": d:/data/pack and d:/data/pack/extra.mkv",
		},
		{
			scenario: "windows drive sibling is safe",
			group:    []qbt.Torrent{{Hash: "pack", ContentPath: `D:\Data\Pack`}},
			all: []qbt.Torrent{
				{Hash: "pack", ContentPath: `D:\Data\Pack`},
				{Hash: "sibling", ContentPath: `D:\Data\Package`},
			},
			want: "",
		},
		{
			scenario: "UNC path nests",
			group:    []qbt.Torrent{{Hash: "pack", ContentPath: `\\server\share\Pack`}},
			all: []qbt.Torrent{
				{Hash: "pack", ContentPath: `\\server\share\Pack`},
				{Hash: "single", ContentPath: `\\server\share\Pack\extra.mkv`},
			},
			want: NestedPathReason + ": //server/share/pack and //server/share/pack/extra.mkv",
		},
		{
			scenario: "UNC sibling is safe",
			group:    []qbt.Torrent{{Hash: "pack", ContentPath: `\\server\share\Pack`}},
			all: []qbt.Torrent{
				{Hash: "pack", ContentPath: `\\server\share\Pack`},
				{Hash: "sibling", ContentPath: `\\server\share\Package`},
			},
			want: "",
		},
		{
			scenario: "empty content path is safe",
			group:    []qbt.Torrent{{Hash: "unknown", ContentPath: ""}},
			all: []qbt.Torrent{
				{Hash: "unknown", ContentPath: ""},
				{Hash: "pack", ContentPath: "/data/Pack"},
			},
			want: "",
		},
		{
			scenario: "a lone group is safe",
			group:    []qbt.Torrent{{Hash: "pack", ContentPath: "/data/Pack"}},
			all:      []qbt.Torrent{{Hash: "pack", ContentPath: "/data/Pack"}},
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			got := BuildIndex(tc.all).NestedPathExclusion(tc.group)
			assert.Equal(t, tc.want, got)
			if tc.want != "" {
				assert.Contains(t, got, NestedPathReason)
			}
		})
	}
}

// The reason names the two paths so a preview can show it, and the conflict is chosen
// deterministically when several torrents nest with the group.
func TestNestedPathExclusion_PicksDeterministicConflict(t *testing.T) {
	all := []qbt.Torrent{
		{Hash: "pack", ContentPath: "/data/Pack"},
		{Hash: "z", ContentPath: "/data/Pack/z.mkv"},
		{Hash: "a", ContentPath: "/data/Pack/a.mkv"},
	}

	got := BuildIndex(all).NestedPathExclusion([]qbt.Torrent{all[0]})

	assert.Equal(t, NestedPathReason+": /data/pack and /data/pack/a.mkv", got)
	assert.True(t, strings.HasPrefix(got, NestedPathReason))
}
