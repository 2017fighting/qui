// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package automations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/fsops"
	localbackend "github.com/autobrr/qui/internal/fsops/local"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/qbittorrent"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

const skipHash = "dddddddddddddddddddddddddddddddddddddddd"

type filePrioCall struct {
	hash     string
	ids      string
	priority int
}

// skipSmallFilesRig is a Service in front of a stub qBittorrent that keeps a mutable
// file list, so a priority change is visible to the next pass.
type skipSmallFilesRig struct {
	svc           *Service
	instanceID    int
	webAPIVersion string

	mu      sync.Mutex
	files   map[string]qbt.TorrentFiles
	calls   []filePrioCall
	private bool
}

func newSkipSmallFilesRig(t *testing.T, files qbt.TorrentFiles) *skipSmallFilesRig {
	t.Helper()

	rig := &skipSmallFilesRig{
		webAPIVersion: "2.10.0",
		files:         map[string]qbt.TorrentFiles{skipHash: files},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/webapiVersion":
			_, _ = w.Write([]byte(rig.webAPIVersion))
		case "/api/v2/sync/maindata":
			rig.mu.Lock()
			private := rig.private
			rig.mu.Unlock()
			_, _ = w.Write([]byte(`{"rid":1,"full_update":true,"torrents":{"` + skipHash + `":{"name":"skip-me","category":"jav","private":` +
				strconv.FormatBool(private) + `,"ratio":2,"progress":1,"size":4000,"state":"stalledUP","save_path":"/data","content_path":"/data/skip-me"}}}`))
		case "/api/v2/torrents/files":
			rig.mu.Lock()
			list := rig.files[r.URL.Query().Get("hash")]
			rig.mu.Unlock()
			payload, err := json.Marshal(list)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(payload)
		case "/api/v2/torrents/filePrio":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			rig.recordPriority(r.FormValue("hash"), r.FormValue("id"), r.FormValue("priority"))
			_, _ = w.Write([]byte(""))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	db := testdb.NewMigratedSQLite(t, t.Name())
	instanceStore, err := models.NewInstanceStore(db, make([]byte, 32))
	require.NoError(t, err)
	instance, err := instanceStore.Create(t.Context(), "skip", server.URL, "", "", nil, nil, false, new(false))
	require.NoError(t, err)

	clientPool, err := qbittorrent.NewClientPool(instanceStore, models.NewInstanceErrorStore(db), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientPool.Close() })

	syncManager := qbittorrent.NewSyncManager(clientPool, nil)
	rig.svc = NewService(Config{ApplyTimeout: time.Minute}, instanceStore, nil, models.NewAutomationActivityStore(db), nil, syncManager, nil, nil, nil, fsops.NewPool(instanceStore, localbackend.NewBackend()))
	rig.instanceID = instance.ID
	return rig
}

// recordPriority stores the call and applies it, so the next file list read sees the change.
func (r *skipSmallFilesRig) recordPriority(hash string, ids string, priority string) {
	prio, _ := strconv.Atoi(priority)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, filePrioCall{hash: hash, ids: ids, priority: prio})

	changed := make(map[int]struct{})
	for raw := range strings.SplitSeq(ids, "|") {
		if id, err := strconv.Atoi(raw); err == nil {
			changed[id] = struct{}{}
		}
	}
	for i := range r.files[hash] {
		if _, ok := changed[r.files[hash][i].Index]; ok {
			r.files[hash][i].Priority = prio
		}
	}
}

func (r *skipSmallFilesRig) priorityCalls() []filePrioCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]filePrioCall(nil), r.calls...)
}

func (r *skipSmallFilesRig) setPrivate(private bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.private = private
}

// setFilePriority simulates a user changing one file by hand.
func (r *skipSmallFilesRig) setFilePriority(hash string, index int, priority int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.files[hash] {
		if r.files[hash][i].Index == index {
			r.files[hash][i].Priority = priority
		}
	}
}

func (r *skipSmallFilesRig) activities(t *testing.T) []*models.AutomationActivity {
	t.Helper()
	activities, err := r.svc.activityStore.ListByInstance(t.Context(), r.instanceID, 100)
	require.NoError(t, err)
	return activities
}

