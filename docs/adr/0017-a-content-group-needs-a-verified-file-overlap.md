---
status: accepted
date: 2026-10-14
---

# A content group needs a verified file overlap

A content group is the unit a delete plan and an automation delete action expand to. Membership needs both halves: the normalized Content Paths are equal, and a file-overlap check confirms that at least 90% of the smaller torrent's bytes resolve to the same on-disk paths. A member that shares no file is dropped, a member that overlaps partly (above zero, below 90%) drops the whole group, and a member whose file list cannot be read drops the whole group. A torrent whose Content Path nests inside or around the Content Path of a torrent outside the group drops the group too: two nested paths land in different buckets, so the overlap check never compares them, and the delete removes the files of the path.

Release metadata never creates a group. Cross-seed search reads release names and the corpus lists the differences between its match paths; a delete path must not, because a title is weaker evidence than a shared file and the delete removes files. The grouping lives in one package that both automations and the delete plan call, so the two cannot disagree about a pair.

## Considered options

- **Reuse `crossseed.FindLocalMatches`.** Rejected: it walks every instance, it answers for one source torrent at a time, and its release-metadata tiers would authorize a file deletion on a title match.
- **Content Path only, no file check.** Rejected: qBittorrent reports the storage directory as the Content Path of a multi-file torrent when "create subfolder" is off, so unrelated packs that share a directory would form one group and lose their files together.
- **Hardlink signature only.** Rejected: it needs Local Filesystem Access on the instance, and it misses the ordinary case where two torrents point at the same path without a hardlink.
- **Answer the ambiguous case with `ambiguousPolicy: "skip"` everywhere.** Rejected as the default: `verify_overlap` keeps real cross-seed groups, and `skip` stays available for an automation action whose owner chose it.

## Consequences

- A renamed cross-seed with its own Content Path is not in the group, so the plan does not know about it. Its own files survive a delete of the group, because the delete removes the group's path. Nothing warns about it either.
- The nested-path guard is part of the contract, not a plan-only extra. An automation delete action inherits it and stops expanding to such a group. That is a behaviour change to automations, and it is a safety improvement, so `documentation/docs/features/automations.md` states it.
- A file list that cannot be read drops the group instead of reading as "no cross-seeds", the same way the delete dialogs already fail closed.
- `internal/services/crossseed/match_corpus_test.go` keeps its scope: it covers release matching. The grouping has its own table of scenarios.
