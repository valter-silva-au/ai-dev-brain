package ticket

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

func TestPropertySlugifyAlwaysProducesEmptyOrValidPortableSlug(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		title := rapid.String().Draw(t, "title")
		slug := Slugify(title)
		if slug == "" {
			return
		}
		if err := ValidateSlug(slug); err != nil {
			t.Fatalf("Slugify(%q) produced invalid slug %q: %v", title, slug, err)
		}
	})
}

func TestPropertyAllocatedKeyIsGreaterThanEveryMatchingPortableSelector(
	t *testing.T,
) {
	rapid.Check(t, func(t *rapid.T) {
		numbers := rapid.SliceOfN(
			rapid.IntRange(1, 999_999),
			0,
			40,
		).Draw(t, "numbers")
		identities := make([]Identity, 0, len(numbers))
		highest := 0
		for index, number := range numbers {
			if number > highest {
				highest = number
			}
			identity := Identity{
				LocalKey: fmt.Sprintf("TASK-%05d", number),
			}
			if index%2 == 0 {
				identity.LocalKey = fmt.Sprintf("OTHER-%d", index+1)
				identity.Aliases = []string{
					fmt.Sprintf("TASK-%05d", number),
				}
			}
			identities = append(identities, identity)
		}

		got, err := AllocateLocalKey(DefaultKeyPolicy(), identities)
		if err != nil {
			t.Fatalf("allocate local key: %v", err)
		}
		want := fmt.Sprintf("TASK-%05d", highest+1)
		if got != want {
			t.Fatalf("allocated key = %q, want %q", got, want)
		}
	})
}
