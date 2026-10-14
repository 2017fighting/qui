// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package ranking

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/models"
)

// groupKeyValues builds the values of the default chain: member count, seeders,
// group byte size, private.
func groupKeyValues(members int, seeders, size float64, private bool) []Value {
	privateValue := 0.0
	if private {
		privateValue = 1
	}
	return []Value{
		{Number: float64(members)},
		{Number: seeders},
		{Number: size},
		{Number: privateValue},
	}
}

func defaultChain() []models.KeyRule {
	return []models.KeyRule{
		{Field: models.FieldGroupSize, Enabled: true, Direction: models.SortDirectionASC},
		{Field: models.FieldNumComplete, Enabled: true, Direction: models.SortDirectionDESC},
		{Field: models.FieldSize, Enabled: true, Direction: models.SortDirectionASC},
		{Field: models.FieldPrivate, Enabled: true, Direction: models.SortDirectionASC},
	}
}

func TestCompare_DefaultChain(t *testing.T) {
	tests := []struct {
		name     string
		keys     []models.KeyRule
		groups   map[string][]Value
		expected []string
	}{
		{
			name: "member count first, then seeders desc, then size asc, then non-private",
			keys: defaultChain(),
			groups: map[string][]Value{
				// One member, fewer seeders, largest bytes, public.
				"c": groupKeyValues(1, 5, 100, false),
				// Two members, most seeders, public.
				"a": groupKeyValues(2, 10, 300, false),
				// One member, few seeders, smallest bytes, public.
				"b": groupKeyValues(1, 5, 50, false),
				// One member, few seeders, smallest bytes, private.
				"d": groupKeyValues(1, 5, 50, true),
			},
			expected: []string{"b", "d", "c", "a"},
		},
		{
			name: "disabling the member count key drops it from the order",
			keys: []models.KeyRule{
				{Field: models.FieldGroupSize, Enabled: false, Direction: models.SortDirectionASC},
				{Field: models.FieldNumComplete, Enabled: true, Direction: models.SortDirectionDESC},
				{Field: models.FieldSize, Enabled: true, Direction: models.SortDirectionASC},
				{Field: models.FieldPrivate, Enabled: true, Direction: models.SortDirectionASC},
			},
			groups: map[string][]Value{
				"x": groupKeyValues(3, 1, 1, false),
				"y": groupKeyValues(1, 1, 1, false),
				"z": groupKeyValues(2, 1, 1, false),
			},
			// Equal on every enabled key, so the hash breaks the tie ascending.
			expected: []string{"x", "y", "z"},
		},
		{
			name: "a disabled key never decides",
			keys: []models.KeyRule{
				{Field: models.FieldSize, Enabled: true, Direction: models.SortDirectionASC},
				{Field: models.FieldNumComplete, Enabled: false, Direction: models.SortDirectionDESC},
			},
			groups: map[string][]Value{
				"a": {{Number: 1}, {Number: 1000}},
				"b": {{Number: 2}, {Number: 0}},
			},
			expected: []string{"a", "b"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hashes := make([]string, 0, len(tc.groups))
			for hash := range tc.groups {
				hashes = append(hashes, hash)
			}

			SortByKeys(hashes, tc.keys, func(hash string) []Value { return tc.groups[hash] }, func(hash string) string { return hash })

			require.Equal(t, tc.expected, hashes)
		})
	}
}

func TestCompare_ReturnsSign(t *testing.T) {
	keys := []models.KeyRule{{Field: models.FieldSize, Enabled: true, Direction: models.SortDirectionASC}}
	require.Equal(t, -1, Compare([]Value{{Number: 1}}, []Value{{Number: 2}}, keys))
	require.Equal(t, 0, Compare([]Value{{Number: 2}}, []Value{{Number: 2}}, keys))
	require.Equal(t, 1, Compare([]Value{{Number: 3}}, []Value{{Number: 2}}, keys))
}

func TestCompare_TextKeys(t *testing.T) {
	keys := []models.KeyRule{{Field: models.FieldName, Enabled: true, Direction: models.SortDirectionASC}}
	require.Equal(t, -1, Compare([]Value{{Text: "alpha", IsText: true}}, []Value{{Text: "beta", IsText: true}}, keys))
	require.Equal(t, 1, Compare([]Value{{Text: "beta", IsText: true}}, []Value{{Text: "alpha", IsText: true}}, keys))
}

func TestSortByKeys_IsDeterministic(t *testing.T) {
	keys := defaultChain()
	groups := map[string][]Value{
		"e": groupKeyValues(1, 5, 50, false),
		"b": groupKeyValues(1, 5, 50, false),
		"d": groupKeyValues(1, 5, 50, false),
		"a": groupKeyValues(1, 5, 50, false),
		"c": groupKeyValues(1, 5, 50, false),
	}

	first := []string{"a", "b", "c", "d", "e"}
	second := []string{"e", "d", "c", "b", "a"}
	for i := 0; i < 2; i++ {
		SortByKeys(first, keys, func(hash string) []Value { return groups[hash] }, func(hash string) string { return hash })
	}
	for i := 0; i < 2; i++ {
		SortByKeys(second, keys, func(hash string) []Value { return groups[hash] }, func(hash string) string { return hash })
	}

	require.Equal(t, []string{"a", "b", "c", "d", "e"}, first)
	require.Equal(t, first, second)
}
