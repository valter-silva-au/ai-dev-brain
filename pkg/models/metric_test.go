package models

import "testing"

func TestMetric_GraphID(t *testing.T) {
	m := Metric{Initiative: "onboarding", Name: "sean-ellis"}
	if got := m.GraphID(); got != "metric:onboarding:sean-ellis" {
		t.Fatalf("GraphID() = %q", got)
	}
}

func TestMetric_Validate(t *testing.T) {
	if err := (Metric{Initiative: "i", Name: "sean-ellis", Value: 42}).Validate(); err != nil {
		t.Fatalf("valid metric rejected: %v", err)
	}
	// Zero value is legitimate.
	if err := (Metric{Initiative: "i", Name: "retention", Value: 0}).Validate(); err != nil {
		t.Fatalf("zero-value metric rejected: %v", err)
	}
	for _, m := range []Metric{
		{Name: "sean-ellis"}, // no initiative
		{Initiative: "i"},    // no name
	} {
		if err := m.Validate(); err == nil {
			t.Fatalf("expected validation error for %+v", m)
		}
	}
}
