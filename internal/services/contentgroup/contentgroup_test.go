// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package contentgroup

import (
	"fmt"
	"testing"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/assert"
)

// -----------------------------------------------------------------------------
// NormalizePath tests
// -----------------------------------------------------------------------------

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "lowercase conversion",
			input: "/Data/Movie",
			want:  "/data/movie",
		},
		{
			name:  "backslash to forward slash",
			input: "D:\\Data\\Movie",
			want:  "d:/data/movie",
		},
		{
			name:  "trailing slash removed",
			input: "/data/movie/",
			want:  "/data/movie",
		},
		{
			name:  "all transformations",
			input: "D:\\Data\\Movie\\",
			want:  "d:/data/movie",
		},
		{
			name:  "already normalized",
			input: "/data/movie",
			want:  "/data/movie",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePath(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

// -----------------------------------------------------------------------------
// Index.Group tests
// -----------------------------------------------------------------------------

// Groups by ContentPath equality only; does not expand by SavePath.
// Cross-seeds are exact file matches (same content from different trackers).
func TestIndexGroup(t *testing.T) {
	tests := []struct {
		scenario    string
		target      qbt.Torrent
		allTorrents []qbt.Torrent
		wantCount   int
		wantHashes  []string
	}{
		{
			scenario: "unique ContentPath => group contains only target",
			target: qbt.Torrent{
				Hash:        "abc123",
				Name:        "My.Movie.2024.1080p.BluRay.x264-GRP",
				ContentPath: "/downloads/movies/My.Movie.2024.1080p.BluRay.x264-GRP",
			},
			allTorrents: []qbt.Torrent{
				{Hash: "abc123", Name: "My.Movie.2024.1080p.BluRay.x264-GRP", ContentPath: "/downloads/movies/My.Movie.2024.1080p.BluRay.x264-GRP"},
				{Hash: "def456", Name: "Other.Movie.2024.1080p.BluRay.x264-GRP", ContentPath: "/downloads/movies/Other.Movie.2024.1080p.BluRay.x264-GRP"},
			},
			wantCount:  1,
			wantHashes: []string{"abc123"},
		},
		{
			scenario: "same ContentPath (cross-seed from different tracker) => both in group",
			target: qbt.Torrent{
				Hash:        "abc123",
				Name:        "My.Movie.2024.1080p.BluRay.x264-GRP",
				ContentPath: "/downloads/movies/My.Movie.2024.1080p.BluRay.x264-GRP",
			},
			allTorrents: []qbt.Torrent{
				// Same release cross-seeded to two trackers (identical files, different .torrent)
				{Hash: "abc123", Name: "My.Movie.2024.1080p.BluRay.x264-GRP", ContentPath: "/downloads/movies/My.Movie.2024.1080p.BluRay.x264-GRP"},
				{Hash: "xyz789", Name: "My.Movie.2024.1080p.BluRay.x264-GRP", ContentPath: "/downloads/movies/My.Movie.2024.1080p.BluRay.x264-GRP"},
				{Hash: "def456", Name: "Other.Movie.2024.1080p.BluRay.x264-GRP", ContentPath: "/downloads/movies/Other.Movie.2024.1080p.BluRay.x264-GRP"},
			},
			wantCount:  2,
			wantHashes: []string{"abc123", "xyz789"},
		},
		{
			scenario: "ContentPath match is case-insensitive",
			target: qbt.Torrent{
				Hash:        "abc123",
				Name:        "My.Movie.2024.1080p.BluRay.x264-GRP",
				ContentPath: "/Downloads/Movies/My.Movie.2024.1080p.BluRay.x264-GRP",
			},
			allTorrents: []qbt.Torrent{
				{Hash: "abc123", Name: "My.Movie.2024.1080p.BluRay.x264-GRP", ContentPath: "/Downloads/Movies/My.Movie.2024.1080p.BluRay.x264-GRP"},
				{Hash: "xyz789", Name: "My.Movie.2024.1080p.BluRay.x264-GRP", ContentPath: "/downloads/movies/my.movie.2024.1080p.bluray.x264-grp"},
			},
			wantCount:  2,
			wantHashes: []string{"abc123", "xyz789"},
		},
		{
			scenario: "same SavePath but different ContentPath => NOT grouped",
			target: qbt.Torrent{
				Hash:        "abc123",
				Name:        "My.Movie.2024.1080p.BluRay.x264-GRP",
				SavePath:    "/downloads/movies",
				ContentPath: "/downloads/movies/My.Movie.2024.1080p.BluRay.x264-GRP",
			},
			allTorrents: []qbt.Torrent{
				{Hash: "abc123", Name: "My.Movie.2024.1080p.BluRay.x264-GRP", SavePath: "/downloads/movies", ContentPath: "/downloads/movies/My.Movie.2024.1080p.BluRay.x264-GRP"},
				// Different releases in same SavePath - NOT cross-seeds (different files)
				{Hash: "def456", Name: "Other.Movie.2024.1080p.BluRay.x264-GRP", SavePath: "/downloads/movies", ContentPath: "/downloads/movies/Other.Movie.2024.1080p.BluRay.x264-GRP"},
				{Hash: "ghi789", Name: "Another.Movie.2024.1080p.BluRay.x264-GRP", SavePath: "/downloads/movies", ContentPath: "/downloads/movies/Another.Movie.2024.1080p.BluRay.x264-GRP"},
			},
			wantCount:  1,
			wantHashes: []string{"abc123"}, // Only target; others share SavePath but NOT ContentPath
		},
		{
			scenario: "empty ContentPath => returns nil (no grouping possible)",
			target: qbt.Torrent{
				Hash:        "abc123",
				Name:        "Unknown",
				ContentPath: "",
			},
			allTorrents: []qbt.Torrent{
				{Hash: "abc123", Name: "Unknown", ContentPath: ""},
				{Hash: "def456", Name: "My.Movie.2024.1080p.BluRay.x264-GRP", ContentPath: "/downloads/movies/My.Movie.2024.1080p.BluRay.x264-GRP"},
			},
			wantCount:  0,
			wantHashes: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			got := BuildIndex(tc.allTorrents).Group(tc.target)
			if tc.wantHashes == nil {
				assert.Nil(t, got)
			} else {
				assert.Len(t, got, tc.wantCount)
				gotHashes := make([]string, len(got))
				for i, torrent := range got {
					gotHashes[i] = torrent.Hash
				}
				assert.ElementsMatch(t, tc.wantHashes, gotHashes)
			}
		})
	}
}

// TestIndexGroupByPath covers the path lookup a frozen plan row's re-check uses.
func TestIndexGroupByPath(t *testing.T) {
	idx := BuildIndex([]qbt.Torrent{
		{Hash: "a", ContentPath: "/downloads/Movies/Pack"},
		{Hash: "b", ContentPath: "/downloads/movies/pack"},
		{Hash: "c", ContentPath: "/downloads/Other"},
	})

	group := idx.GroupByPath("/downloads/Movies/Pack")
	assert.Len(t, group, 2, "the lookup normalizes the path, so case does not split a bucket")
	assert.ElementsMatch(t, []string{"a", "b"}, []string{group[0].Hash, group[1].Hash})

	assert.Nil(t, idx.GroupByPath(""), "an empty path has no bucket")
	assert.Nil(t, idx.GroupByPath("/downloads/Unknown"), "an unknown path has no bucket")
}

// -----------------------------------------------------------------------------
// VerifiedMembers tests
// -----------------------------------------------------------------------------

func TestVerifiedMembers(t *testing.T) {
	// Two unrelated packs whose payload folder is a bare "Season 2", so qBittorrent
	// reports the same content path for both while they hold different files.
	trigger := qbt.Torrent{
		Hash:        "trigger",
		Name:        "[Subs] Silver Lantern Chronicle S2 (BD 1080p)",
		ContentPath: "/downloads/anime/Season 2",
		SavePath:    "/downloads/anime",
	}
	stranger := qbt.Torrent{
		Hash:        "stranger",
		Name:        "[Subs] Paper Crane Detective S2 (BD 1080p)",
		ContentPath: "/downloads/anime/Season 2",
		SavePath:    "/downloads/anime",
	}
	crossSeed := qbt.Torrent{
		Hash:        "crossseed",
		Name:        "Silver.Lantern.Chronicle.S02.1080p.BluRay-GRPB",
		ContentPath: "/downloads/anime/Season 2",
		SavePath:    "/downloads/anime",
	}

	// Ten equal episodes, so a swapped file moves the overlap by exactly 10%.
	episodes := func(title string, shared int) qbt.TorrentFiles {
		files := make(qbt.TorrentFiles, 0, 10)
		for i := 1; i <= 10; i++ {
			name := fmt.Sprintf("Season 2/%s - %02d.mkv", title, i)
			if i > shared {
				name = fmt.Sprintf("Season 2/%s - %02d (v2).mkv", title, i)
			}
			files = append(files, qbt.TorrentFile{Name: name, Size: 1_000_000_000})
		}
		return files
	}
	triggerFiles := episodes("Silver Lantern Chronicle", 10)
	strangerFiles := episodes("Paper Crane Detective", 10)
	thresholdFiles := episodes("Silver Lantern Chronicle", 9)
	partialFiles := episodes("Silver Lantern Chronicle", 5)

	tests := []struct {
		scenario string
		group    []qbt.Torrent
		skip     map[string]struct{}
		files    map[string]qbt.TorrentFiles
		filesErr error
		want     []string
		wantOK   bool
	}{
		{
			scenario: "bucket with one member keeps the trigger",
			group:    []qbt.Torrent{trigger},
			files:    map[string]qbt.TorrentFiles{"trigger": triggerFiles},
			want:     []string{"trigger"},
			wantOK:   true,
		},
		{
			scenario: "torrent sharing only the directory is dropped, trigger still deleted",
			group:    []qbt.Torrent{trigger, stranger},
			files:    map[string]qbt.TorrentFiles{"trigger": triggerFiles, "stranger": strangerFiles},
			want:     []string{"trigger"},
			wantOK:   true,
		},
		{
			scenario: "real cross-seed is deleted with the trigger",
			group:    []qbt.Torrent{trigger, crossSeed},
			files:    map[string]qbt.TorrentFiles{"trigger": triggerFiles, "crossseed": triggerFiles},
			want:     []string{"trigger", "crossseed"},
			wantOK:   true,
		},
		{
			scenario: "cross-seed on the overlap threshold is still deleted",
			group:    []qbt.Torrent{trigger, crossSeed},
			files:    map[string]qbt.TorrentFiles{"trigger": triggerFiles, "crossseed": thresholdFiles},
			want:     []string{"trigger", "crossseed"},
			wantOK:   true,
		},
		{
			scenario: "partial overlap skips the whole group",
			group:    []qbt.Torrent{trigger, crossSeed},
			files:    map[string]qbt.TorrentFiles{"trigger": triggerFiles, "crossseed": partialFiles},
			wantOK:   false,
		},
		{
			scenario: "missing file list skips the whole group",
			group:    []qbt.Torrent{trigger, crossSeed},
			files:    map[string]qbt.TorrentFiles{"trigger": triggerFiles},
			wantOK:   false,
		},
		{
			scenario: "fetch error skips the whole group",
			group:    []qbt.Torrent{trigger, crossSeed},
			filesErr: assert.AnError,
			wantOK:   false,
		},
		{
			scenario: "members already queued elsewhere are ignored",
			group:    []qbt.Torrent{trigger, stranger},
			skip:     map[string]struct{}{"stranger": {}},
			files:    map[string]qbt.TorrentFiles{},
			want:     []string{"trigger"},
			wantOK:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			got, ok := VerifiedMembers(trigger, tc.group, tc.skip, func(_ []string) (map[string]qbt.TorrentFiles, error) {
				if tc.filesErr != nil {
					return nil, tc.filesErr
				}
				return tc.files, nil
			})
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("different layouts resolving to the same file are grouped", func(t *testing.T) {
		layoutTrigger := qbt.Torrent{
			Hash:        "layout-trigger",
			ContentPath: "/downloads/Season 2",
			SavePath:    "/downloads",
		}
		layoutCrossSeed := qbt.Torrent{
			Hash:        "layout-crossseed",
			ContentPath: "/downloads/Season 2",
			SavePath:    "/downloads/Season 2",
		}
		filesByHash := map[string]qbt.TorrentFiles{
			layoutTrigger.Hash:   {{Name: "Season 2/01.mkv", Size: 1_000_000_000}},
			layoutCrossSeed.Hash: {{Name: "01.mkv", Size: 1_000_000_000}},
		}

		got, ok := VerifiedMembers(
			layoutTrigger,
			[]qbt.Torrent{layoutTrigger, layoutCrossSeed},
			nil,
			func(_ []string) (map[string]qbt.TorrentFiles, error) { return filesByHash, nil },
		)

		assert.True(t, ok)
		assert.Equal(t, []string{layoutTrigger.Hash, layoutCrossSeed.Hash}, got)
	})
}
