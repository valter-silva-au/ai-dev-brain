//go:build windows

package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestInspectRoleRejectsFinalWindowsJunction(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	createWindowsJunction(
		t,
		filepath.Join(root, "organizations"),
		external,
	)

	layout, err := NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	_, err = layout.InspectRole("organizations")
	assertRoleViolation(t, err, RoleViolationSymlink, "organizations")
}

func TestInspectRoleRejectsIntermediateWindowsJunction(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	if err := os.Mkdir(filepath.Join(external, "organizations"), 0o755); err != nil {
		t.Fatalf("create external role directory: %v", err)
	}
	createWindowsJunction(
		t,
		filepath.Join(root, "managed"),
		external,
	)

	layout, err := NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	_, err = layout.InspectRole(filepath.Join("managed", "organizations"))
	assertRoleViolation(t, err, RoleViolationSymlink, "managed")
}

func createWindowsJunction(t *testing.T, link string, target string) {
	t.Helper()

	output, err := exec.Command(
		"cmd.exe",
		"/c",
		"mklink",
		"/J",
		link,
		target,
	).CombinedOutput()
	if err != nil {
		t.Fatalf(
			"create Windows junction %q -> %q: %v\n%s",
			link,
			target,
			err,
			output,
		)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat Windows junction %q: %v", link, err)
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		t.Fatalf("junction stat data type = %T, want *syscall.Win32FileAttributeData", info.Sys())
	}
	if data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		t.Fatalf("mklink setup did not create a reparse point: attributes=%#x", data.FileAttributes)
	}
}
