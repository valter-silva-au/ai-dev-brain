package core

import (
	"regexp"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// unsafeBranchChars matches characters that are not safe in git branch names.
var unsafeBranchChars = regexp.MustCompile(`[^a-zA-Z0-9._/-]`)

// collapseDashes collapses consecutive dashes into a single dash.
var collapseDashes = regexp.MustCompile(`-{2,}`)

// FormatBranchName applies a pattern with {type}, {id}, {description}, {repo},
// and {prefix} placeholders to produce a formatted branch name. If pattern is
// empty, the description is returned as-is for backward compatibility.
//
// {repo} is replaced with the short repo name extracted from the task ID
// (the second-to-last path segment). {prefix} is replaced with the full prefix
// portion of the task ID (everything except the last segment).
func FormatBranchName(pattern string, taskType models.TaskType, taskID string, description string) string {
	if pattern == "" {
		return description
	}

	result := pattern
	result = strings.ReplaceAll(result, "{type}", string(taskType))
	result = strings.ReplaceAll(result, "{id}", taskID)
	result = strings.ReplaceAll(result, "{description}", sanitizeBranchSegment(description))
	result = strings.ReplaceAll(result, "{repo}", RepoFromTaskID(taskID))
	result = strings.ReplaceAll(result, "{prefix}", PrefixFromTaskID(taskID))

	return result
}

// sanitizeBranchSegment replaces spaces and special characters with dashes,
// collapses consecutive dashes, trims leading/trailing dashes, and lowercases
// the result. The output is safe for use as a git branch name segment.
func sanitizeBranchSegment(s string) string {
	s = strings.ToLower(s)
	s = unsafeBranchChars.ReplaceAllString(s, "-")
	s = collapseDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
