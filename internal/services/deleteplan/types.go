// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package deleteplan

import (
	"errors"

	"github.com/autobrr/qui/internal/models"
)

var ErrPlanAlreadyActive = errors.New("a delete plan is already active for this instance")

var ErrPlanRunNotFound = errors.New("delete plan run not found")

var ErrCannotCancelDuringDeletion = errors.New("cannot cancel a delete plan while deletion is in progress")

var ErrPlanRunAlreadyFinished = errors.New("delete plan run already finished")

var ErrPlanRunInvalidStatus = errors.New("invalid delete plan run status for this operation")

var ErrPlanRowNotInRun = errors.New("delete plan row is not part of the run")

var ErrInvalidTarget = errors.New("target size must be greater than zero")

func DefaultKeys() []models.KeyRule {
	return []models.KeyRule{
		{Field: models.FieldGroupSize, Enabled: true, Direction: models.SortDirectionASC},
		{Field: models.FieldNumComplete, Enabled: true, Direction: models.SortDirectionDESC},
		{Field: models.FieldSize, Enabled: true, Direction: models.SortDirectionASC},
		{Field: models.FieldPrivate, Enabled: true, Direction: models.SortDirectionASC},
	}
}