func skipDetails(t *testing.T, activity *models.AutomationActivity) struct {
	Count   int   `json:"count"`
	Files   int   `json:"files"`
	Bytes   int64 `json:"bytes"`
	Guarded int   `json:"guarded"`
} {
	t.Helper()
	var details struct {
		Count   int   `json:"count"`
		Files   int   `json:"files"`
		Bytes   int64 `json:"bytes"`
		Guarded int   `json:"guarded"`
	}
	require.NoError(t, json.Unmarshal(activity.Details, &details))
	return details
}

func javSkipRule(maxSizeBytes int64, cond *models.RuleCondition) *models.Automation {
	return &models.Automation{
		ID:             7,
		Name:           "skip small files",
		TrackerPattern: "*",
		Enabled:        true,
		Conditions: &models.ActionConditions{
			SchemaVersion:  "1",
			SkipSmallFiles: &models.SkipSmallFilesAction{Enabled: true, MaxSizeBytes: maxSizeBytes, Condition: cond},
		},
	}
}

// javPublicCondition is the rule the user described: the jav category and a public torrent.
func javPublicCondition() *models.RuleCondition {
	return &models.RuleCondition{
		Operator: models.OperatorAnd,
		Conditions: []*models.RuleCondition{
			{Field: models.FieldCategory, Operator: models.OperatorEqual, Value: "jav"},
			{Field: models.FieldPrivate, Operator: models.OperatorEqual, Value: "false"},
		},
	}
}

func sampleFiles() qbt.TorrentFiles {
	return qbt.TorrentFiles{
		{Index: 0, Name: "feature.mkv", Size: 4000 * mib, Priority: 1},
		{Index: 1, Name: "sample.mkv", Size: 40 * mib, Priority: 1},
		{Index: 2, Name: "cover.jpg", Size: 2 * mib, Priority: 6},
	}
}

func TestSkipSmallFiles_SetsSmallFilesToDoNotDownload(t *testing.T) {
	rig := newSkipSmallFilesRig(t, sampleFiles())

	_, err := rig.svc.applyRulesForInstance(t.Context(), rig.instanceID, true, []*models.Automation{javSkipRule(500*mib, javPublicCondition())}, false)
	require.NoError(t, err)

	calls := rig.priorityCalls()
	require.Len(t, calls, 1, "one call per torrent, never one per file")
	require.Equal(t, skipHash, calls[0].hash)
	require.Equal(t, "1|2", calls[0].ids, "only the files under the threshold, in file order")
	require.Equal(t, filePriorityDoNotDownload, calls[0].priority)

	activities := rig.activities(t)
	require.Len(t, activities, 1)
	require.Equal(t, models.ActivityActionSkippedSmallFiles, activities[0].Action)
	require.Equal(t, models.ActivityOutcomeSuccess, activities[0].Outcome)
	require.Empty(t, activities[0].Hash, "a pass reports one aggregated row")

	details := skipDetails(t, activities[0])
	require.Equal(t, 1, details.Count)
	require.Equal(t, 2, details.Files)
	require.Equal(t, int64(42*mib), details.Bytes)
}

func TestSkipSmallFiles_SecondPassIsIdempotent(t *testing.T) {
	rig := newSkipSmallFilesRig(t, sampleFiles())
	rule := javSkipRule(500*mib, javPublicCondition())

	_, err := rig.svc.applyRulesForInstance(t.Context(), rig.instanceID, true, []*models.Automation{rule}, false)
	require.NoError(t, err)
	_, err = rig.svc.applyRulesForInstance(t.Context(), rig.instanceID, true, []*models.Automation{rule}, false)
	require.NoError(t, err)

	require.Len(t, rig.priorityCalls(), 1, "the second pass finds nothing left to change")
	require.Len(t, rig.activities(t), 1, "a no-op pass records nothing")
}

