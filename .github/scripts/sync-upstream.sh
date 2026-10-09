#!/usr/bin/env bash
#
# Sync this fork with upstream and mirror upstream's release tags.
#
# Called by .github/workflows/sync-upstream.yml, which checks out with the
# SYNC_TOKEN personal access token. The pushes need that token: GITHUB_TOKEN is
# refused with "refusing to allow a GitHub App to create or update workflow
# ... without `workflows` permission" as soon as the pushed range holds a
# .github/workflows/* file, and upstream's develop and every release tag do.
#
# The branch merge never force-pushes: upstream is merged into the fork, so
# local commits stay.
#
# Tag rule: a tag upstream released after the newest tag this fork already has,
# plus the newest upstream tag when the fork has no tags at all (otherwise the
# first run would mirror nothing and the current release would never get an
# image). Older upstream tags are never backfilled; pass EXTRA_TAG to mirror one
# by hand. Mirroring a tag triggers this fork's release.yml, which builds and
# publishes the images for it.
#
# Environment:
#   SYNC_TOKEN     PAT with Contents + Workflows write (required in CI)
#   UPSTREAM_REPO  upstream clone URL (default https://github.com/autobrr/qui.git)
#   BRANCH         fork branch to merge upstream into (default develop)
#   EXTRA_TAG      also mirror this upstream tag, even an older one
#   SEED_NEWEST    "false" skips the newest-tag seed when the fork has no tags
#   DRY_RUN        "true" prints what it would push instead of pushing
#   GITHUB_OUTPUT  workflow output file; unset prints outputs to stderr instead
#
# Outputs: develop_moved, develop_sha, tags_pushed
set -euo pipefail

upstream_repo="${UPSTREAM_REPO:-https://github.com/autobrr/qui.git}"
branch="${BRANCH:-develop}"
extra_tag="${EXTRA_TAG:-}"
seed_newest="${SEED_NEWEST:-true}"
dry_run="${DRY_RUN:-false}"

log() { printf '%s\n' "$*" >&2; }

emit() { # emit <name> <value>
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then printf '%s=%s\n' "$1" "$2" >>"$GITHUB_OUTPUT"; fi
  log "  $1=$2"
}

if [[ "${GITHUB_ACTIONS:-}" == "true" && -z "${SYNC_TOKEN:-}" ]]; then
  log "SYNC_TOKEN is empty, so the pushes would be rejected."
  log "Add a fine-grained personal access token with Contents: read and write and"
  log "Workflows: read and write as the repository secret SYNC_TOKEN."
  exit 1
fi

# No prompt, no hang: a bad URL or a missing credential must fail the job.
export GIT_TERMINAL_PROMPT=0

# 1. Upstream and the fork, with the fork's identity for the merge commit.
git remote add upstream "$upstream_repo" 2>/dev/null || git remote set-url upstream "$upstream_repo"
# Fetch only the branch we merge, plus every tag: upstream carries ~100 branches.
git config remote.upstream.fetch "+refs/heads/${branch}:refs/remotes/upstream/${branch}"
git fetch --prune origin
git fetch --prune --tags upstream
git checkout -B "$branch" "origin/${branch}"
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

# 2. Merge upstream into the fork branch. A fast-forward leaves no merge commit;
# a diverged branch gets one; a conflict fails the job for a human.
before_sha=$(git rev-parse HEAD)
log "Merging upstream/${branch} into ${branch}"
git merge --no-edit "upstream/${branch}"
after_sha=$(git rev-parse HEAD)

develop_moved=false
if [[ "$before_sha" != "$after_sha" ]]; then
  develop_moved=true
  if [[ "$dry_run" == "true" ]]; then
    log "would push ${branch} at ${after_sha}"
  else
    git push origin "HEAD:refs/heads/${branch}"
  fi
else
  log "${branch} is already at upstream/${branch}"
fi

# 3. Mirror the tags upstream released since the newest tag this fork has.
list_tags() { # list_tags <remote>
  git ls-remote --tags --refs "$1" | awk '{print $2}' | sed 's|^refs/tags/||' | grep -E '^v[0-9]' || true
}
is_newer() { # is_newer <candidate> <floor>
  [[ "$1" != "$2" && "$(printf '%s\n%s\n' "$2" "$1" | sort -V | tail -1)" == "$1" ]]
}

fork_tags="$(list_tags origin)"
upstream_tags="$(list_tags upstream)"
fork_max="$(printf '%s\n' "$fork_tags" | grep -v '^$' | sort -V | tail -1 || true)"

candidates=()
if [[ -z "$fork_max" && "$seed_newest" == "true" ]]; then
  newest="$(printf '%s\n' "$upstream_tags" | grep -v '^$' | sort -V | tail -1 || true)"
  if [[ -n "$newest" ]]; then candidates+=("$newest"); fi
else
  while read -r tag; do
    [[ -n "$tag" ]] || continue
    if is_newer "$tag" "$fork_max"; then candidates+=("$tag"); fi
  done <<<"$upstream_tags"
fi
if [[ -n "$extra_tag" ]]; then candidates+=("$extra_tag"); fi

tags_pushed=""
for tag in $(printf '%s\n' "${candidates[@]:-}" | grep -v '^$' | sort -u -V); do
  if grep -qxF "$tag" <<<"$fork_tags"; then
    log "tag $tag is already in the fork"
    continue
  fi
  # The annotated tag object is pushed as-is, so the fork's tag matches upstream.
  if [[ "$dry_run" == "true" ]]; then
    log "would push tag $tag"
  else
    git push origin "refs/tags/${tag}:refs/tags/${tag}"
  fi
  tags_pushed+="${tag} "
done

emit "develop_moved" "$develop_moved"
emit "develop_sha" "$after_sha"
emit "tags_pushed" "${tags_pushed% }"

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    printf '## Sync upstream\n\n'
    printf -- '- `%s` moved: %s (`%s`)\n' "$branch" "$develop_moved" "$after_sha"
    printf -- '- tags mirrored: %s\n' "${tags_pushed:-none}"
    printf -- '- a mirrored tag and a moved branch both trigger this fork release.yml, which builds the images\n'
  } >>"$GITHUB_STEP_SUMMARY"
fi
