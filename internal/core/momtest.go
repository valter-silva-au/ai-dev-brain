package core

import (
	"regexp"
	"strings"
)

// The Mom Test linter flags customer-interview questions that violate The Mom
// Test: questions about the future or hypotheticals, questions seeking an opinion
// on your idea, leading questions, and hypothetical-pricing questions. Good
// questions ask about specific PAST behaviour ("Tell me about the last time…",
// "How do you currently…") and never mention your idea — those produce no
// findings. It is a heuristic over question lines (those containing '?'), so
// prose and headings are ignored.

// MomTestRule categorises a flagged interview question.
type MomTestRule string

const (
	// MomHypothetical flags future/conditional questions (would/could/will you…).
	MomHypothetical MomTestRule = "hypothetical"
	// MomOpinion flags questions asking the interviewee's opinion of the idea.
	MomOpinion MomTestRule = "opinion"
	// MomLeading flags questions that prime a "yes" (don't you…, wouldn't it…).
	MomLeading MomTestRule = "leading"
	// MomPricing flags hypothetical-pricing questions (how much would you pay…).
	MomPricing MomTestRule = "pricing-hypothetical"
)

// MomTestFinding is one flagged interview question.
type MomTestFinding struct {
	Line int         // 1-based line number
	Text string      // the offending question (trimmed)
	Rule MomTestRule // which Mom Test rule it breaks
	Hint string      // how to fix it
}

// momTestPattern pairs a rule with its detector; the first matching rule (in this
// order) wins for a given line, so the more specific pricing rule beats the
// generic hypothetical one.
type momTestPattern struct {
	rule MomTestRule
	re   *regexp.Regexp
	hint string
}

var momTestPatterns = []momTestPattern{
	{
		rule: MomPricing,
		re:   regexp.MustCompile(`(?i)how much (would|will|could|might) you (pay|spend)|would you pay`),
		hint: `Don't ask hypothetical prices. Ask what they pay today for the workaround.`,
	},
	{
		rule: MomOpinion,
		re:   regexp.MustCompile(`(?i)\bdo you (like|love)\b|\bwhat do you think (of|about)\b|\bis (this|that|it) a good idea\b|\bdo you think (this|that|it)('?s| is)\b`),
		hint: `Don't ask for an opinion of your idea. Ask what they actually did.`,
	},
	{
		rule: MomLeading,
		re:   regexp.MustCompile(`(?i)\bdon'?t you\b|\bwouldn'?t (it|you)\b|\bisn'?t it\b|,\s*right\?`),
		hint: `Leading question — it fishes for a yes. Ask a neutral, open past-tense question.`,
	},
	{
		rule: MomHypothetical,
		re:   regexp.MustCompile(`(?i)\b(would|could|will) you\b|\bwould it be\b|\bif you (had|could)\b|\bdo you think you('?d| would)\b`),
		hint: `Hypothetical/future question. Rephrase to the past: "Tell me about the last time…".`,
	},
}

// LintMomTest returns a finding for every interview question (a line containing
// '?') that breaks a Mom Test rule. Lines without '?' are treated as prose and
// ignored. At most one finding per line (the highest-priority matching rule).
func LintMomTest(text string) []MomTestFinding {
	var findings []MomTestFinding
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.Contains(line, "?") {
			continue // not a question
		}
		for _, p := range momTestPatterns {
			if p.re.MatchString(line) {
				findings = append(findings, MomTestFinding{
					Line: i + 1,
					Text: line,
					Rule: p.rule,
					Hint: p.hint,
				})
				break // one finding per line
			}
		}
	}
	return findings
}