func TestSkipSmallFiles_LeavesTorrentAloneWhenEveryFileIsSmall(t *testing.T) {
	rig := newSkipSmallFilesRig(t, qbt.TorrentFiles{
		{Index: 0, Name: "part1.mkv", Size: 300 * mib, Priority: 1},
		{Index: 1, Name: "part2.mkv", Size: 200 * mib, Priority: 1},
	})

	_, err := rig.svc.applyRulesForInstance(t.Context(), rig.instanceID, true, []*models.Automation{javSkipRule(500*mib, javPublicCondition())}, false)
	require.NoError(t, err)

	require.Empty(t, rig.priorityCalls(), "skipping every file would leave the torrent with nothing to download")
	require.Empty(t, rig.activities(t))

	activities, err := rig.svc.ApplyRuleDryRun(t.Context(), rig.instanceID, javSkipRule(500*mib, javPublicCondition()))
	require.NoError(t, err)
	require.Len(t, activities, 1)
	details := skipDetails(t, activities[0])
	require.Equal(t, 0, details.Count)
	require.Equal(t, 1, details.Guarded, "the dry-run says why nothing happened")
}

func TestSkipSmallFiles_ConditionKeepsPrivateTorrents(t *testing.T) {
	rig := newSkipSmallFilesRig(t, sampleFiles())
	rig.setPrivate(true)

	_, err := rig.svc.applyRulesForInstance(t.Context(), rig.instanceID, true, []*models.Automation{javSkipRule(500*mib, javPublicCondition())}, false)
	require.NoError(t, err)

	require.Empty(t, rig.priorityCalls())
	require.Empty(t, rig.activities(t))
}

func TestSkipSmallFiles_SetsAReEnabledFileBackToDoNotDownload(t *testing.T) {
	rig := newSkipSmallFilesRig(t, sampleFiles())
	rule := javSkipRule(500*mib, javPublicCondition())

	_, err := rig.svc.applyRulesForInstance(t.Context(), rig.instanceID, true, []*models.Automation{rule}, false)
	require.NoError(t, err)

	// The user re-enables the sample by hand; the next pass takes it back.
	rig.setFilePriority(skipHash, 1, 1)
	_, err = rig.svc.applyRulesForInstance(t.Context(), rig.instanceID, true, []*models.Automation{rule}, false)
	require.NoError(t, err)

	calls := rig.priorityCalls()
	require.Len(t, calls, 2)
	require.Equal(t, "1", calls[1].ids, "the re-enabled file is the only one left to change")
	require.Len(t, rig.activities(t), 2, "the second pass records its own change")
}

func TestSkipSmallFiles_UnsupportedClientRecordsFailure(t *testing.T) {
	rig := newSkipSmallFilesRig(t, sampleFiles())
	rig.webAPIVersion = "2.1.0" // file priority needs 2.2.0

	_, err := rig.svc.applyRulesForInstance(t.Context(), rig.instanceID, true, []*models.Automation{javSkipRule(500*mib, javPublicCondition())}, false)
	require.NoError(t, err)

	require.Empty(t, rig.priorityCalls(), "the client is never asked")
	activities := rig.activities(t)
	require.Len(t, activities, 1)
	require.Equal(t, models.ActivityActionSkipSmallFilesFailed, activities[0].Action)
	require.Equal(t, models.ActivityOutcomeFailed, activities[0].Outcome)
	require.Contains(t, activities[0].Reason, "file priority")
}

func TestSkipSmallFiles_DryRunMatchesTheLivePass(t *testing.T) {
	rig := newSkipSmallFilesRig(t, sampleFiles())

	activities, err := rig.svc.ApplyRuleDryRun(t.Context(), rig.instanceID, javSkipRule(500*mib, javPublicCondition()))
	require.NoError(t, err)
	require.Len(t, activities, 1)

	activity := activities[0]
	require.Equal(t, models.ActivityActionSkippedSmallFiles, activity.Action)
	require.Equal(t, models.ActivityOutcomeDryRun, activity.Outcome)

	details := skipDetails(t, activity)
	require.Equal(t, 1, details.Count)
	require.Equal(t, 2, details.Files)
	require.Equal(t, int64(42*mib), details.Bytes)
	require.Empty(t, rig.priorityCalls(), "a dry run changes nothing")

	items, err := rig.svc.GetActivityRun(rig.instanceID, activity.ID, 50, 0)
	require.NoError(t, err)
	require.Len(t, items.Items, 1)
	require.Equal(t, []string{"sample.mkv", "cover.jpg"}, items.Items[0].SkippedFiles)
	require.Equal(t, 2, items.Items[0].SkippedFileCount)
	require.Equal(t, int64(42*mib), items.Items[0].SkippedBytes)
}
