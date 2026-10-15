// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package deleteplan

import "time"

// MaxSyncAge is the oldest sync data a plan trusts. A run stays pending with
// waitingForSync until the instance's last successful sync is newer than this.
// It matches the orphan scan threshold.
const MaxSyncAge = 2 * time.Minute

// DeleteProtectionTag takes a content group out of a plan when any member carries it.
const DeleteProtectionTag = "no_delete"

// Config holds the service configuration.
type Config struct {
	// SyncPollInterval is how often a pending run rechecks sync freshness.
	SyncPollInterval time.Duration

	// StuckRunThreshold is how long a pending/planning run may live before a
	// restart marks it failed.
	StuckRunThreshold time.Duration
}

// DefaultConfig returns the default service configuration.
func DefaultConfig() Config {
	return Config{
		SyncPollInterval:  5 * time.Second,
		StuckRunThreshold: time.Hour,
	}
}
