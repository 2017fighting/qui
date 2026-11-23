// Copyright (c) 2025-2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package deleteplan

import (
	"cmp"
	"slices"
	"strings"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/qbittorrent"
	"github.com/autobrr/qui/internal/services/contentgroup"
	"github.com/autobrr/qui/internal/services/ranking"
)

const (
	ReasonIncomplete      = "incomplete"
	ReasonNoDeleteTag     = "no_delete_tag"
	ReasonPartialOverlap  = "partial_overlap"
	ReasonUnreadableFiles = "unreadable_files"
	ReasonNestedPath      = "nested_path"

	ReasonMembersChanged = "members_changed"
)

type ProtectionRules struct {
	DeleteTag string

	RemoveDeadGroups bool

	FilesByHash map[string]qbt.TorrentFiles
}

type PlanGroup struct {
	ContentPath   string
	SizeBytes     int64
	Members       []qbt.Torrent
	Dead          bool
	Included      bool
	Excluded      bool
	ExcludeReason string
	DecidingKey   models.ConditionField
}

type Plan struct {
	Groups         []PlanGroup
	ExcludedGroups []PlanGroup
	PlannedBytes   int64
	DeadBytes      int64
	OvershootBytes int64
	TargetReached  bool
}

func BuildPlan(torrents []qbt.Torrent, idx contentgroup.Index, keys []models.KeyRule, targetBytes int64, rules ProtectionRules) Plan {
	groups := make([]PlanGroup, 0, len(torrents)/2)
	excluded := make([]PlanGroup, 0)

	seen := make(map[string]struct{}, len(torrents)/2)
	for i := range torrents {
		path := contentgroup.NormalizePath(torrents[i].ContentPath)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		group := buildGroup(idx.Group(torrents[i]), idx, rules)
		if group.Excluded {
			excluded = append(excluded, group)
			continue
		}
		groups = append(groups, group)
	}

	sortGroupsByKeys(groups, keys)
	for i := range groups {
		groups[i].DecidingKey = decidingKey(groups, i, keys)
	}

	ranked, dead := splitDead(groups)

	var plannedBytes int64
	reached := false
	planGroups := make([]PlanGroup, 0, len(ranked)+len(dead))
	for _, group := range ranked {
		if !reached {
			group.Included = true
			plannedBytes += group.SizeBytes
			reached = plannedBytes >= targetBytes
		}
		planGroups = append(planGroups, group)
	}

	var deadBytes int64
	if rules.RemoveDeadGroups {
		for _, group := range dead {
			group.Included = true
			planGroups = append(planGroups, group)
			deadBytes += group.SizeBytes
		}
	}

	overshoot := max(plannedBytes-targetBytes, 0)

	return Plan{
		Groups:         planGroups,
		ExcludedGroups: excluded,
		PlannedBytes:   plannedBytes,
		DeadBytes:      deadBytes,
		OvershootBytes: overshoot,
		TargetReached:  plannedBytes >= targetBytes,
	}
}

func buildGroup(bucket []qbt.Torrent, idx contentgroup.Index, rules ProtectionRules) PlanGroup {
	representative := largest(bucket)
	group := PlanGroup{
		ContentPath: representative.ContentPath,
		SizeBytes:   representative.Size,
		Members:     sortedLargestFirst(bucket),
		Dead:        !anySeeding(bucket),
	}

	switch {
	case anyIncomplete(bucket):
		return excludeGroup(group, ReasonIncomplete)
	case anyTagged(bucket, rules.DeleteTag):
		return excludeGroup(group, ReasonNoDeleteTag)
	case idx.NestedPathExclusion(bucket) != "":
		return excludeGroup(group, ReasonNestedPath)
	case len(bucket) > 1:
		verified, reason := verifyMembers(bucket, rules.FilesByHash)
		if reason != "" {
			return excludeGroup(group, reason)
		}
		group.Members = sortedLargestFirst(verified)
		group.SizeBytes = largest(verified).Size
		group.Dead = !anySeeding(verified)
	}
	return group
}

