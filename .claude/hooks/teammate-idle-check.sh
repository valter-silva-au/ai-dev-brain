#!/usr/bin/env bash
# Hook: TeammateIdle - No-op. Always exits 0.
# Teammates go idle after EVERY turn, so any work here runs dozens of times
# per team session. Keep this hook empty to avoid unnecessary overhead.
# Use TaskCompleted hooks for quality gates instead.
exit 0
