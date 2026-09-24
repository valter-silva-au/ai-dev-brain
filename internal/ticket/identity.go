package ticket

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const MaxSlugLength = 80

var (
	localKeyPattern = regexp.MustCompile(
		`^[A-Z][A-Z0-9]{0,15}-[0-9]{1,9}$`,
	)
	remoteVisibleKeyPattern = regexp.MustCompile(
		`^[a-z][a-z0-9.-]*:[^[:space:]]+$`,
	)
	keyPrefixPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,15}$`)
	slugPattern      = regexp.MustCompile(
		`^[a-z0-9](?:[a-z0-9-]{0,78}[a-z0-9])?$`,
	)
	slugReplaceRunes = regexp.MustCompile(`[^a-z0-9-]+`)
	slugDashRun      = regexp.MustCompile(`-+`)
)

type Identity struct {
	LocalKey string
	Aliases  []string
}

type KeyPolicy struct {
	Prefix string
	Width  int
}

func DefaultKeyPolicy() KeyPolicy {
	return KeyPolicy{Prefix: "TASK", Width: 5}
}

func ValidateLocalKey(value string) error {
	if !localKeyPattern.MatchString(value) {
		return fmt.Errorf(
			"ticket local key %q must be PREFIX-number using uppercase ASCII",
			value,
		)
	}
	return nil
}

func ValidateVisibleKey(value string) error {
	switch {
	case value == "":
		return errors.New("ticket visible key is required")
	case len(value) > 256:
		return errors.New("ticket visible key exceeds 256 characters")
	case !norm.NFC.IsNormalString(value):
		return errors.New("ticket visible key must use NFC normalization")
	case containsControl(value):
		return errors.New("ticket visible key contains a control character")
	case localKeyPattern.MatchString(value):
		return nil
	case remoteVisibleKeyPattern.MatchString(value):
		return nil
	default:
		return fmt.Errorf("ticket visible key %q is not a canonical selector", value)
	}
}

func ValidateSlug(value string) error {
	switch {
	case value == "":
		return errors.New("ticket slug is required")
	case len(value) > MaxSlugLength:
		return fmt.Errorf(
			"ticket slug exceeds %d characters",
			MaxSlugLength,
		)
	case !slugPattern.MatchString(value):
		return fmt.Errorf(
			"ticket slug %q must use lowercase ASCII letters, digits, and interior dashes",
			value,
		)
	}
	return nil
}

func Slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugReplaceRunes.ReplaceAllString(value, "-")
	value = slugDashRun.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if len(value) <= MaxSlugLength {
		return value
	}
	value = strings.Trim(value[:MaxSlugLength], "-")
	return value
}

func BranchIntent(
	workType string,
	localKey string,
	branchSlug string,
	activeProfile profile.Profile,
) (string, error) {
	if err := validateResolvedProfile(activeProfile); err != nil {
		return "", err
	}
	if err := ValidateLocalKey(localKey); err != nil {
		return "", err
	}
	if err := ValidateSlug(branchSlug); err != nil {
		return "", err
	}
	if !validWorkType(workType) {
		return "", fmt.Errorf("ticket work type %q is invalid", workType)
	}

	declared := containsString(activeProfile.WorkTypes, workType)
	if workType != "spike" && !declared {
		return "", fmt.Errorf(
			"ticket work type %q is not declared by profile %s@%s",
			workType,
			activeProfile.ID,
			activeProfile.Version,
		)
	}
	branchType := workType
	if workType == "spike" && !declared {
		branchType = "chore"
	}
	return branchType + "/" + localKey + "-" + branchSlug, nil
}

func AllocateLocalKey(
	policy KeyPolicy,
	identities []Identity,
) (string, error) {
	if err := validateKeyPolicy(policy); err != nil {
		return "", err
	}
	prefix := policy.Prefix + "-"
	var highest uint64
	for _, identity := range identities {
		if err := ValidateLocalKey(identity.LocalKey); err != nil {
			return "", fmt.Errorf(
				"validate existing ticket local key: %w",
				err,
			)
		}
		selectors := append(
			[]string{identity.LocalKey},
			identity.Aliases...,
		)
		for _, selector := range selectors {
			if !strings.HasPrefix(selector, prefix) {
				continue
			}
			if ValidateLocalKey(selector) != nil {
				continue
			}
			suffix := strings.TrimPrefix(selector, prefix)
			if suffix == "" || !allDecimalDigits(suffix) {
				continue
			}
			number, err := strconv.ParseUint(suffix, 10, 32)
			if err != nil {
				return "", fmt.Errorf(
					"ticket key %q exceeds the supported numeric range",
					selector,
				)
			}
			if number > highest {
				highest = number
			}
		}
	}
	if highest >= 999_999_999 {
		return "", errors.New("ticket local key range is exhausted")
	}
	next := highest + 1
	key := fmt.Sprintf(
		"%s-%0*d",
		policy.Prefix,
		policy.Width,
		next,
	)
	if err := ValidateLocalKey(key); err != nil {
		return "", fmt.Errorf("allocate ticket local key: %w", err)
	}
	return key, nil
}

func validateKeyPolicy(policy KeyPolicy) error {
	switch {
	case !keyPrefixPattern.MatchString(policy.Prefix):
		return fmt.Errorf(
			"ticket key prefix %q must use 1-16 uppercase ASCII letters or digits",
			policy.Prefix,
		)
	case policy.Width < 1 || policy.Width > 9:
		return errors.New("ticket key width must be between 1 and 9")
	}
	return nil
}

func validateResolvedProfile(value profile.Profile) error {
	if err := profile.ValidateProfile(value); err != nil {
		return fmt.Errorf("validate active ticket profile: %w", err)
	}
	if len(value.Inherits) != 0 {
		return errors.New("active ticket profile must have resolved inheritance")
	}
	return nil
}

func validWorkType(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if character == '-' ||
			(character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func foldSelector(value string) string {
	return cases.Fold().String(norm.NFC.String(value))
}

func allDecimalDigits(value string) bool {
	for _, character := range value {
		if !unicode.IsDigit(character) ||
			character < '0' ||
			character > '9' {
			return false
		}
	}
	return true
}
