// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package automations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/fsops"
	localbackend "github.com/autobrr/qui/internal/fsops/local"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/qbittorrent"
	"github.com/autobrr/qui/internal/services/contentgroup"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

// A delete action must not expand to a content group whose Content Path nests with
// another torrent's Content Path: the expansion would remove files that torrent needs.
func TestPreviewDeleteIncludeCrossSeeds_SkipsNestedContentPath(t *testing.T) {
	tests := []struct {
		scenario     string
		contentPaths []string
		wantTotal    int
	}{
		{
			scenario:     "nested pack and single file are both skipped",
			contentPaths: []string{"/data/Pack", "/data/Pack/extra.mkv"},
			wantTotal:    0,
		},
		{
			scenario:     "sibling paths sharing a string prefix still expand",
			contentPaths: []string{"/data/Pack", "/data/Package"},
			wantTotal:    2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			s := &Service{syncManager: qbittorrent.NewSyncManager(nil, nil)}
			torrents := make([]qbt.Torrent, 0, len(tc.contentPaths))
			for i, contentPath := range tc.contentPaths {
				torrents = append(torrents, qbt.Torrent{
					Hash:        string(rune('a' + i)),
					Tags:        "delete-me",
					SavePath:    "/data",
					ContentPath: contentPath,
				})
			}
			rule := &models.Automation{
				TrackerPattern: "*",
				Conditions: &models.ActionConditions{
					Delete: &models.DeleteAction{
						Enabled: true,
						Mode:    DeleteModeWithFilesIncludeCrossSeeds,
						Condition: &models.RuleCondition{
							Field:    models.FieldTags,
							Operator: models.OperatorContains,
							Value:    "delete-me",
						},
					},
				},
			}

			result, err := s.previewDeleteIncludeCrossSeeds(
				rule,
				torrents,
				&EvalContext{},
				nil,
				10,
				0,
				true,
				nil,
				contentgroup.BuildIndex(torrents),
				func(_ []string) (map[string]qbt.TorrentFiles, error) { return nil, nil },
			)

			require.NoError(t, err)
			assert.Equal(t, tc.wantTotal, result.TotalMatches)
		})
	}
}

// newNestedPathDryRunService boots a Service against a stub qBittorrent whose maindata
// holds the given torrents, so a dry run exercises the delete expansion.
func newNestedPathDryRunService(t *testing.T, torrentsJSON string) (*Service, int) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/webapiVersion":
			_, _ = w.Write([]byte("2.10.0"))
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"rid":1,"full_update":true,"server_state":{"free_space_on_disk":1000},"torrents":` + torrentsJSON + `}`))
		case "/api/v2/torrents/files":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	db := testdb.NewMigratedSQLite(t, t.Name())
	instanceStore, err := models.NewInstanceStore(db, make([]byte, 32))
	require.NoError(t, err)
	instance, err := instanceStore.Create(t.Context(), "nested", server.URL, "", "", nil, nil, false, new(false))
	require.NoError(t, err)

	clientPool, err := qbittorrent.NewClientPool(instanceStore, models.NewInstanceErrorStore(db), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientPool.Close() })

	syncManager := qbittorrent.NewSyncManager(clientPool, nil)
	svc := NewService(Config{}, instanceStore, nil, models.NewAutomationActivityStore(db), nil, syncManager, nil, nil, nil, fsops.NewPool(instanceStore, localbackend.NewBackend()))
	return svc, instance.ID
}

const (
	nestedPackHash   = "1111111111111111111111111111111111111111"
	nestedSingleHash = "2222222222222222222222222222222222222222"
)

func nestedPathTorrentsJSON(packContentPath, singleContentPath string) string {
	return `{
		"` + nestedPackHash + `":{"name":"pack","tags":"delete-me","ratio":2,"progress":1,"size":100,"state":"stalledUP","save_path":"/data","content_path":"` + packContentPath + `"},
		"` + nestedSingleHash + `":{"name":"extra","tags":"delete-me","ratio":2,"progress":1,"size":10,"state":"stalledUP","save_path":"/data","content_path":"` + singleContentPath + `"}
	}`
}

func TestApplyRuleDryRun_IncludeCrossSeedsSkipsNestedContentPath(t *testing.T) {
	tests := []struct {
		scenario      string
		torrentsJSON  string
		wantDeletions int
	}{
		{
			scenario:      "nested paths stop the delete expansion",
			torrentsJSON:  nestedPathTorrentsJSON("/data/Pack", "/data/Pack/extra.mkv"),
			wantDeletions: 0,
		},
		{
			scenario:      "sibling paths keep the delete expansion",
			torrentsJSON:  nestedPathTorrentsJSON("/data/Pack", "/data/Package"),
			wantDeletions: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			svc, instanceID := newNestedPathDryRunService(t, tc.torrentsJSON)
			rule := parityDeleteRule(&models.RuleCondition{
				Field:    models.FieldTags,
				Operator: models.OperatorContains,
				Value:    "delete-me",
			})

			activities, err := svc.ApplyRuleDryRun(t.Context(), instanceID, rule)

			require.NoError(t, err)
			assert.Equal(t, tc.wantDeletions, deleteActivityCount(t, activities))
		})
	}
}

// deleteActivityCount sums the torrents the dry run would delete, across the activity
// rows the run records per action type.
func deleteActivityCount(t *testing.T, activities []*models.AutomationActivity) int {
	t.Helper()
	total := 0
	for _, activity := range activities {
		if activity.Action != models.ActivityActionDeletedCondition {
			continue
		}
		var details struct {
			Count int `json:"count"`
		}
		require.NoError(t, json.Unmarshal(activity.Details, &details))
		total += details.Count
	}
	return total
}

// The free-space projection must match the delete: a nested group keeps its files, so its
// bytes do not count as reclaimed.
func TestDeleteFreesSpace_NestedContentPath(t *testing.T) {
	nested := []qbt.Torrent{
		{Hash: "pack", Size: 100, ContentPath: "/data/Pack"},
		{Hash: "single", Size: 10, ContentPath: "/data/Pack/extra.mkv"},
	}
	siblings := []qbt.Torrent{
		{Hash: "pack", Size: 100, ContentPath: "/data/Pack"},
		{Hash: "sibling", Size: 10, ContentPath: "/data/Package"},
	}

	assert.False(t, deleteFreesSpace(DeleteModeWithFilesPreserveCrossSeeds, nested[0], contentgroup.BuildIndex(nested), nil))
	assert.True(t, deleteFreesSpace(DeleteModeWithFilesPreserveCrossSeeds, siblings[0], contentgroup.BuildIndex(siblings), nil))
	// A plain remove-with-files does not expand to a group, so the guard does not apply.
	assert.True(t, deleteFreesSpace(DeleteModeWithFiles, nested[0], contentgroup.BuildIndex(nested), nil))
}

func TestUpdateCumulativeFreeSpaceCleared_NestedContentPathFreesNothing(t *testing.T) {
	nested := []qbt.Torrent{
		{Hash: "pack", Size: 100, ContentPath: "/data/Pack"},
		{Hash: "single", Size: 10, ContentPath: "/data/Pack/extra.mkv"},
	}
	evalCtx := &EvalContext{
		FilesToClear:           make(map[crossSeedKey]struct{}),
		CrossSeedHashesToClear: make(map[string]struct{}),
		CrossSeedFilesByHash:   map[string]qbt.TorrentFiles{},
	}

	updateCumulativeFreeSpaceCleared(nested[0], evalCtx, DeleteModeWithFilesIncludeCrossSeeds, contentgroup.BuildIndex(nested))

	assert.Zero(t, evalCtx.SpaceToClear)
}
