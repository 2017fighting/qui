---
status: accepted
date: 2026-10-08
---

# A skipped file still counts as missing

The Skip small files automation action sets every file below a size threshold to download priority 0 ("Do not download"). `HAS_MISSING_FILES` keeps answering one question, whether the disk holds every file the torrent lists, so a skipped file that is not on disk still reports missing. `HAS_SKIPPED_FILES` carries the intent. A user who wants both readings gates on the two fields together. A torrent whose every file is below the threshold is skipped whole, because skipping all of them would leave it with zero wanted bytes.

## Considered options

- **Exclude priority 0 files from the missing-files check.** Rejected: qBittorrent does not record who unchecked a file, so a hand-unchecked file and a rule-set one are the same API state. The filter would hide real missing files in every torrent where a person ever unchecked one, and qui would have to remember the intent itself.
- **Add a second condition, `HAS_MISSING_WANTED_FILES`.** Rejected: two fields that differ only in a filter whose input the client does not expose. The pair `HAS_MISSING_FILES` + `HAS_SKIPPED_FILES` already separates the two readings.
- **Reject a rule that combines the action with a `HAS_MISSING_FILES` condition.** Rejected: rules stay independent, and the combination is legitimate — a pass that skips a file can miss for a real reason on a later pass, and the pair of fields states that.
- **Skip all but the largest file, and let a torrent with one file skip nothing.** Rejected: the guard exists to keep a torrent from reaching zero wanted bytes, not to choose a survivor. A threshold the user picked decides which files are skipped, and one rule that says "under this size" should not quietly except a file for being the biggest.

## Consequences

- The action's helper text and `documentation/docs/features/automations.md` state the effect, so the combination is a documented reading, not a surprise.
- `buildMissingFilesResult` in `internal/services/automations/missing_files.go` stays free of priority checks. Reversing this decision means a new ADR plus a change to that function and its tests.
- The dry-run preview reports the files the action would skip per torrent, which is where a user sees the effect before it reaches `HAS_MISSING_FILES`.
