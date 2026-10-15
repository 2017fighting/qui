// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package ranking orders items by an ordered chain of keys. Automations ranks
// torrents with it and the delete plan ranks content groups, so the two cannot
// disagree about the same key.
package ranking

import (
	"sort"
	"strings"

	"github.com/autobrr/qui/internal/models"
)

// Value is one key's value for an item: a number or a string.
type Value struct {
	Number float64
	Text   string
	IsText bool
}

// Compare orders two value lists by the enabled keys in order and returns -1, 0
// or 1. Values must be built for every key, disabled ones included, so the
// positions line up.
func Compare(a, b []Value, keys []models.KeyRule) int {
	for i, key := range keys {
		if !key.Enabled || i >= len(a) || i >= len(b) {
			continue
		}
		cmp := compareValue(a[i], b[i])
		if cmp == 0 {
			continue
		}
		if key.Direction == models.SortDirectionDESC {
			return -cmp
		}
		return cmp
	}
	return 0
}

func compareValue(a, b Value) int {
	if a.IsText && b.IsText {
		return strings.Compare(a.Text, b.Text)
	}
	switch {
	case a.Number < b.Number:
		return -1
	case a.Number > b.Number:
		return 1
	default:
		return 0
	}
}

// SortByKeys sorts items in place by the key chain, with ties broken by hash
// ascending so the same input always gives the same order.
func SortByKeys[T any](items []T, keys []models.KeyRule, values func(T) []Value, hash func(T) string) {
	sort.SliceStable(items, func(i, j int) bool {
		if cmp := Compare(values(items[i]), values(items[j]), keys); cmp != 0 {
			return cmp < 0
		}
		return hash(items[i]) < hash(items[j])
	})
}
