#!/usr/bin/env bash
#
# End-to-end acceptance test for the consuming half of Quartermaster.
#
# Builds a bundle from a real store, initializes a scratch repository against
# it, and asserts that every content type the store carries actually lands where
# the harness expects it. Then it exercises the rest of the consumer surface,
# including the paths that are supposed to fail: a tampered file must fail
# verify, and a dropped ruleset must prune what it produced.
#
# It touches nothing outside its own temporary directory. The cache, usage log,
# spool, and facet directories are all redirected, so running this never writes
# into a developer's real state.
#
# Usage:
#   scripts/acceptance.sh [path-to-store]
#
# The store defaults to ../admiral-knowledge/store. Any OKF store works; what
# the assertions need is at least one rule, one skill, one agent, and a
# knowledge tree, which the script verifies before it starts asserting.

set -uo pipefail

STORE="${1:-$(cd "$(dirname "$0")/../../../admiral/admiral-knowledge/store" 2>/dev/null && pwd)}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); printf '  ok    %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf '  FAIL  %s\n' "$1"; }

# exists asserts a path is present.
exists() {
  if [ -e "$2" ]; then pass "$1"; else fail "$1 (missing $2)"; fi
}

# absent asserts a path is not present, which is how pruning is checked.
absent() {
  if [ ! -e "$2" ]; then pass "$1"; else fail "$1 (still present: $2)"; fi
}

# contains asserts a file holds a string.
contains() {
  if grep -q -- "$3" "$2" 2>/dev/null; then pass "$1"; else fail "$1 (not in $2: $3)"; fi
}

# succeeds and refuses assert on exit status. A tool that cannot fail is a tool
# that cannot tell you anything, so both directions are tested.
succeeds() {
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then pass "$label"; else fail "$label (exit $?)"; fi
}

refuses() {
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then fail "$label (succeeded, expected failure)"; else pass "$label"; fi
}

section() { printf '\n%s\n' "$1"; }

# ---------------------------------------------------------------------------

if [ ! -d "$STORE" ]; then
  echo "no store at $STORE" >&2
  echo "usage: $0 [path-to-store]" >&2
  exit 2
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export QM_CACHE_DIR="$TMP/cache"
export QM_USAGE_DIR="$TMP/usage"
export QM_TRACE_DIR="$TMP/spool"
export QM_FACET_DIR="$TMP/facets"

QM="$TMP/qm"
REPO="$TMP/repo"

printf 'store   %s\n' "$STORE"
printf 'scratch %s\n' "$TMP"

section 'build'
if ! (cd "$ROOT" && go build -o "$QM" .); then
  echo "could not build qm" >&2
  exit 2
fi
pass "qm builds"
succeeds "qm bundle validate accepts the store" "$QM" bundle validate --root "$STORE"

# What the store offers decides what can be asserted, so read it rather than
# assume it. A store with no agent cannot prove agents install.
SKILL_ID="$(grep -rl '^type: skill' "$STORE" | head -1 | xargs grep -h '^id:' | awk '{print $2}')"
AGENT_ID="$(grep -rl '^type: agent' "$STORE" | head -1 | xargs grep -h '^id:' | awk '{print $2}')"
RULESET="$(grep -E '^[a-z][a-z-]*:' "$STORE"/meta/rulesets.yaml | head -1 | tr -d ':')"

printf '  ruleset=%s skill=%s agent=%s\n' "${RULESET:-none}" "${SKILL_ID:-none}" "${AGENT_ID:-none}"
[ -n "$RULESET" ] && pass "store offers a ruleset" || fail "store offers a ruleset"
[ -n "$SKILL_ID" ] && pass "store offers a skill" || fail "store offers a skill"
[ -n "$AGENT_ID" ] && pass "store offers an agent" || fail "store offers an agent"