func excludeGroup(group PlanGroup, reason string) PlanGroup {
	group.Included = false
	group.Excluded = true
	group.ExcludeReason = reason
	return group
}

func verifyMembers(bucket []qbt.Torrent, filesByHash map[string]qbt.TorrentFiles) ([]qbt.Torrent, string) {
	trigger := largest(bucket)
	triggerFiles, ok := filesByHash[trigger.Hash]
	if !ok || len(triggerFiles) == 0 {
		return nil, ReasonUnreadableFiles
	}
	triggerIndex := contentgroup.BuildFileOverlapIndex(trigger, triggerFiles)

	verified := make([]qbt.Torrent, 0, len(bucket))
	verified = append(verified, trigger)
	for _, candidate := range bucket {
		if candidate.Hash == trigger.Hash {
			continue
		}
		candidateFiles, ok := filesByHash[candidate.Hash]
		if !ok || len(candidateFiles) == 0 {
			return nil, ReasonUnreadableFiles
		}
		switch percent := contentgroup.FileOverlapPercent(triggerIndex, candidate, candidateFiles); {
		case percent == contentgroup.OverlapUnknown:
			return nil, ReasonUnreadableFiles
		case percent >= contentgroup.MinFileOverlapPercent:
			verified = append(verified, candidate)
		case percent == 0:

		default:
			return nil, ReasonPartialOverlap
		}
	}
	return verified, ""
}

func largest(members []qbt.Torrent) qbt.Torrent {
	rep := members[0]
	for _, member := range members[1:] {
		if member.Size > rep.Size || (member.Size == rep.Size && member.Hash < rep.Hash) {
			rep = member
		}
	}
	return rep
}

func sortedLargestFirst(members []qbt.Torrent) []qbt.Torrent {
	sorted := slices.Clone(members)
	slices.SortFunc(sorted, func(a, b qbt.Torrent) int {
		if a.Size != b.Size {
			return cmp.Compare(b.Size, a.Size)
		}
		return strings.Compare(a.Hash, b.Hash)
	})
	return sorted
}

func anyIncomplete(members []qbt.Torrent) bool {
	for _, member := range members {
		if member.Progress < 1 {
			return true
		}
	}
	return false
}

func anyTagged(members []qbt.Torrent, tag string) bool {
	if tag == "" {
		return false
	}
	for _, member := range members {
		for memberTag := range strings.SplitSeq(member.Tags, ",") {
			if strings.TrimSpace(memberTag) == tag {
				return true
			}
		}
	}
	return false
}

//nolint:exhaustive // Only the seeding states count; every other state is not seeding.
func anySeeding(members []qbt.Torrent) bool {
	for _, member := range members {
		switch member.State {
		case qbt.TorrentStateUploading,
			qbt.TorrentStateStalledUp,
			qbt.TorrentStateForcedUp,
			qbt.TorrentStateQueuedUp,
			qbt.TorrentStateCheckingUp:
			return true
		}
	}
	return false
}

func splitDead(groups []PlanGroup) (ranked, dead []PlanGroup) {
	for _, group := range groups {
		if group.Dead {
			dead = append(dead, group)
			continue
		}
		ranked = append(ranked, group)
	}
	return ranked, dead
}

func sortGroupsByKeys(groups []PlanGroup, keys []models.KeyRule) {
	ranking.SortByKeys(groups, keys, func(group PlanGroup) []ranking.Value {
		return groupKeyValues(group, keys)
	}, func(group PlanGroup) string {
		return group.Members[0].Hash
	})
}

func decidingKey(groups []PlanGroup, index int, keys []models.KeyRule) models.ConditionField {
	first := firstEnabledKey(keys)
	if index == 0 {
		return first
	}
	previous := groupKeyValues(groups[index-1], keys)
	current := groupKeyValues(groups[index], keys)
	for i, key := range keys {
		if !key.Enabled {
			continue
		}
		if previous[i] != current[i] {
			return key.Field
		}
	}
	return first
}

