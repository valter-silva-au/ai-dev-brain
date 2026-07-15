#!/usr/bin/env bash
# ADB Shell Prompt Integration
# Source this file from ~/.bashrc to show task context in your prompt.
#
# When inside an ADB worktree:  [PRIS-00022 spike P0 *] user@host:~/Code/work/PRIS-00022$
# When inside ADB_HOME:         [adb 5B/2A/1X] user@host:~/Code$
# When outside ADB context:     (normal prompt, unchanged)
#
# Install: add this line to ~/.bashrc:
#   source /path/to/adb-prompt.sh
# Or run: adb sync claude-user (installs automatically)

__adb_prompt() {
    local adb_ctx
    adb_ctx="$(adb prompt 2>/dev/null)"
    if [ -n "$adb_ctx" ]; then
        PS1="${adb_ctx} ${__ADB_ORIG_PS1}"
    else
        PS1="${__ADB_ORIG_PS1}"
    fi
}

# Only install if not already installed
if [ -z "$__ADB_PROMPT_INSTALLED" ]; then
    __ADB_ORIG_PS1="$PS1"
    PROMPT_COMMAND="__adb_prompt${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
    __ADB_PROMPT_INSTALLED=1
fi