section 'init'
mkdir -p "$REPO"
git -C "$REPO" init -q
git -C "$REPO" remote add origin git@github.com:example/acceptance.git
git -C "$REPO" commit -q --allow-empty -m "initial"

succeeds "qm init" "$QM" init --dir "$REPO" --source "file://$STORE" --target claude --ruleset "$RULESET"
exists   "manifest written"            "$REPO/.quartermaster.yaml"
contains "manifest names the ruleset"  "$REPO/.quartermaster.yaml" "$RULESET"
contains "telemetry opted in"          "$REPO/.quartermaster.yaml" "telemetry: true"
exists   "gitignore written"           "$REPO/.gitignore"
contains "gitignore covers qm state"   "$REPO/.gitignore" "/.quartermaster/"
exists   "post-checkout hook"          "$REPO/.git/hooks/post-checkout"
exists   "post-merge hook"             "$REPO/.git/hooks/post-merge"
exists   "session hook settings"       "$REPO/.claude/settings.json"
contains "session hook is guarded"     "$REPO/.claude/settings.json" "command -v qm"

section 'rules and knowledge materialize'
exists "rules directory"    "$REPO/.claude/rules/qm"
exists "knowledge tree"     "$REPO/.quartermaster/knowledge"
exists "state file"         "$REPO/.quartermaster/state.json"

RULES="$(find "$REPO/.claude/rules/qm" -name '*.md' | wc -l | tr -d ' ')"
if [ "$RULES" -gt 0 ]; then pass "at least one rule rendered ($RULES)"; else fail "at least one rule rendered"; fi

DOCS="$(find "$REPO/.quartermaster/knowledge" -name '*.md' | wc -l | tr -d ' ')"
if [ "$DOCS" -gt 0 ]; then pass "knowledge documents on disk ($DOCS)"; else fail "knowledge documents on disk"; fi

# A rule with no scope loads every session; a scoped one declares paths. Both
# shapes must survive rendering, because the distinction is the whole budget
# argument.
if grep -rlq '^paths:' "$REPO/.claude/rules/qm" 2>/dev/null; then
  pass "a scoped rule declares paths"
else
  printf '  note  no scoped rule in this ruleset; scope rendering not exercised\n'
fi

section 'skills and agents install when selected'
# Skills and agents are named in the manifest by id rather than selected by a
# ruleset, so this is what a repository opting into one looks like.
python3 - "$REPO/.quartermaster.yaml" "$SKILL_ID" "$AGENT_ID" <<'PY'
import sys, pathlib
path, skill, agent = sys.argv[1], sys.argv[2], sys.argv[3]
p = pathlib.Path(path)
lines = p.read_text().splitlines(keepends=True)
out = []
for line in lines:
    out.append(line)
    if line.strip().startswith("rulesets:"):
        if skill:
            out.append(f"    skills: [{skill}]\n")
        if agent:
            out.append(f"    agents: [{agent}]\n")
p.write_text("".join(out))
PY
succeeds "qm sync after selecting a skill and an agent" "$QM" sync --dir "$REPO"

if [ -n "$SKILL_ID" ]; then
  SKILL_NAME="${SKILL_ID##*.}"
  exists   "skill directory"        "$REPO/.claude/skills/$SKILL_NAME"
  exists   "skill file"             "$REPO/.claude/skills/$SKILL_NAME/SKILL.md"
  contains "skill declares a name"  "$REPO/.claude/skills/$SKILL_NAME/SKILL.md" "name:"
  exists   "skill ignores itself"   "$REPO/.claude/skills/$SKILL_NAME/.gitignore"
fi

if [ -n "$AGENT_ID" ]; then
  # Agents are filed by document id, not by declared name, which is how they
  # cannot collide with an agent somebody wrote by hand. Skills are filed by
  # name instead, so the two disagree; this asserts what actually ships.
  exists   "agent namespaced under qm"     "$REPO/.claude/agents/qm/$AGENT_ID.md"
  contains "agent declares a harness name" "$REPO/.claude/agents/qm/$AGENT_ID.md" "name:"
  contains "agent says where it came from" "$REPO/.claude/agents/qm/$AGENT_ID.md" "source: $AGENT_ID"
