package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

func TestNewFileChannelAdapter(t *testing.T) {
	t.Run("creates inbox and outbox directories", func(t *testing.T) {
		dir := t.TempDir()
		adapter, err := NewFileChannelAdapter(FileChannelConfig{
			Name:    "test",
			BaseDir: dir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Name() != "test" {
			t.Errorf("got name %q, want %q", adapter.Name(), "test")
		}
		if adapter.Type() != models.ChannelFile {
			t.Errorf("got type %q, want %q", adapter.Type(), models.ChannelFile)
		}

		for _, sub := range []string{"inbox", "outbox"} {
			info, err := os.Stat(filepath.Join(dir, sub))
			if err != nil {
				t.Errorf("expected %s directory to exist: %v", sub, err)
			} else if !info.IsDir() {
				t.Errorf("expected %s to be a directory", sub)
			}
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		_, err := NewFileChannelAdapter(FileChannelConfig{
			Name:    "",
			BaseDir: t.TempDir(),
		})
		if err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("rejects empty base dir", func(t *testing.T) {
		_, err := NewFileChannelAdapter(FileChannelConfig{
			Name:    "test",
			BaseDir: "",
		})
		if err == nil {
			t.Fatal("expected error for empty base dir")
		}
	})
}

func TestFileChannelAdapter_Fetch(t *testing.T) {
	t.Run("returns pending items only", func(t *testing.T) {
		dir := t.TempDir()
		adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

		// Write a pending item.
		writeMD(t, filepath.Join(dir, "inbox", "msg-001.md"), "msg-001", "pending", "alice", "Hello", "Body text")
		// Write a processed item (should be excluded).
		writeMD(t, filepath.Join(dir, "inbox", "msg-002.md"), "msg-002", "processed", "bob", "Old", "Old body")

		items, err := adapter.Fetch()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
		if items[0].ID != "msg-001" {
			t.Errorf("got ID %q, want %q", items[0].ID, "msg-001")
		}
		if items[0].From != "alice" {
			t.Errorf("got From %q, want %q", items[0].From, "alice")
		}
		if items[0].Subject != "Hello" {
			t.Errorf("got Subject %q, want %q", items[0].Subject, "Hello")
		}
		if items[0].Content != "Body text" {
			t.Errorf("got Content %q, want %q", items[0].Content, "Body text")
		}
		if items[0].Channel != models.ChannelFile {
			t.Errorf("got Channel %q, want %q", items[0].Channel, models.ChannelFile)
		}
	})

	t.Run("skips malformed files", func(t *testing.T) {
		dir := t.TempDir()
		adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

		// Write a file without frontmatter.
		if err := os.WriteFile(filepath.Join(dir, "inbox", "bad.md"), []byte("no frontmatter here"), 0o644); err != nil {
			t.Fatal(err)
		}

		items, err := adapter.Fetch()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 0 {
			t.Errorf("got %d items, want 0", len(items))
		}
	})

	t.Run("ignores non-markdown files", func(t *testing.T) {
		dir := t.TempDir()
		adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

		if err := os.WriteFile(filepath.Join(dir, "inbox", "notes.txt"), []byte("text file"), 0o644); err != nil {
			t.Fatal(err)
		}

		items, err := adapter.Fetch()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 0 {
			t.Errorf("got %d items, want 0", len(items))
		}
	})

	t.Run("uses filename as ID when frontmatter ID is empty", func(t *testing.T) {
		dir := t.TempDir()
		adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

		content := "---\nsubject: Test\ndate: 2025-01-01\nstatus: pending\n---\n\nBody"
		if err := os.WriteFile(filepath.Join(dir, "inbox", "fallback-id.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		items, err := adapter.Fetch()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
		if items[0].ID != "fallback-id" {
			t.Errorf("got ID %q, want %q", items[0].ID, "fallback-id")
		}
	})
}

func TestFileChannelAdapter_Send(t *testing.T) {
	t.Run("writes output file to outbox", func(t *testing.T) {
		dir := t.TempDir()
		adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

		item := models.OutputItem{
			ID:          "out-001",
			Channel:     models.ChannelFile,
			Destination: "bob@example.com",
			Subject:     "Reply",
			Content:     "Thanks for the update.",
			InReplyTo:   "msg-001",
			SourceTask:  "TASK-00042",
		}

		err := adapter.Send(item)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		outPath := filepath.Join(dir, "outbox", "out-001.md")
		data, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("expected outbox file: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "subject: Reply") {
			t.Error("expected subject in frontmatter")
		}
		if !strings.Contains(content, "to: bob@example.com") {
			t.Error("expected destination in frontmatter")
		}
		if !strings.Contains(content, "Thanks for the update.") {
			t.Error("expected body content")
		}
		if !strings.Contains(content, "in_reply_to: msg-001") {
			t.Error("expected in_reply_to in metadata")
		}
		if !strings.Contains(content, "source_task: TASK-00042") {
			t.Error("expected source_task in metadata")
		}
	})

	t.Run("rejects empty ID", func(t *testing.T) {
		dir := t.TempDir()
		adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

		err := adapter.Send(models.OutputItem{ID: ""})
		if err == nil {
			t.Fatal("expected error for empty ID")
		}
	})
}

func TestFileChannelAdapter_MarkProcessed(t *testing.T) {
	t.Run("updates status to processed", func(t *testing.T) {
		dir := t.TempDir()
		adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

		writeMD(t, filepath.Join(dir, "inbox", "msg-010.md"), "msg-010", "pending", "alice", "Test", "Body")

		err := adapter.MarkProcessed("msg-010")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the file now has status: processed.
		data, err := os.ReadFile(filepath.Join(dir, "inbox", "msg-010.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "status: processed") {
			t.Error("expected status to be updated to processed")
		}
	})

	t.Run("finds item by frontmatter ID", func(t *testing.T) {
		dir := t.TempDir()
		adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

		// File name differs from frontmatter ID.
		writeMD(t, filepath.Join(dir, "inbox", "some-file.md"), "custom-id", "pending", "bob", "Test", "Body")

		err := adapter.MarkProcessed("custom-id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns error for unknown item", func(t *testing.T) {
		dir := t.TempDir()
		adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

		err := adapter.MarkProcessed("nonexistent")
		if err == nil {
			t.Fatal("expected error for unknown item")
		}
	})
}

func TestParseFrontmatter(t *testing.T) {
	t.Run("parses valid frontmatter", func(t *testing.T) {
		content := "---\nid: test-1\nsubject: Hello\ndate: 2025-01-01\nstatus: pending\n---\n\nBody text"
		fm, body, err := parseFrontmatter(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fm.ID != "test-1" {
			t.Errorf("got ID %q, want %q", fm.ID, "test-1")
		}
		if fm.Subject != "Hello" {
			t.Errorf("got Subject %q, want %q", fm.Subject, "Hello")
		}
		if body != "Body text" {
			t.Errorf("got body %q, want %q", body, "Body text")
		}
	})

	t.Run("returns error without opening delimiter", func(t *testing.T) {
		_, _, err := parseFrontmatter("no frontmatter")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("returns error without closing delimiter", func(t *testing.T) {
		_, _, err := parseFrontmatter("---\nid: x\n")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestPriorityParsing(t *testing.T) {
	tests := []struct {
		input string
		want  models.ChannelItemPriority
	}{
		{"high", models.ChannelPriorityHigh},
		{"low", models.ChannelPriorityLow},
		{"medium", models.ChannelPriorityMedium},
		{"", models.ChannelPriorityMedium},
		{"unknown", models.ChannelPriorityMedium},
	}

	for _, tt := range tests {
		t.Run("priority_"+tt.input, func(t *testing.T) {
			dir := t.TempDir()
			adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

			content := "---\nid: p-test\nsubject: Test\ndate: 2025-01-01\nstatus: pending\npriority: " + tt.input + "\n---\n\nBody"
			if err := os.WriteFile(filepath.Join(dir, "inbox", "p-test.md"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}

			items, err := adapter.Fetch()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != 1 {
				t.Fatalf("got %d items, want 1", len(items))
			}
			if items[0].Priority != tt.want {
				t.Errorf("got priority %q, want %q", items[0].Priority, tt.want)
			}
		})
	}
}

// --- Send with metadata ---

func TestFileChannelAdapter_SendWithInReplyTo(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	item := models.OutputItem{
		ID:        "reply-001",
		Subject:   "Re: Original",
		Content:   "Reply body",
		InReplyTo: "original-msg-001",
	}
	if err := adapter.Send(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "outbox", "reply-001.md"))
	if err != nil {
		t.Fatalf("expected outbox file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "in_reply_to: original-msg-001") {
		t.Error("expected in_reply_to metadata in file")
	}
}

func TestFileChannelAdapter_SendWithSourceTask(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	item := models.OutputItem{
		ID:         "task-ref-001",
		Subject:    "Task Update",
		Content:    "Progress report",
		SourceTask: "TASK-00099",
	}
	if err := adapter.Send(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "outbox", "task-ref-001.md"))
	if err != nil {
		t.Fatalf("expected outbox file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "source_task: TASK-00099") {
		t.Error("expected source_task metadata in file")
	}
}

func TestFileChannelAdapter_SendWithCustomMetadata(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	item := models.OutputItem{
		ID:      "meta-001",
		Subject: "With Metadata",
		Content: "Custom metadata body",
		Metadata: map[string]string{
			"custom_key":  "custom_value",
			"another_key": "another_value",
		},
	}
	if err := adapter.Send(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "outbox", "meta-001.md"))
	if err != nil {
		t.Fatalf("expected outbox file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "custom_key: custom_value") {
		t.Error("expected custom_key in metadata")
	}
	if !strings.Contains(content, "another_key: another_value") {
		t.Error("expected another_key in metadata")
	}
}

func TestFileChannelAdapter_SendMergesAllMetadata(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	item := models.OutputItem{
		ID:         "merged-001",
		Subject:    "Merged Metadata",
		Content:    "All metadata sources",
		InReplyTo:  "orig-msg",
		SourceTask: "TASK-00001",
		Metadata: map[string]string{
			"extra": "data",
		},
	}
	if err := adapter.Send(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "outbox", "merged-001.md"))
	if err != nil {
		t.Fatalf("expected outbox file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "in_reply_to: orig-msg") {
		t.Error("expected in_reply_to in merged metadata")
	}
	if !strings.Contains(content, "source_task: TASK-00001") {
		t.Error("expected source_task in merged metadata")
	}
	if !strings.Contains(content, "extra: data") {
		t.Error("expected custom key in merged metadata")
	}
}

func TestFileChannelAdapter_SendWithNoMetadata(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	item := models.OutputItem{
		ID:      "plain-001",
		Subject: "Plain Message",
		Content: "No metadata at all",
	}
	if err := adapter.Send(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "outbox", "plain-001.md"))
	if err != nil {
		t.Fatalf("expected outbox file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "subject: Plain Message") {
		t.Error("expected subject in frontmatter")
	}
	if !strings.Contains(content, "No metadata at all") {
		t.Error("expected body content")
	}
	// Should not have a metadata block.
	if strings.Contains(content, "metadata:") {
		t.Error("expected no metadata block when none is set")
	}
}

// --- MarkProcessed edge cases ---

func TestFileChannelAdapter_MarkProcessedNonexistent(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	err := adapter.MarkProcessed("does-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent item")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestFileChannelAdapter_MarkProcessedVerifyFileContent(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	writeMD(t, filepath.Join(dir, "inbox", "proc-test.md"), "proc-test", "pending", "alice", "Subject", "Original body")

	if err := adapter.MarkProcessed("proc-test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "inbox", "proc-test.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "status: processed") {
		t.Error("expected status: processed")
	}
	// Verify body is preserved.
	if !strings.Contains(content, "Original body") {
		t.Error("expected original body to be preserved")
	}
	// Verify other frontmatter fields are preserved.
	if !strings.Contains(content, "id: proc-test") {
		t.Error("expected id to be preserved")
	}
	if !strings.Contains(content, "from: alice") {
		t.Error("expected from to be preserved")
	}
}

func TestFileChannelAdapter_MarkProcessedIdempotent(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	writeMD(t, filepath.Join(dir, "inbox", "idem-test.md"), "idem-test", "pending", "alice", "Test", "Body")

	// Mark processed twice.
	if err := adapter.MarkProcessed("idem-test"); err != nil {
		t.Fatalf("first mark processed error: %v", err)
	}
	if err := adapter.MarkProcessed("idem-test"); err != nil {
		t.Fatalf("second mark processed error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "inbox", "idem-test.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "status: processed") {
		t.Error("expected status to remain processed")
	}
}

// --- Constructor edge cases ---

func TestNewFileChannelAdapter_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "nested", "channel")

	adapter, err := NewFileChannelAdapter(FileChannelConfig{
		Name:    "nested-test",
		BaseDir: baseDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if adapter.Name() != "nested-test" {
		t.Errorf("got name %q, want %q", adapter.Name(), "nested-test")
	}

	// Verify both inbox and outbox were created in nested path.
	for _, sub := range []string{"inbox", "outbox"} {
		info, err := os.Stat(filepath.Join(baseDir, sub))
		if err != nil {
			t.Errorf("expected %s directory to exist: %v", sub, err)
		} else if !info.IsDir() {
			t.Errorf("expected %s to be a directory", sub)
		}
	}
}

func TestNewFileChannelAdapter_ErrorMessages(t *testing.T) {
	tests := []struct {
		name    string
		cfg     FileChannelConfig
		wantErr string
	}{
		{
			name:    "empty name error message",
			cfg:     FileChannelConfig{Name: "", BaseDir: "/tmp"},
			wantErr: "name is empty",
		},
		{
			name:    "empty base dir error message",
			cfg:     FileChannelConfig{Name: "test", BaseDir: ""},
			wantErr: "base dir is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFileChannelAdapter(tt.cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestNewFileChannelAdapter_ChannelType(t *testing.T) {
	dir := t.TempDir()
	adapter, err := NewFileChannelAdapter(FileChannelConfig{
		Name:    "type-test",
		BaseDir: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if adapter.Type() != models.ChannelFile {
		t.Errorf("got type %q, want %q", adapter.Type(), models.ChannelFile)
	}
}

// --- Status parsing ---

func TestStatusParsing(t *testing.T) {
	tests := []struct {
		input string
		want  models.ChannelItemStatus
	}{
		{"pending", models.ChannelStatusPending},
		{"processed", models.ChannelStatusProcessed},
		{"actionable", models.ChannelStatusActionable},
		{"archived", models.ChannelStatusArchived},
		{"", models.ChannelStatusPending},
		{"unknown", models.ChannelStatusPending},
	}

	for _, tt := range tests {
		t.Run("status_"+tt.input, func(t *testing.T) {
			dir := t.TempDir()
			adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

			content := "---\nid: s-test\nsubject: Test\ndate: 2025-01-01\nstatus: " + tt.input + "\n---\n\nBody"
			if err := os.WriteFile(filepath.Join(dir, "inbox", "s-test.md"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}

			// Use parseInboxFile directly to check the parsed status
			// without the pending-only filter that Fetch applies.
			item, err := adapter.parseInboxFile(filepath.Join(dir, "inbox", "s-test.md"))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if item.Status != tt.want {
				t.Errorf("got status %q, want %q", item.Status, tt.want)
			}
		})
	}
}

// --- Fetch excludes non-pending statuses ---

func TestFileChannelAdapter_FetchExcludesActionableAndArchived(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	writeMD(t, filepath.Join(dir, "inbox", "pending.md"), "pending-1", "pending", "alice", "Pending", "Body")
	writeMD(t, filepath.Join(dir, "inbox", "actionable.md"), "act-1", "actionable", "bob", "Actionable", "Body")
	writeMD(t, filepath.Join(dir, "inbox", "archived.md"), "arch-1", "archived", "carol", "Archived", "Body")

	items, err := adapter.Fetch()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (only pending)", len(items))
	}
	if items[0].ID != "pending-1" {
		t.Errorf("got ID %q, want %q", items[0].ID, "pending-1")
	}
}

// --- Fetch ignores directories in inbox ---

func TestFileChannelAdapter_FetchIgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	// Create a subdirectory in inbox.
	if err := os.MkdirAll(filepath.Join(dir, "inbox", "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a valid pending file.
	writeMD(t, filepath.Join(dir, "inbox", "valid.md"), "valid-1", "pending", "alice", "Valid", "Body")

	items, err := adapter.Fetch()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
}

// --- parseFrontmatter with end-of-file delimiter ---

func TestParseFrontmatter_EOFDelimiter(t *testing.T) {
	// Frontmatter closed by "---" at end of file (no trailing newline after body).
	content := "---\nid: eof-test\nsubject: EOF\ndate: 2025-01-01\nstatus: pending\n---"
	fm, body, err := parseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.ID != "eof-test" {
		t.Errorf("got ID %q, want %q", fm.ID, "eof-test")
	}
	if body != "" {
		t.Errorf("got body %q, want empty", body)
	}
}

// --- parseFrontmatter with tags and metadata ---

func TestParseFrontmatter_WithTags(t *testing.T) {
	content := "---\nid: tag-test\nsubject: Tags\ndate: 2025-01-01\nstatus: pending\ntags:\n  - urgent\n  - review\nrelated_task: TASK-00042\nmetadata:\n  source: manual\n---\n\nBody with tags"
	fm, body, err := parseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fm.Tags) != 2 {
		t.Errorf("got %d tags, want 2", len(fm.Tags))
	}
	if fm.RelatedTask != "TASK-00042" {
		t.Errorf("got related_task %q, want %q", fm.RelatedTask, "TASK-00042")
	}
	if fm.Metadata["source"] != "manual" {
		t.Errorf("got metadata[source] %q, want %q", fm.Metadata["source"], "manual")
	}
	if body != "Body with tags" {
		t.Errorf("got body %q, want %q", body, "Body with tags")
	}
}

// --- parseInboxFile preserves tags, metadata, relatedTask ---

func TestFileChannelAdapter_FetchPreservesFieldsFromFrontmatter(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "fields-test", BaseDir: dir})

	content := "---\nid: rich-item\nfrom: alice\nsubject: Rich\ndate: 2025-01-15\nstatus: pending\npriority: high\ntags:\n  - urgent\n  - bug\nrelated_task: TASK-00010\nmetadata:\n  source: api\n---\n\nRich body content"
	if err := os.WriteFile(filepath.Join(dir, "inbox", "rich-item.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := adapter.Fetch()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}

	item := items[0]
	if item.ID != "rich-item" {
		t.Errorf("ID = %q, want %q", item.ID, "rich-item")
	}
	if item.Priority != models.ChannelPriorityHigh {
		t.Errorf("Priority = %q, want %q", item.Priority, models.ChannelPriorityHigh)
	}
	if len(item.Tags) != 2 {
		t.Errorf("got %d tags, want 2", len(item.Tags))
	}
	if item.RelatedTask != "TASK-00010" {
		t.Errorf("RelatedTask = %q, want %q", item.RelatedTask, "TASK-00010")
	}
	if item.Metadata["source"] != "api" {
		t.Errorf("Metadata[source] = %q, want %q", item.Metadata["source"], "api")
	}
	if item.Source != "fields-test" {
		t.Errorf("Source = %q, want %q", item.Source, "fields-test")
	}
}

// --- findInboxFile slow path: scan files ignoring non-md and errors ---

func TestFileChannelAdapter_MarkProcessedSlowPathIgnoresNonMD(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	// Create a non-markdown file in inbox.
	if err := os.WriteFile(filepath.Join(dir, "inbox", "notes.txt"), []byte("text file"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create a file with a different filename but matching frontmatter ID.
	writeMD(t, filepath.Join(dir, "inbox", "different-name.md"), "target-id", "pending", "alice", "Test", "Body")

	err := adapter.MarkProcessed("target-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the file was updated.
	data, err := os.ReadFile(filepath.Join(dir, "inbox", "different-name.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "status: processed") {
		t.Error("expected status to be updated to processed")
	}
}

func TestFileChannelAdapter_MarkProcessedSlowPathSkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	adapter, _ := NewFileChannelAdapter(FileChannelConfig{Name: "test", BaseDir: dir})

	// Create a malformed markdown file (no frontmatter).
	if err := os.WriteFile(filepath.Join(dir, "inbox", "malformed.md"), []byte("no frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create the target file with correct frontmatter.
	writeMD(t, filepath.Join(dir, "inbox", "good-file.md"), "scan-target", "pending", "bob", "Test", "Body")

	err := adapter.MarkProcessed("scan-target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- parseFrontmatter with invalid YAML ---

func TestParseFrontmatter_InvalidYAML(t *testing.T) {
	content := "---\n: invalid: yaml: [[\n---\n\nBody"
	_, _, err := parseFrontmatter(content)
	if err == nil {
		t.Fatal("expected error for invalid YAML frontmatter")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

// writeMD is a test helper that writes a markdown file with YAML frontmatter.
func writeMD(t *testing.T, path, id, status, from, subject, body string) {
	t.Helper()
	content := "---\nid: " + id + "\nfrom: " + from + "\nsubject: " + subject + "\ndate: 2025-01-01\nstatus: " + status + "\n---\n\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
