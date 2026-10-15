// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package contentgroup

import (
	"context"
	"testing"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/assert"

	"github.com/autobrr/qui/internal/models"
)

type stubGroupIndex struct {
	ambiguous map[string]bool
	members   map[string][]string
}

func (s stubGroupIndex) IsAmbiguousForHash(hash string) bool { return s.ambiguous[hash] }

func (s stubGroupIndex) MembersForHash(hash string) []string { return s.members[hash] }

func TestShouldExpandAmbiguousGroup(t *testing.T) {
	trigger := qbt.Torrent{Hash: "trigger", ContentPath: "/downloads/movie", SavePath: "/downloads/movie"}
	partner := qbt.Torrent{Hash: "partner", ContentPath: "/downloads/movie", SavePath: "/downloads/movie"}
	torrentByHash := map[string]qbt.Torrent{"trigger": trigger, "partner": partner}
	matchingFiles := map[string]qbt.TorrentFiles{
		"trigger": {{Name: "movie.mkv", Size: 1_000_000_000}},
		"partner": {{Name: "movie.mkv", Size: 1_000_000_000}},
	}
	unrelatedFiles := map[string]qbt.TorrentFiles{
		"trigger": {{Name: "movie.mkv", Size: 1_000_000_000}},
		"partner": {{Name: "other.mkv", Size: 1_000_000_000}},
	}
	fetch := func(_ context.Context, _ []string) (map[string]qbt.TorrentFiles, error) {
		return matchingFiles, nil
	}
	ambiguous := stubGroupIndex{ambiguous: map[string]bool{"trigger": true}, members: map[string][]string{"trigger": {"trigger", "partner"}}}
	notAmbiguous := stubGroupIndex{ambiguous: map[string]bool{}, members: map[string][]string{"trigger": {"trigger", "partner"}}}

	contentPathDef := func() *models.GroupDefinition {
		return &models.GroupDefinition{Keys: []string{KeyContentPath}, AmbiguousPolicy: AmbiguityVerifyOverlap, MinFileOverlapPercent: 90}
	}

	tests := []struct {
		scenario string
		fetch    BatchFileFetcher
		def      *models.GroupDefinition
		idx      GroupIndex
		want     bool
	}{
		{
			scenario: "unambiguous Content Path expands without reading files",
			def:      contentPathDef(),
			idx:      notAmbiguous,
			want:     true,
		},
		{
			scenario: "skip policy never expands",
			def:      &models.GroupDefinition{Keys: []string{KeyContentPath}, AmbiguousPolicy: AmbiguitySkip},
			idx:      ambiguous,
			want:     false,
		},
		{
			scenario: "verify_overlap expands when every member shares files",
			fetch:    fetch,
			def:      contentPathDef(),
			idx:      ambiguous,
			want:     true,
		},
		{
			scenario: "verify_overlap refuses when a member shares no files",
			fetch: func(_ context.Context, _ []string) (map[string]qbt.TorrentFiles, error) {
				return unrelatedFiles, nil
			},
			def:  contentPathDef(),
			idx:  ambiguous,
			want: false,
		},
		{
			scenario: "definition without the Content Path key expands",
			def:      &models.GroupDefinition{Keys: []string{"savePath"}},
			idx:      ambiguous,
			want:     true,
		},
		{
			scenario: "no file fetcher refuses",
			def:      contentPathDef(),
			idx:      ambiguous,
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			got := ShouldExpandAmbiguousGroup(context.Background(), tc.fetch, tc.def, tc.idx, "trigger", torrentByHash)
			assert.Equal(t, tc.want, got)
		})
	}
}
