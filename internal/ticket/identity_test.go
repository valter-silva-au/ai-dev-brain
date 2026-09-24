package ticket

import (
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
)

func TestSlugifyKeepsTitleIndependentAndPortable(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		" Design AI Dev Brain v3 ": "design-ai-dev-brain-v3",
		"API: retries / recovery":  "api-retries-recovery",
		"Crème brûlée":             "cr-me-br-l-e",
		"---":                      "",
	}
	for title, want := range tests {
		if got := Slugify(title); got != want {
			t.Fatalf("Slugify(%q) = %q, want %q", title, got, want)
		}
	}

	long := Slugify(strings.Repeat("word ", 40))
	if len(long) > MaxSlugLength {
		t.Fatalf("slug length = %d, want <= %d", len(long), MaxSlugLength)
	}
}

func TestBranchIntentUsesTicketKeyAndProfileAwareSpikeMapping(t *testing.T) {
	t.Parallel()

	builtin, err := profile.BuiltinTicketProfile()
	if err != nil {
		t.Fatalf("load built-in profile: %v", err)
	}

	branch, err := BranchIntent(
		"feat",
		"TASK-00039",
		"design-ai-dev-brain-v3",
		builtin,
	)
	if err != nil {
		t.Fatalf("build branch intent: %v", err)
	}
	if branch != "feat/TASK-00039-design-ai-dev-brain-v3" {
		t.Fatalf("branch intent = %q", branch)
	}

	branch, err = BranchIntent(
		"spike",
		"TASK-00039",
		"design-ai-dev-brain-v3",
		builtin,
	)
	if err != nil {
		t.Fatalf("build default spike branch intent: %v", err)
	}
	if branch != "chore/TASK-00039-design-ai-dev-brain-v3" {
		t.Fatalf("default spike branch intent = %q", branch)
	}

	custom := builtin
	custom.ID = "research"
	custom.Version = "v1"
	custom.WorkTypes = append(custom.WorkTypes, "spike")
	branch, err = BranchIntent(
		"spike",
		"TASK-00039",
		"design-ai-dev-brain-v3",
		custom,
	)
	if err != nil {
		t.Fatalf("build profile spike branch intent: %v", err)
	}
	if branch != "spike/TASK-00039-design-ai-dev-brain-v3" {
		t.Fatalf("profile spike branch intent = %q", branch)
	}
}

func TestBranchIntentRejectsUndeclaredCustomWorkType(t *testing.T) {
	t.Parallel()

	builtin, err := profile.BuiltinTicketProfile()
	if err != nil {
		t.Fatalf("load built-in profile: %v", err)
	}
	if _, err := BranchIntent(
		"prototype",
		"TASK-00039",
		"design-ai-dev-brain-v3",
		builtin,
	); err == nil {
		t.Fatal("undeclared custom work type was accepted")
	}
}

func TestBranchIntentSupportsEveryBuiltInWorkType(t *testing.T) {
	t.Parallel()

	builtin, err := profile.BuiltinTicketProfile()
	if err != nil {
		t.Fatalf("load built-in profile: %v", err)
	}
	for _, workType := range builtin.WorkTypes {
		branch, branchErr := BranchIntent(
			workType,
			"TASK-00039",
			"portable-branch",
			builtin,
		)
		if branchErr != nil {
			t.Fatalf("build %q branch intent: %v", workType, branchErr)
		}
		want := workType + "/TASK-00039-portable-branch"
		if branch != want {
			t.Fatalf("%q branch = %q, want %q", workType, branch, want)
		}
	}
}

func TestVisibleKeyAcceptsCanonicalRemoteSelectorButLocalKeyDoesNot(t *testing.T) {
	t.Parallel()

	remote := "github:valter-silva-au/ai-dev-brain#39"
	if err := ValidateVisibleKey(remote); err != nil {
		t.Fatalf("canonical remote visible key rejected: %v", err)
	}
	if err := ValidateLocalKey(remote); err == nil {
		t.Fatal("remote selector was accepted as a path-forming local key")
	}
}

func TestAllocateLocalKeyUsesHighestPortableIdentityWithoutReusingAliases(
	t *testing.T,
) {
	t.Parallel()

	got, err := AllocateLocalKey(
		KeyPolicy{Prefix: "TASK", Width: 5},
		[]Identity{
			{LocalKey: "TASK-00002"},
			{
				LocalKey: "OTHER-9",
				Aliases:  []string{"TASK-00007"},
			},
			{LocalKey: "TASK-00004"},
		},
	)
	if err != nil {
		t.Fatalf("allocate local key: %v", err)
	}
	if got != "TASK-00008" {
		t.Fatalf("allocated key = %q, want TASK-00008", got)
	}
}

func TestAllocateLocalKeyIgnoresKeyShapedAliasesOutsideLocalKeyBounds(
	t *testing.T,
) {
	t.Parallel()

	got, err := AllocateLocalKey(
		DefaultKeyPolicy(),
		[]Identity{{
			LocalKey: "TASK-00001",
			Aliases: []string{
				"TASK-4294967296",
				"TASK-not-a-local-key",
			},
		}},
	)
	if err != nil {
		t.Fatalf("allocate local key: %v", err)
	}
	if got != "TASK-00002" {
		t.Fatalf("allocated key = %q, want TASK-00002", got)
	}
}
