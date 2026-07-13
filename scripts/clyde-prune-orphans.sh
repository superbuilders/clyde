#!/usr/bin/env bash
#
# clyde-prune-orphans.sh
#
# Finds and deletes tool-use files in a clyde session that have no
# corresponding tool-result file (matched by toolu_* ID).
#
# Usage:
#   ./clyde-prune-orphans.sh <session-name> [--dry-run]
#
# Examples:
#   ./clyde-prune-orphans.sh 2026-07-01T14-49-58_thisistheaj --dry-run
#   ./clyde-prune-orphans.sh 2026-07-01T14-49-58_thisistheaj
#

set -euo pipefail

# --- Config ---
SESSIONS_DIR=".clyde/sessions"

# --- Args ---
if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <session-name> [--dry-run]"
    echo ""
    echo "Finds tool-use blocks with no matching tool-result (by toolu_* ID)"
    echo "and deletes them from the session."
    exit 1
fi

SESSION="$1"
DRY_RUN=false
if [[ "${2:-}" == "--dry-run" ]]; then
    DRY_RUN=true
fi

SESSION_DIR="$SESSIONS_DIR/$SESSION"

# --- Validate ---
if [[ ! -d "$SESSION_DIR" ]]; then
    echo "ERROR: Session directory not found: $SESSION_DIR"
    echo ""
    echo "Available sessions:"
    ls "$SESSIONS_DIR" 2>/dev/null | head -20
    exit 1
fi

# --- Collect tool-result IDs into a temp file (sorted) ---
result_ids_file=$(mktemp)
trap "rm -f '$result_ids_file'" EXIT

for f in "$SESSION_DIR"/*_tool-result.md; do
    [[ -f "$f" ]] || continue
    grep -o 'toolu_[^]]*' "$f" | head -1
done | sort -u > "$result_ids_file"

# --- Find orphaned tool-use files ---
orphan_count=0
orphan_files=()

for f in "$SESSION_DIR"/*_tool-use.md; do
    [[ -f "$f" ]] || continue
    id=$(grep -o 'toolu_[^]]*' "$f" | head -1)
    if [[ -z "$id" ]]; then
        echo "WARNING: No tool ID found in $f — skipping"
        continue
    fi

    # Check if this ID exists in the results
    if ! grep -q "^${id}$" "$result_ids_file"; then
        orphan_count=$((orphan_count + 1))
        orphan_files+=("$f")

        # Extract the tool name for display
        tool_name=$(grep '^name: ' "$f" | head -1 | sed 's/^name: //')
        first_line=$(head -1 "$f")
        echo "ORPHAN #$orphan_count: $(basename "$f")"
        echo "  ID:   $id"
        echo "  Tool: ${tool_name:-unknown}"
        echo "  Head: $first_line"
        echo ""
    fi
done

# --- Summary & action ---
total_uses=$(ls "$SESSION_DIR"/*_tool-use.md 2>/dev/null | wc -l | tr -d ' ')
total_results=$(ls "$SESSION_DIR"/*_tool-result.md 2>/dev/null | wc -l | tr -d ' ')

echo "========================================="
echo "Session:       $SESSION"
echo "Tool-use:      $total_uses files"
echo "Tool-result:   $total_results files"
echo "Orphaned:      $orphan_count files"
echo "========================================="

if [[ $orphan_count -eq 0 ]]; then
    echo "✓ No orphaned tool-use blocks found. Nothing to do."
    exit 0
fi

if $DRY_RUN; then
    echo ""
    echo "[DRY RUN] Would delete $orphan_count file(s):"
    for f in "${orphan_files[@]}"; do
        echo "  rm $f"
    done
    echo ""
    echo "Run without --dry-run to actually delete."
else
    echo ""
    echo "Deleting $orphan_count orphaned tool-use file(s)..."
    for f in "${orphan_files[@]}"; do
        rm -v "$f"
    done
    echo ""
    echo "✓ Done. Deleted $orphan_count orphaned tool-use file(s)."
fi
