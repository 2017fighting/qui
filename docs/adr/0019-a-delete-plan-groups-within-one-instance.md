---
status: accepted
date: 2026-10-14
---

# A delete plan groups within one instance

A delete plan reads the torrents of one instance and groups them against each other. It does not look at another instance, even though the cross-seed delete dialog does: `crossseed.FindLocalMatches` walks every instance, because it answers a question about one torrent and a wrong answer only costs a warning.

A plan cannot remove the other instance's torrent, so a cross-instance check would only protect it. That protection needs the other instance's file lists and its filesystem identity, and it changes the answer for a shared mount only. The confirmation dialog says that the plan does not check other instances, so the one accepted risk is stated rather than implied.

## Considered options

- **Check other instances for a matching torrent and protect the group.** Rejected for this version: the cost is every instance's file lists, and the common deployment gives each instance its own storage. Revisit if a shared mount turns out to be common.
- **Refuse to plan when another instance holds a matching torrent.** Rejected: it blocks the ordinary case to guard the rare one.

## Consequences

- Two instances that share a mount can lose files a torrent on the other instance still needs. The dialog names it.
- A later cross-instance check is additive: it needs no change to the group contract, only a wider file index.
