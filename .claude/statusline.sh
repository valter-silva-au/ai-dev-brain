#!/usr/bin/env bash
# Claude Code Status Line for AI Dev Brain (adb)
# Combines Claude Code session data with adb task context.
# Receives JSON session data on stdin, prints formatted status to stdout.

# --- Color definitions (ANSI) ---
RESET='\033[0m'
BOLD='\033[1m'
DIM='\033[2m'
RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
BLUE='\033[34m'
MAGENTA='\033[35m'
CYAN='\033[36m'
WHITE='\033[37m'
BG_RED='\033[41m'

# --- Read JSON session data from stdin ---
SESSION_JSON=$(cat)

# --- Parse session fields via jq (with fallback) ---
parse_json() {
    local field="$1"
    local default="$2"
    local val=""
    if command -v jq >/dev/null 2>&1; then
        val=$(echo "$SESSION_JSON" | jq -r "$field // empty" 2>/dev/null)
    fi
    echo "${val:-$default}"
}

MODEL_NAME=$(parse_json '.model.display_name' '?')
CONTEXT_PCT=$(parse_json '.context_window.used_percentage' '?')
COST_USD=$(parse_json '.cost.total_cost_usd' '0')
DURATION_MS=$(parse_json '.cost.total_duration_ms' '0')
LINES_ADDED=$(parse_json '.cost.total_lines_added' '0')
LINES_REMOVED=$(parse_json '.cost.total_lines_removed' '0')
AGENT_NAME=$(parse_json '.agent.name' '')

# --- Compute session duration ---
if [ "$DURATION_MS" != "0" ] && [ "$DURATION_MS" != "?" ]; then
    DURATION_SEC=$((DURATION_MS / 1000))
    DURATION_MIN=$((DURATION_SEC / 60))
    DURATION_REM=$((DURATION_SEC % 60))
    if [ "$DURATION_MIN" -gt 59 ]; then
        DURATION_HR=$((DURATION_MIN / 60))
        DURATION_MIN=$((DURATION_MIN % 60))
        SESSION_TIME="${DURATION_HR}h${DURATION_MIN}m"
    elif [ "$DURATION_MIN" -gt 0 ]; then
        SESSION_TIME="${DURATION_MIN}m${DURATION_REM}s"
    else
        SESSION_TIME="${DURATION_SEC}s"
    fi
else
    SESSION_TIME="0s"
fi

# --- Format cost ---
if [ "$COST_USD" != "0" ] && [ "$COST_USD" != "?" ]; then
    COST_FMT=$(printf '$%.2f' "$COST_USD" 2>/dev/null || echo "\$${COST_USD}")
else
    COST_FMT='$0.00'
fi

# --- Context window color ---
ctx_color() {
    local pct="$1"
    if [ "$pct" = "?" ] || [ -z "$pct" ]; then
        echo "$DIM"
        return
    fi
    if [ "$pct" -lt 50 ] 2>/dev/null; then
        echo "$GREEN"
    elif [ "$pct" -lt 80 ] 2>/dev/null; then
        echo "$YELLOW"
    else
        echo "$RED"
    fi
}
CTX_COLOR=$(ctx_color "$CONTEXT_PCT")

# --- Resolve ADB workspace root (walk up to .taskconfig) ---
find_adb_root() {
    local dir
    dir="$(pwd)"
    while [ "$dir" != "/" ] && [ "$dir" != "." ]; do
        if [ -f "$dir/.taskconfig" ]; then
            echo "$dir"
            return
        fi
        dir="$(dirname "$dir")"
    done
    if [ -n "${ADB_HOME:-}" ] && [ -f "$ADB_HOME/.taskconfig" ]; then
        echo "$ADB_HOME"
        return
    fi
    echo ""
}

ADB_ROOT=$(find_adb_root)

# --- Task context (from env vars or status.yaml) ---
TASK_ID="${ADB_TASK_ID:-}"
TASK_BRANCH="${ADB_BRANCH:-}"
TASK_TYPE=""
TASK_PRIORITY=""
TASK_STATUS=""
TASK_REPO=""

# Try to detect task from directory name if env var not set
if [ -z "$TASK_ID" ]; then
    DIRNAME=$(basename "$(pwd)")
    if echo "$DIRNAME" | grep -qE '^TASK-[0-9]+$'; then
        TASK_ID="$DIRNAME"
    fi
fi

