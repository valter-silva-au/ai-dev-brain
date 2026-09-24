package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type RoleInspectionState string

const (
	RoleInspectionContained   RoleInspectionState = "contained"
	RoleInspectionMissingTail RoleInspectionState = "missing_tail"
)

type RoleInspection struct {
	State           RoleInspectionState
	Target          string
	DeepestExisting string
	FirstMissing    string
}

type RoleViolationReason string

const (
	RoleViolationLexicalEscape RoleViolationReason = "lexical_escape"
	RoleViolationSymlink       RoleViolationReason = "symlink"
	RoleViolationUninspectable RoleViolationReason = "uninspectable"
)

type RoleViolation struct {
	Component string
	Reason    RoleViolationReason
	Err       error
}

func (violation *RoleViolation) Error() string {
	if violation.Err == nil {
		return fmt.Sprintf(
			"workspace role component %q violates physical containment: %s",
			violation.Component,
			violation.Reason,
		)
	}
	return fmt.Sprintf(
		"workspace role component %q violates physical containment (%s): %v",
		violation.Component,
		violation.Reason,
		violation.Err,
	)
}

func (violation *RoleViolation) Unwrap() error {
	return violation.Err
}

// InspectRole physically inspects a relative managed role beneath root.
func InspectRole(root string, role string) (RoleInspection, error) {
	target, err := resolveInspectionRole(root, role)
	if err != nil {
		return RoleInspection{}, err
	}
	return InspectPath(root, target)
}

// InspectRole physically inspects a managed workspace role.
func (layout Layout) InspectRole(role string) (RoleInspection, error) {
	return InspectRole(layout.root, role)
}

// InspectPath physically inspects target from the caller-selected trusted root.
func InspectPath(root string, target string) (RoleInspection, error) {
	if root == "" {
		return RoleInspection{}, errors.New("inspection root is required")
	}
	if !filepath.IsAbs(root) {
		return RoleInspection{}, fmt.Errorf(
			"inspection root must be absolute: %q",
			root,
		)
	}

	cleanRoot := filepath.Clean(root)
	cleanTarget := filepath.Clean(target)
	relative, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil ||
		relative == ".." ||
		filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return RoleInspection{}, &RoleViolation{
			Component: target,
			Reason:    RoleViolationLexicalEscape,
			Err:       err,
		}
	}

	openedRoot, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return RoleInspection{}, &RoleViolation{
			Component: ".",
			Reason:    RoleViolationUninspectable,
			Err:       err,
		}
	}
	defer func() {
		_ = openedRoot.Close()
	}()

	inspection := RoleInspection{
		State:           RoleInspectionContained,
		Target:          cleanTarget,
		DeepestExisting: cleanRoot,
	}
	if relative == "." {
		return inspection, nil
	}

	components := strings.Split(relative, string(filepath.Separator))
	for index := range components {
		component := filepath.Join(components[:index+1]...)
		info, statErr := openedRoot.Lstat(component)
		if errors.Is(statErr, fs.ErrNotExist) {
			inspection.State = RoleInspectionMissingTail
			inspection.FirstMissing = filepath.Join(cleanRoot, component)
			return inspection, nil
		}
		if statErr != nil {
			return RoleInspection{}, &RoleViolation{
				Component: component,
				Reason:    RoleViolationUninspectable,
				Err:       statErr,
			}
		}
		reason, componentErr := inspectRoleComponent(
			info,
			index == len(components)-1,
		)
		if reason != "" {
			return RoleInspection{}, &RoleViolation{
				Component: component,
				Reason:    reason,
				Err:       componentErr,
			}
		}
		inspection.DeepestExisting = filepath.Join(cleanRoot, component)
	}
	return inspection, nil
}

func resolveInspectionRole(root string, role string) (string, error) {
	if role == "" {
		return "", &RoleViolation{
			Component: role,
			Reason:    RoleViolationLexicalEscape,
			Err:       errors.New("role path is required"),
		}
	}
	if filepath.IsAbs(role) || filepath.VolumeName(role) != "" {
		return "", &RoleViolation{
			Component: role,
			Reason:    RoleViolationLexicalEscape,
			Err:       errors.New("role path must be relative"),
		}
	}

	target := filepath.Clean(filepath.Join(root, role))
	relative, err := filepath.Rel(filepath.Clean(root), target)
	if err != nil ||
		relative == ".." ||
		filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", &RoleViolation{
			Component: role,
			Reason:    RoleViolationLexicalEscape,
			Err:       err,
		}
	}
	return target, nil
}