fi

section 'verify catches drift'
succeeds "qm verify passes on a clean tree" "$QM" verify --dir "$REPO"

VICTIM="$(find "$REPO/.claude/rules/qm" -name '*.md' | head -1)"
echo "tampered" >> "$VICTIM"
refuses  "qm verify fails on an edited rule" "$QM" verify --dir "$REPO"
succeeds "qm sync repairs it"                "$QM" sync --dir "$REPO"
succeeds "qm verify passes again"            "$QM" verify --dir "$REPO"

rm -f "$VICTIM"
refuses  "qm verify fails on a deleted rule" "$QM" verify --dir "$REPO"
succeeds "qm sync restores it"               "$QM" sync --dir "$REPO"

section 'reporting commands'
succeeds "qm status"  "$QM" status --dir "$REPO"
succeeds "qm targets" "$QM" targets
succeeds "qm stats"   "$QM" stats --dir "$REPO"
"$QM" status --dir "$REPO" >"$TMP/status.txt" 2>&1
contains "status names the source"  "$TMP/status.txt" "file://"
contains "status reports residency" "$TMP/status.txt" "resident"

section 'a second target renders alongside the first'
python3 - "$REPO/.quartermaster.yaml" <<'PY'
import sys, pathlib
p = pathlib.Path(sys.argv[1])
p.write_text(p.read_text().replace("targets:\n  - claude\n", "targets:\n  - claude\n  - cursor\n"))
PY
succeeds "qm sync with a second target" "$QM" sync --dir "$REPO"
exists   "cursor rules rendered"        "$REPO/.cursor/rules/qm"
exists   "claude rules still there"     "$REPO/.claude/rules/qm"

section 'pruning removes what is no longer selected'
BEFORE="$(find "$REPO/.claude/rules/qm" -name '*.md' | wc -l | tr -d ' ')"
python3 - "$REPO/.quartermaster.yaml" <<'PY'
import sys, pathlib, re
p = pathlib.Path(sys.argv[1])
p.write_text(re.sub(r"^\s*rulesets:.*$", "    rulesets: []", p.read_text(), flags=re.M))
PY
succeeds "qm sync with no rulesets" "$QM" sync --dir "$REPO"
AFTER="$(find "$REPO/.claude/rules/qm" -name '*.md' 2>/dev/null | wc -l | tr -d ' ')"
if [ "$AFTER" -lt "$BEFORE" ]; then
  pass "rules pruned when deselected ($BEFORE to $AFTER)"
else
  fail "rules pruned when deselected (still $AFTER)"
fi
exists "knowledge survives losing a ruleset" "$REPO/.quartermaster/knowledge"

section 'telemetry records only where it should'
DOC="$(find "$REPO/.quartermaster/knowledge" -name '*.md' ! -name index.md | head -1)"
succeeds "qm usage record on a knowledge document" "$QM" usage record "$DOC"
if [ -n "$(find "$QM_USAGE_DIR" -name '*.jsonl' 2>/dev/null)" ]; then
  pass "usage log written"
else
  fail "usage log written"
fi
succeeds "qm usage record ignores a non-knowledge path" "$QM" usage record "$REPO/.quartermaster.yaml"

printf '{"session_id":"acceptance-1","cwd":"%s","hook_event_name":"SessionEnd"}' "$REPO" \
  | "$QM" trace record >/dev/null 2>&1
if [ -s "$QM_TRACE_DIR/pending.jsonl" ]; then
  pass "session spooled"
  contains "spool names the repository, not the path" "$QM_TRACE_DIR/pending.jsonl" "example/acceptance"
else
  fail "session spooled"
fi

section 'result'
printf '%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
