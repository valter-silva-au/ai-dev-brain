package core

import "testing"

// TestLintMomTest_FlagsBadQuestions checks each violating question is flagged
// with the right rule.
func TestLintMomTest_FlagsBadQuestions(t *testing.T) {
	cases := []struct {
		q    string
		rule MomTestRule
	}{
		{"Would you use this product?", MomHypothetical},
		{"Do you think you'd switch to it?", MomHypothetical},
		{"Do you like this idea?", MomOpinion},
		{"What do you think of my app?", MomOpinion},
		{"Is this a good idea?", MomOpinion},
		{"Don't you hate doing taxes by hand?", MomLeading},
		{"Wouldn't it be great to automate this?", MomLeading},
		{"How much would you pay for this?", MomPricing},
	}
	for _, c := range cases {
		f := LintMomTest(c.q)
		if len(f) != 1 {
			t.Errorf("%q → %d findings, want 1 (%+v)", c.q, len(f), f)
			continue
		}
		if f[0].Rule != c.rule {
			t.Errorf("%q → rule %q, want %q", c.q, f[0].Rule, c.rule)
		}
		if f[0].Hint == "" {
			t.Errorf("%q → finding has no hint", c.q)
		}
	}
}

// TestLintMomTest_PassesGoodQuestions checks past-tense/behavioural questions are
// not flagged, and that non-question prose is ignored even with trigger words.
func TestLintMomTest_PassesGoodQuestions(t *testing.T) {
	good := []string{
		"Tell me about the last time you did your taxes?",
		"How do you currently handle invoicing?",
		"What did you do when that broke?",
		"How much did that cost you last month?",
		"Walk me through the last time this happened?",
	}
	for _, q := range good {
		if f := LintMomTest(q); len(f) != 0 {
			t.Errorf("good question %q was flagged: %+v", q, f)
		}
	}
	// Prose (no '?') is ignored even though it contains "would".
	if f := LintMomTest("The user would use it every day."); len(f) != 0 {
		t.Errorf("non-question prose flagged: %+v", f)
	}
}

// TestLintMomTest_LineNumbers checks multi-line input reports the right line and
// only flags the offending question.
func TestLintMomTest_LineNumbers(t *testing.T) {
	in := "Good: how do you currently cope?\nBad: would you pay for this?"
	f := LintMomTest(in)
	if len(f) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(f), f)
	}
	if f[0].Line != 2 {
		t.Errorf("finding line = %d, want 2", f[0].Line)
	}
	if f[0].Rule != MomPricing {
		t.Errorf("finding rule = %q, want pricing-hypothetical", f[0].Rule)
	}
}
