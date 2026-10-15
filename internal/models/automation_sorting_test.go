// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortingConfigValidate_Keys(t *testing.T) {
	valid := SortingConfig{
		SchemaVersion: "1",
		Type:          SortingTypeKeys,
		Direction:     SortDirectionASC,
		Keys: []KeyRule{
			{Field: FieldGroupSize, Enabled: true, Direction: SortDirectionASC},
			{Field: FieldNumComplete, Enabled: true, Direction: SortDirectionDESC},
			{Field: FieldSize, Enabled: true, Direction: SortDirectionASC},
			{Field: FieldPrivate, Enabled: true, Direction: SortDirectionASC},
		},
	}

	tests := []struct {
		name    string
		edit    func(c *SortingConfig)
		wantErr string
	}{
		{name: "default chain", edit: func(*SortingConfig) {}},
		{
			name:    "no keys",
			edit:    func(c *SortingConfig) { c.Keys = nil },
			wantErr: "keys sort requires at least one key",
		},
		{
			name:    "empty field",
			edit:    func(c *SortingConfig) { c.Keys[0].Field = "" },
			wantErr: "key 0: field required",
		},
		{
			name:    "unsupported field",
			edit:    func(c *SortingConfig) { c.Keys[0].Field = FieldUploadedOverSize },
			wantErr: "key 0: unknown sort field: UPLOADED_OVER_SIZE",
		},
		{
			name:    "invalid key direction",
			edit:    func(c *SortingConfig) { c.Keys[0].Direction = "SIDEWAYS" },
			wantErr: "key 0: invalid direction: SIDEWAYS",
		},
		{
			name:    "missing chain direction stays required",
			edit:    func(c *SortingConfig) { c.Direction = "" },
			wantErr: "direction is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := valid
			config.Keys = append([]KeyRule(nil), valid.Keys...)
			tc.edit(&config)

			if tc.wantErr == "" {
				require.NoError(t, config.Validate())
				return
			}
			require.ErrorContains(t, config.Validate(), tc.wantErr)
		})
	}
}

func TestConditionFieldIsSortableKey(t *testing.T) {
	require.True(t, FieldGroupSize.IsSortableKey())
	require.True(t, FieldPrivate.IsSortableKey())
	require.True(t, FieldSize.IsSortableKey())
	require.True(t, FieldName.IsSortableKey())
	require.False(t, FieldUploadedOverSize.IsSortableKey())
	require.False(t, FieldIsGrouped.IsSortableKey())
}
