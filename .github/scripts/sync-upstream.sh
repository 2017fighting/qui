#!/usr/bin/env bash
#
# Sync this fork with upstream and mirror newly released tags.
#
# Called by .github/workflows/sync-upstream.yml. The branch merge never
# force-pushes: upstream is merged into the fork, so local commits stay.
#
# Tag rule: a tag upstream released after the newest tag this fork already has,
# plus the newest upstream tag when the fork has no tags at all (otherwise the
# first run would mirror nothing and the current release would never get an
# image). Older upstream tags are never backfilled; pass EXTRA_TAG to mirror one
# by hand.
#
# Environment:
#   UPSTREAM_REPO  upstream clone URL (default https://github.com/autobrr/qui.git)
#   BRANCH         fork branch to merge upstream into (default develop)
#   EXTRA_TAG      also mirror this upstream tag, even an older one
#   SEED_NEWEST    "false" skips the newest-tag seed when the fork has no tags
#   FORCE_BUILD    "true" builds the images even when nothing moved
#   DRY_RUN        "true" prints what it would push instead of pushing
#   GITHUB_OUTPUT  workflow output file; unset prints outputs to stderr instead
#
# Outputs: develop_moved, develop_sha, tags_pushed, targets (build matrix JSON)
set -euo pipefail

upstream_repo="${UPSTREAM_REPO:-https://github.com/autobrr/qui.git}"
branch="${BRANCH:-develop}"
extra_tag="${EXTRA_TAG:-}"
seed_newest="${SEED_NEWEST:-true}"
force_build="${FORCE_BUILD:-false}"
dry_run="${DRY_RUN:-false}"
image="${IMAGE:-ghcr.io/${GITHUB_REPOSITORY:-autobrr/qui}}"

log() { printf '%s\n' "$*" >&2; }

emit() { # emit <name> <value>
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then printf '%s=%s\n' "$1" "$2" >>"$GITHUB_OUTPUT"; fi
  log "  $1=$2"
}

# No prompt, no hang: a bad URL or a missing credential must fail the job.
export GIT_TERMINAL_PROMPT=0

targets_json="["
first_target=1
# add_target <ref> <version> <sha> <tag suffixes, comma separated>
# The tag list is written as fully qualified image references, one per line: the
# build action reads `ghcr.io/x/y:1.2.3,1.2` as the repositories "1.2" and
# "latest" rather than as tags of the image.
add_target() { # add_target <ref> <version> <sha> <tag suffixes, comma separated>
  local refs=""
  local separator=""
  local suffix
  for suffix in ${4//,/ }; do
    refs+="${separator}${image}:${suffix}"
    separator='\n'
  done
  [[ "$first_target" -eq 1 ]] || targets_json+=","
  first_target=0
  targets_json+="$(printf '{"ref":"%s","version":"%s","sha":"%s","tags":"%s","date":"%s"}' \
    "$1" "$2" "$3" "$refs" "$(date -u +%Y-%m-%dT%H:%M:%SZ)")"
}

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

# The newest upstream tag at this moment, used to decide which image carries
# latest: a hand-mirrored older tag must not move latest backwards.
newest_upstream="$(printf '%s\n' "$upstream_tags" | grep -v '^$' | sort -V | tail -1 || true)"

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
  tag_sha=$(git rev-parse "refs/tags/${tag}^{commit}")
  tag_tags="$tag"
  # v1.31.1 also carries v1.31; latest only moves forward, on the newest stable tag.
  tag_tags+=",v$(cut -d. -f1,2 <<<"${tag#v}")"
  if [[ "$tag" != *-* && "$tag" == "$newest_upstream" ]]; then tag_tags+=",latest"; fi
  add_target "refs/tags/${tag}" "$tag" "$tag_sha" "$tag_tags"
done

if [[ "$develop_moved" == "true" || "$force_build" == "true" ]]; then
  add_target "refs/heads/${branch}" "$branch" "$after_sha" "$branch"
fi

targets_json+="]"
emit "develop_moved" "$develop_moved"
emit "develop_sha" "$after_sha"
emit "tags_pushed" "${tags_pushed% }"
emit "targets" "$targets_json"

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    printf '## Sync upstream\n\n'
    printf -- '- `%s` moved: %s (`%s`)\n' "$branch" "$develop_moved" "$after_sha"
    printf -- '- tags mirrored: %s\n' "${tags_pushed:-none}"
    printf -- '- images to build: %s\n' "$targets_json"
  } >>"$GITHUB_STEP_SUMMARY"
fi
