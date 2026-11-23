// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package deleteplan

import "time"

const MaxSyncAge = 2 * time.Minute

const DeleteProtectionTag = "no_delete"

type Config struct {
	SyncPollInterval time.Duration

	StuckRunThreshold time.Duration
}

func DefaultConfig() Config {
	return Config{
		SyncPollInterval:  5 * time.Second,
		StuckRunThreshold: time.Hour,
	}
}
