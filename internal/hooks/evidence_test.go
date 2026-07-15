package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceTracker_RecordAndReads(t *testing.T) {
	tmp := t.TempDir()
	et := NewEvidenceTracker(tmp)

	if err := et.Record("screenshots/one.png"); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := et.Record("logs/build-result.txt"); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	reads, err := et.Reads()
	if err != nil {
		t.Fatalf("Reads() error = %v", err)
	}
	if len(reads) != 2 {
		t.Fatalf("Reads() returned %d entries, want 2", len(reads))
	}
	if reads[0] != "screenshots/one.png" {
		t.Errorf("Reads()[0] = %q, want screenshots/one.png", reads[0])
	}
	if reads[1] != "logs/build-result.txt" {
		t.Errorf("Reads()[1] = %q, want logs/build-result.txt", reads[1])
	}
}

func TestEvidenceTracker_ReadsOnEmpty(t *testing.T) {
	tmp := t.TempDir()
	et := NewEvidenceTracker(tmp)

	reads, err := et.Reads()
	if err != nil {
		t.Fatalf("Reads() on missing file should not error, got %v", err)
	}
	if len(reads) != 0 {
		t.Errorf("Reads() on missing file returned %d entries, want 0", len(reads))
	}
}

func TestEvidenceTracker_Clear(t *testing.T) {
	tmp := t.TempDir()
	et := NewEvidenceTracker(tmp)

	if err := et.Record("foo.png"); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, ".adb", "evidence_reads")); err != nil {
		t.Fatalf("evidence file should exist after Record, got %v", err)
	}

	if err := et.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".adb", "evidence_reads")); !os.IsNotExist(err) {
		t.Errorf("evidence file should be gone after Clear, got %v", err)
	}

	// Clear on missing file is safe.
	if err := et.Clear(); err != nil {
		t.Errorf("Clear() on missing file should be safe, got %v", err)
	}
}