func firstEnabledKey(keys []models.KeyRule) models.ConditionField {
	for _, key := range keys {
		if key.Enabled {
			return key.Field
		}
	}
	return ""
}

func groupKeyValues(group PlanGroup, keys []models.KeyRule) []ranking.Value {
	representative := group.Members[0]
	values := make([]ranking.Value, len(keys))
	for i, key := range keys {
		values[i] = groupKeyValue(group, representative, key.Field)
	}
	return values
}

//nolint:exhaustive // Only GROUP_SIZE and PRIVATE are group-aware; every other key reads the representative.
func groupKeyValue(group PlanGroup, representative qbt.Torrent, field models.ConditionField) ranking.Value {
	switch field {
	case models.FieldGroupSize:
		return ranking.Value{Number: float64(len(group.Members))}
	case models.FieldPrivate:
		if representative.Private {
			return ranking.Value{Number: 1}
		}
		return ranking.Value{Number: 0}
	}
	if field.IsString() {
		return ranking.Value{Text: stringFieldValue(representative, field), IsText: true}
	}
	return ranking.Value{Number: numericFieldValue(representative, field)}
}

//nolint:exhaustive // Only the pure per-torrent fields are ranked here.
func numericFieldValue(torrent qbt.Torrent, field models.ConditionField) float64 {
	switch field {
	case models.FieldSize:
		return float64(torrent.Size)
	case models.FieldTotalSize:
		return float64(torrent.TotalSize)
	case models.FieldDownloaded:
		return float64(torrent.Downloaded)
	case models.FieldUploaded:
		return float64(torrent.Uploaded)
	case models.FieldAmountLeft:
		return float64(torrent.AmountLeft)
	case models.FieldAddedOn:
		return float64(torrent.AddedOn)
	case models.FieldCompletionOn:
		return float64(qbittorrent.NormalizeCompletionTimestamp(torrent.CompletionOn))
	case models.FieldLastActivity:
		return float64(torrent.LastActivity)
	case models.FieldSeedingTime:
		return float64(torrent.SeedingTime)
	case models.FieldTimeActive:
		return float64(torrent.TimeActive)
	case models.FieldRatio:
		return torrent.Ratio
	case models.FieldProgress:
		return torrent.Progress * 100
	case models.FieldAvailability:
		return torrent.Availability
	case models.FieldDlSpeed:
		return float64(torrent.DlSpeed)
	case models.FieldUpSpeed:
		return float64(torrent.UpSpeed)
	case models.FieldNumSeeds:
		return float64(torrent.NumSeeds)
	case models.FieldNumLeechs:
		return float64(torrent.NumLeechs)
	case models.FieldNumComplete:
		return float64(torrent.NumComplete)
	case models.FieldNumIncomplete:
		return float64(torrent.NumIncomplete)
	case models.FieldTrackersCount:
		return float64(torrent.TrackersCount)
	default:
		return 0
	}
}

//nolint:exhaustive // Only the sortable string fields are ranked here.
func stringFieldValue(torrent qbt.Torrent, field models.ConditionField) string {
	switch field {
	case models.FieldName:
		return strings.ToLower(torrent.Name)
	case models.FieldCategory:
		return strings.ToLower(torrent.Category)
	case models.FieldTags:
		return strings.ToLower(torrent.Tags)
	case models.FieldTracker:
		return strings.ToLower(torrent.Tracker)
	case models.FieldState:
		return strings.ToLower(string(torrent.State))
	case models.FieldSavePath:
		return strings.ToLower(torrent.SavePath)
	case models.FieldContentPath:
		return strings.ToLower(torrent.ContentPath)
	case models.FieldComment:
		return strings.ToLower(torrent.Comment)
	default:
		return ""
	}
}
