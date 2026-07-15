package models

import "testing"

func TestStage_IsValid(t *testing.T) {
	cases := []struct {
		stage Stage
		want  bool
	}{
		{StageIdea, true},
		{StageMVP, true},
		{StageLaunch, true},
		{StageScale, true},
		{Stage("idea"), false}, // case-sensitive
		{Stage(""), false},
		{Stage("Growth"), false},
	}
	for _, tc := range cases {
		if got := tc.stage.IsValid(); got != tc.want {
			t.Errorf("Stage(%q).IsValid() = %v, want %v", tc.stage, got, tc.want)
		}
	}
}

func TestValidStages_Order(t *testing.T) {
	want := []Stage{StageIdea, StageMVP, StageLaunch, StageScale}
	if len(ValidStages) != len(want) {
		t.Fatalf("ValidStages has %d entries, want %d", len(ValidStages), len(want))
	}
	for i, s := range want {
		if ValidStages[i] != s {
			t.Errorf("ValidStages[%d] = %q, want %q", i, ValidStages[i], s)
		}
	}
}
