// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package deleteplan

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/models"
)

var errNotStubbed = errors.New("deleteplan test: call not stubbed")

// fakeSync stands in for the sync manager and the instance store. A nil field
// fails the call, so a test stubs only what its path reads.
type fakeSync struct {
	getAllTorrents       func(ctx context.Context, instanceID int) ([]qbt.Torrent, error)
	getTorrentFilesBatch func(ctx context.Context, instanceID int, hashes []string) (map[string]qbt.TorrentFiles, error)
	getClient            func(ctx context.Context, instanceID int) (healthChecker, error)
	getInstance          func(ctx context.Context, id int) (*models.Instance, error)
}

// stubSync installs a fakeSync on svc the first time it is called and returns it.
func stubSync(svc *Service) *fakeSync {
	if f, ok := svc.sync.(*fakeSync); ok {
		return f
	}
	f := &fakeSync{}
	svc.sync, svc.clients, svc.instances = f, f, f
	return f
}

func (f *fakeSync) GetAllTorrents(ctx context.Context, instanceID int) ([]qbt.Torrent, error) {
	if f.getAllTorrents == nil {
		return nil, errNotStubbed
	}
	return f.getAllTorrents(ctx, instanceID)
}

func (f *fakeSync) GetTorrentFilesBatch(ctx context.Context, instanceID int, hashes []string) (map[string]qbt.TorrentFiles, error) {
	if f.getTorrentFilesBatch == nil {
		return nil, errNotStubbed
	}
	return f.getTorrentFilesBatch(ctx, instanceID, hashes)
}

func (f *fakeSync) Client(ctx context.Context, instanceID int) (healthChecker, error) {
	if f.getClient == nil {
		return nil, errNotStubbed
	}
	return f.getClient(ctx, instanceID)
}

func (f *fakeSync) Get(ctx context.Context, id int) (*models.Instance, error) {
	if f.getInstance == nil {
		return nil, errNotStubbed
	}
	return f.getInstance(ctx, id)
}

// fakeClient is a healthChecker whose freshness a test can flip mid-run.
type fakeClient struct {
	healthy  atomic.Bool
	lastSync atomic.Int64
}

func newFakeClient(lastSync time.Time) *fakeClient {
	c := &fakeClient{}
	c.healthy.Store(true)
	c.lastSync.Store(lastSync.UnixNano())
	return c
}

func (c *fakeClient) IsHealthy() bool { return c.healthy.Load() }

func (c *fakeClient) GetLastSyncUpdate() time.Time { return time.Unix(0, c.lastSync.Load()) }

func (c *fakeClient) setLastSync(t time.Time) { c.lastSync.Store(t.UnixNano()) }
