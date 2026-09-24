//go:build !windows

package workspace

import (
	"io/fs"
	"os"
	"testing"
	"time"
)

func TestInspectRoleComponentRejectsFinalModeIrregular(t *testing.T) {
	t.Parallel()

	reason, err := inspectRoleComponent(irregularFileInfo{}, true)
	if reason != RoleViolationUninspectable {
		t.Fatalf("reason = %q, want %q", reason, RoleViolationUninspectable)
	}
	if err == nil {
		t.Fatal("expected final ModeIrregular component to fail")
	}
}

type irregularFileInfo struct{}

func (irregularFileInfo) Name() string       { return "irregular" }
func (irregularFileInfo) Size() int64        { return 0 }
func (irregularFileInfo) Mode() fs.FileMode  { return os.ModeIrregular }
func (irregularFileInfo) ModTime() time.Time { return time.Time{} }
func (irregularFileInfo) IsDir() bool        { return false }
func (irregularFileInfo) Sys() any           { return nil }
