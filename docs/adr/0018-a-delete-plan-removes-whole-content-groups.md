---
status: accepted
date: 2026-10-14
---

# A delete plan removes whole content groups

A delete plan never removes one torrent of a content group. It removes the group, and it counts the group's bytes once, as the `size` of its largest member. The reason is that qBittorrent's delete-with-files removes the files of the path: removing one member of a group can delete the files another member is seeding.

Group membership is a safety input, not only a unit. Any member with `progress < 1` protects the whole group. Any member that carries the delete protection tag protects the whole group. A group whose members changed between planning and deleting is skipped whole rather than partly removed.

## Dead groups leave the target arithmetic

A dead group is a content group in which no member is seeding. When the dead-group rule is on, the plan removes every dead group whatever the target is, and their bytes do not count toward it. The preview shows the dead groups and the ranked groups as two totals, so the run reports what it removed instead of promising it under a target the user typed.

## Considered options

- **One row per torrent.** Rejected: the same data would count toward the target more than once, and the user would see a plan whose numbers cannot be reached by the deletion.
- **Remove the whole group when any member matches the ranking.** Rejected: it hides a decision that costs more bytes than the row shows.
- **Fold dead groups into the target arithmetic.** Rejected: a torrent in the `missingFiles` state has no files on disk, so counting its `size` promises bytes that cannot come back.
- **Let dead groups be a ranking key only.** Rejected: a paused torrent is often deliberate, and the user asked for them to go first regardless of the target. Keeping them a switch with a separate total keeps the promise of the target intact.

## Consequences

- The confirmation dialog names the two parts, so the extra removal is visible before the run and not after it.
- The plan's totals are a promise about the ranked part only.
- A dead group with one member still seeding is not dead, because the group is the unit.
