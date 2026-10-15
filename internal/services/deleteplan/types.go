// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package deleteplan builds and previews a delete plan for one instance. It
// ranks content groups against a target size, drops the protected ones with a
// reason, and freezes the result as plan rows. Nothing is deleted here.
package deleteplan

import (
	"errors"

	"github.com/autobrr/qui/internal/models"
)

// ErrPlanAlreadyActive is returned when an instance already has an active plan.
var ErrPlanAlreadyActive = errors.New("a delete plan is already active for this instance")

// ErrPlanRunNotFound is returned when a run cannot be found.
var ErrPlanRunNotFound = errors.New("delete plan run not found")

// ErrCannotCancelDuringDeletion is returned when cancel is attempted mid-deletion.
var ErrCannotCancelDuringDeletion = errors.New("cannot cancel a delete plan while deletion is in progress")

// ErrPlanRunAlreadyFinished is returned when a terminal run is modified.
var ErrPlanRunAlreadyFinished = errors.New("delete plan run already finished")

// ErrPlanRunInvalidStatus is returned for a status an operation cannot act on.
var ErrPlanRunInvalidStatus = errors.New("invalid delete plan run status for this operation")

// ErrPlanRowNotInRun is returned when an unticked row ID is not part of the run.
var ErrPlanRowNotInRun = errors.New("delete plan row is not part of the run")

// ErrInvalidTarget is returned when the target size is not positive.
var ErrInvalidTarget = errors.New("target size must be greater than zero")

// DefaultKeys is the ranking chain a plan uses when the request sends none:
// group member count ascending, seeders descending, group bytes ascending,
// non-private before private.
func DefaultKeys() []models.KeyRule {
	return []models.KeyRule{
		{Field: models.FieldGroupSize, Enabled: true, Direction: models.SortDirectionASC},
		{Field: models.FieldNumComplete, Enabled: true, Direction: models.SortDirectionDESC},
		{Field: models.FieldSize, Enabled: true, Direction: models.SortDirectionASC},
		{Field: models.FieldPrivate, Enabled: true, Direction: models.SortDirectionASC},
	}
}
