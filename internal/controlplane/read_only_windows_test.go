//go:build windows

package controlplane

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenReadOnlyUNCAdministrativeShare(t *testing.T) {
	ctx := context.Background()
	drivePath := filepath.Join(t.TempDir(), "state.sqlite")
	want := WorkspaceProjection{
		ID:           "workspace-unc-runtime",
		Root:         "file:///workspace",
		ManifestHash: "manifest-hash",
		ObservedAt: time.Date(
			2026,
			time.September,
			11,
			8,
			0,
			0,
			0,
			time.UTC,
		),
	}

	writer, err := Open(ctx, drivePath)
	if err != nil {
		t.Fatalf("create control plane through drive path %q: %v", drivePath, err)
	}
	if err := writer.ObserveWorkspace(ctx, want); err != nil {
		_ = writer.Close()
		t.Fatalf("seed workspace projection through drive path %q: %v", drivePath, err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close control plane created through drive path %q: %v", drivePath, err)
	}

	uncPath, err := localhostAdministrativeShareUNC(drivePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(uncPath); err != nil {
		t.Fatalf(
			"UNC runtime validation requires an accessible localhost administrative share: drive path %q maps to %q: %v",
			drivePath,
			uncPath,
			err,
		)
	}

	reader, err := OpenReadOnly(ctx, uncPath)
	if err != nil {
		t.Fatalf(
			"open control plane through localhost administrative-share UNC path %q: %v",
			uncPath,
			err,
		)
	}
	t.Cleanup(func() {
		_ = reader.Close()
	})

	check, err := reader.Check(ctx)
	if err != nil {
		t.Fatalf("check control plane through UNC path %q: %v", uncPath, err)
	}
	if check.Integrity != "ok" {
		t.Errorf("integrity through UNC path = %q, want ok", check.Integrity)
	}
	if check.SchemaVersion != CurrentSchemaVersion {
		t.Errorf(
			"schema version through UNC path = %d, want %d",
			check.SchemaVersion,
			CurrentSchemaVersion,
		)
	}

	got, err := reader.Workspace(ctx)
	if err != nil {
		t.Fatalf("read workspace projection through UNC path %q: %v", uncPath, err)
	}
	if got != want {
		t.Errorf("workspace projection through UNC path = %#v, want %#v", got, want)
	}
}

func localhostAdministrativeShareUNC(path string) (string, error) {
	cleaned := filepath.Clean(path)
	volume := filepath.VolumeName(cleaned)
	if len(volume) != 2 || volume[1] != ':' {
		return "", fmt.Errorf(
			"UNC runtime validation requires a drive-qualified path, got %q with volume %q",
			path,
			volume,
		)
	}

	tail := strings.TrimLeft(strings.TrimPrefix(cleaned, volume), `\/`)
	return `\\localhost\` +
		strings.ToLower(volume[:1]) +
		`$\` +
		tail, nil
}