# Read status.yaml if we have a task ID and ADB root
if [ -n "$TASK_ID" ] && [ -n "$ADB_ROOT" ]; then
    STATUS_FILE="$ADB_ROOT/tickets/$TASK_ID/status.yaml"
    if [ ! -f "$STATUS_FILE" ]; then
        STATUS_FILE="$ADB_ROOT/tickets/_archived/$TASK_ID/status.yaml"
    fi
    if [ -f "$STATUS_FILE" ]; then
        TASK_TYPE=$(grep '^type:' "$STATUS_FILE" 2>/dev/null | head -1 | sed 's/^type:[[:space:]]*//')
        TASK_PRIORITY=$(grep '^priority:' "$STATUS_FILE" 2>/dev/null | head -1 | sed 's/^priority:[[:space:]]*//' | tr -d '"')
        TASK_STATUS=$(grep '^status:' "$STATUS_FILE" 2>/dev/null | head -1 | sed 's/^status:[[:space:]]*//')
        if [ -z "$TASK_BRANCH" ]; then
            TASK_BRANCH=$(grep '^branch:' "$STATUS_FILE" 2>/dev/null | head -1 | sed 's/^branch:[[:space:]]*//')
        fi
        TASK_REPO=$(grep '^repo:' "$STATUS_FILE" 2>/dev/null | head -1 | sed 's/^repo:[[:space:]]*//')
    fi
fi

# --- Short repo name (last path segment, strip .git) ---
REPO_SHORT=""
if [ -n "$TASK_REPO" ]; then
    REPO_SHORT=$(echo "$TASK_REPO" | sed 's/\.git$//' | sed 's|.*[/\\:]||')
fi

# --- Priority color ---
pri_color() {
    case "$1" in
        P0) echo "${BG_RED}${WHITE}${BOLD}" ;;
        P1) echo "${RED}${BOLD}" ;;
        P2) echo "$CYAN" ;;
        P3) echo "$DIM" ;;
        *)  echo "$DIM" ;;
    esac
}

# --- Status indicator ---
status_icon() {
    case "$1" in
        in_progress) echo "${GREEN}*${RESET}" ;;
        blocked)     echo "${RED}!${RESET}" ;;
        review)      echo "${YELLOW}?${RESET}" ;;
        backlog)     echo "${DIM}.${RESET}" ;;
        done)        echo "${BLUE}+${RESET}" ;;
        *)           echo "" ;;
    esac
}

# --- Portfolio counts from backlog.yaml (cached 30s) ---
CACHE_TTL=30
BACKLOG_COUNT=0
ACTIVE_COUNT=0
BLOCKED_COUNT=0
REVIEW_COUNT=0
DONE_COUNT=0

if [ -n "$ADB_ROOT" ] && [ -f "$ADB_ROOT/backlog.yaml" ]; then
    CACHE_FILE="/tmp/adb-statusline-portfolio"
    CACHE_STALE=true
    if [ -f "$CACHE_FILE" ]; then
        CACHE_AGE=$(( $(date +%s) - $(date -r "$CACHE_FILE" +%s 2>/dev/null || echo 0) ))
        if [ "$CACHE_AGE" -lt "$CACHE_TTL" ] 2>/dev/null; then
            CACHE_STALE=false
        fi
    fi

    if [ "$CACHE_STALE" = true ]; then
        BF="$ADB_ROOT/backlog.yaml"
        BACKLOG_COUNT=$(grep -c 'status: backlog' "$BF" 2>/dev/null || echo 0)
        ACTIVE_COUNT=$(grep -c 'status: in_progress' "$BF" 2>/dev/null || echo 0)
        BLOCKED_COUNT=$(grep -c 'status: blocked' "$BF" 2>/dev/null || echo 0)
        REVIEW_COUNT=$(grep -c 'status: review' "$BF" 2>/dev/null || echo 0)
        DONE_COUNT=$(grep -c 'status: done' "$BF" 2>/dev/null || echo 0)
        echo "$BACKLOG_COUNT $ACTIVE_COUNT $BLOCKED_COUNT $REVIEW_COUNT $DONE_COUNT" > "$CACHE_FILE" 2>/dev/null
    else
        read -r BACKLOG_COUNT ACTIVE_COUNT BLOCKED_COUNT REVIEW_COUNT DONE_COUNT < "$CACHE_FILE" 2>/dev/null
    fi
fi

# --- Alert summary (sanitize counts to pure integers) ---
BLOCKED_COUNT=$(echo "${BLOCKED_COUNT:-0}" | tr -dc '0-9')
REVIEW_COUNT=$(echo "${REVIEW_COUNT:-0}" | tr -dc '0-9')
BACKLOG_COUNT=$(echo "${BACKLOG_COUNT:-0}" | tr -dc '0-9')
ACTIVE_COUNT=$(echo "${ACTIVE_COUNT:-0}" | tr -dc '0-9')
DONE_COUNT=$(echo "${DONE_COUNT:-0}" | tr -dc '0-9')
: "${BLOCKED_COUNT:=0}" "${REVIEW_COUNT:=0}" "${BACKLOG_COUNT:=0}" "${ACTIVE_COUNT:=0}" "${DONE_COUNT:=0}"
ALERT_HIGH=$BLOCKED_COUNT
ALERT_COUNT=$((ALERT_HIGH + REVIEW_COUNT))

