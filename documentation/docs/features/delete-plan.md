---
sidebar_position: 6
title: Delete plan to reclaim disk space
sidebar_label: Delete Plan
description: Reclaim a target size from one qBittorrent instance by removing whole content groups.
---

# Delete Plan

A delete plan reclaims a target size from one instance. You type the size you need back, review the exact content groups qui will remove, and confirm. The plan is one-shot: it is not a saved automation rule.

## How it works

1. On the **Automations** page, open **Delete Plan** and pick one instance.
2. Type the size to reclaim.
3. qui ranks the instance's content groups and builds a preview.
4. Review the groups, the excluded groups and their reasons, and the totals. Untick a group to veto it, and refill to reach the target again.
5. Confirm. qui removes the groups in the background and reports what it removed.

The plan is frozen when it is built, so paging through the preview shows one set and not a moving one. You can export it as CSV.

## Content groups, not torrents

The unit of removal is the content group: the local torrents that hold the same data. qui removes the whole group and counts its bytes once, as the largest member's size. Removing one member could delete files another member is still seeding.

Bytes count once. A torrent that shares its Content Path with a renamed copy but no files is not in the group, and its own files survive.

## Dead groups

A dead group is a content group in which no member is seeding. A torrent in the `missingFiles` state counts as not seeding. When **Remove dead groups first** is on, every dead group is removed whatever the target is, and its bytes do not count toward the target. The preview shows the dead groups and the ranked groups as two totals, so the target stays a promise about the ranked part.

## Protections

A group is excluded whole, with a reason, when any member is still downloading, when any member carries the `no_delete` tag, when the file overlap is only partial, when a file list cannot be read, or when the group's Content Path nests with another torrent's. One `no_delete` tag protects every copy of that content.

## Ranking

Groups are ranked by a chain of keys that you can reorder, switch off, and point up or down. The default chain is: fewest members first, then most seeders, then smallest group, then non-private before private. Ties break by hash. The preview names the key that decided each group's position.

## Deletion

Deletion runs in the background, so you can close the dialog. Before qui removes a group it reads the group again: a torrent added or protected since the plan was built skips the whole group with a reason. Removal goes through the same bulk delete path as the rest of qui, with files, in chunks of 50 hashes. A failed chunk does not stop the rest, and the run reports it. You can cancel a plan before deletion starts.

One plan runs per instance at a time.

## Scope

A plan reads one instance. It does not check other instances, so two instances that share a mount can lose files a torrent on the other instance still needs. The confirmation dialog states this. A plan removes files; it never removes a torrent without files.
