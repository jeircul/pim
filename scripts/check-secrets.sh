#!/usr/bin/env sh
# check-secrets.sh — scan staged content and key doc files for sensitive identifiers.
# Exits non-zero and prints offending file:line on any hit.
# Run via: task check
# Wired as a precondition of task release:dev and task release:stable.

set -e

FAIL=0
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Allowlisted GUID prefixes that are known synthetic fixtures.
GUID_ALLOWLIST="00000000|aaaaaaaa|bbbbbbbb|12345678|11111111|22222222|33333333|44444444|55555555|66666666|77777777|88888888|99999999"

# --- Helper ---
check_content() {
  label="$1"
  content="$2"

  # Real GUID check: matches a full UUID not in the allowlist.
  hits=$(printf '%s' "$content" | grep -ioE '[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}' \
    | grep -vEi "^($GUID_ALLOWLIST)" || true)
  if [ -n "$hits" ]; then
    printf 'FAIL [real GUID] in %s:\n' "$label"
    printf '%s\n' "$hits"
    FAIL=1
  fi

  # Blocklist check: skip if file absent.
  BLOCKLIST="$REPO_ROOT/.secrets-blocklist"
  if [ -f "$BLOCKLIST" ]; then
    while IFS= read -r word || [ -n "$word" ]; do
      [ -z "$word" ] && continue
      case "$word" in '#'*) continue ;; esac
      if printf '%s' "$content" | grep -qiF "$word"; then
        printf 'FAIL [blocklist: %s] in %s\n' "$word" "$label"
        FAIL=1
      fi
    done < "$BLOCKLIST"
  fi
}

# --- Scan staged content ---
staged=$(git -C "$REPO_ROOT" diff --cached --name-only 2>/dev/null || true)
for f in $staged; do
  full="$REPO_ROOT/$f"
  [ -f "$full" ] || continue
  # Only text files.
  case "$f" in *.go|*.md|*.toml|*.yml|*.yaml|*.sh|*.json|*.txt) ;;
    *) continue ;;
  esac
  content=$(git -C "$REPO_ROOT" show ":$f" 2>/dev/null || true)
  check_content "$f (staged)" "$content"
done

# --- Always scan key doc files (working tree) ---
for doc in README.md CHANGELOG.md; do
  full="$REPO_ROOT/$doc"
  [ -f "$full" ] || continue
  check_content "$doc" "$(cat "$full")"
done

if [ "$FAIL" -ne 0 ]; then
  printf '\ncheck-secrets: sensitive identifiers detected. Replace with placeholders before committing.\n'
  printf 'See AGENTS.md "Public-artifact sanitization" for the placeholder vocabulary.\n'
  exit 1
fi

printf 'check-secrets: OK\n'