ALERT_COLOR="$GREEN"
if [ "${ALERT_HIGH:-0}" -gt 0 ] 2>/dev/null; then
    ALERT_COLOR="$RED"
elif [ "${ALERT_COUNT:-0}" -gt 0 ] 2>/dev/null; then
    ALERT_COLOR="$YELLOW"
fi

# --- Build line 1: Task + Session ---
LINE1=""

if [ -n "$TASK_ID" ]; then
    PRI_CLR=$(pri_color "$TASK_PRIORITY")
    STAT_ICON=$(status_icon "$TASK_STATUS")
    LINE1="${BOLD}${CYAN}${TASK_ID}${RESET}"
    if [ -n "$TASK_TYPE" ]; then
        LINE1="${LINE1} ${MAGENTA}${TASK_TYPE}${RESET}"
    fi
    if [ -n "$TASK_PRIORITY" ]; then
        LINE1="${LINE1} ${PRI_CLR}${TASK_PRIORITY}${RESET}"
    fi
    if [ -n "$STAT_ICON" ]; then
        LINE1="${LINE1} ${STAT_ICON}"
    fi
    LINE1="${LINE1} ${DIM}|${RESET} "
fi

# Repo + branch
if [ -n "$REPO_SHORT" ]; then
    LINE1="${LINE1}${WHITE}${REPO_SHORT}${RESET}"
    if [ -n "$TASK_BRANCH" ]; then
        LINE1="${LINE1}${DIM}@${RESET}${BLUE}${TASK_BRANCH}${RESET}"
    fi
elif [ -n "$TASK_BRANCH" ]; then
    LINE1="${LINE1}${BLUE}${TASK_BRANCH}${RESET}"
else
    LINE1="${LINE1}${DIM}$(basename "$(pwd)")${RESET}"
fi

LINE1="${LINE1} ${DIM}|${RESET} "

# Agent name (if running as agent)
if [ -n "$AGENT_NAME" ]; then
    LINE1="${LINE1}${MAGENTA}${AGENT_NAME}${RESET} "
fi

# Model + context %
LINE1="${LINE1}${DIM}${MODEL_NAME}${RESET} ${CTX_COLOR}${CONTEXT_PCT}%${RESET}"

# Cost + lines changed
LINE1="${LINE1} ${DIM}|${RESET} ${YELLOW}${COST_FMT}${RESET}"
if [ "$LINES_ADDED" != "0" ] || [ "$LINES_REMOVED" != "0" ]; then
    LINE1="${LINE1} ${GREEN}+${LINES_ADDED}${RESET}${DIM}/${RESET}${RED}-${LINES_REMOVED}${RESET}"
fi

# --- Build line 2: Portfolio + Alerts + Time ---
LINE2=""

# Portfolio counts
PARTS=""
if [ "${BACKLOG_COUNT:-0}" -gt 0 ] 2>/dev/null; then
    PARTS="${PARTS}${DIM}backlog:${RESET}${BACKLOG_COUNT}"
fi
if [ "${ACTIVE_COUNT:-0}" -gt 0 ] 2>/dev/null; then
    [ -n "$PARTS" ] && PARTS="${PARTS} "
    PARTS="${PARTS}${GREEN}active:${ACTIVE_COUNT}${RESET}"
fi
if [ "${BLOCKED_COUNT:-0}" -gt 0 ] 2>/dev/null; then
    [ -n "$PARTS" ] && PARTS="${PARTS} "
    PARTS="${PARTS}${RED}blocked:${BLOCKED_COUNT}${RESET}"
fi
if [ "${REVIEW_COUNT:-0}" -gt 0 ] 2>/dev/null; then
    [ -n "$PARTS" ] && PARTS="${PARTS} "
    PARTS="${PARTS}${YELLOW}review:${REVIEW_COUNT}${RESET}"
fi
if [ "${DONE_COUNT:-0}" -gt 0 ] 2>/dev/null; then
    [ -n "$PARTS" ] && PARTS="${PARTS} "
    PARTS="${PARTS}${BLUE}done:${DONE_COUNT}${RESET}"
fi

if [ -n "$PARTS" ]; then
    LINE2="${PARTS} ${DIM}|${RESET} "
fi

# Alert count
if [ "${ALERT_COUNT:-0}" -eq 0 ] 2>/dev/null; then
    LINE2="${LINE2}${GREEN}0 alerts${RESET}"
else
    LINE2="${LINE2}${ALERT_COLOR}${ALERT_COUNT} alert(s)${RESET}"
fi

# Session duration
LINE2="${LINE2} ${DIM}|${RESET} ${DIM}${SESSION_TIME}${RESET}"

# --- Output ---
printf '%b\n' "$LINE1"
printf '%b\n' "$LINE2"
