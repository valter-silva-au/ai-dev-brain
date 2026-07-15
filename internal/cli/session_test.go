package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

func TestSessionCmd(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Initialize App for testing
	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("Failed to initialize app: %v", err)
	}
	defer app.Cleanup()

	// Set the global App variable
	App = app

	t.Run("SessionCommandExists", func(t *testing.T) {
		cmd := NewSessionCmd()
		if cmd == nil {
			t.Fatal("Expected session command to be created")
		}

		if cmd.Use != "session" {
			t.Errorf("Expected command use 'session', got '%s'", cmd.Use)
		}

		// Check subcommands
		subcommands := cmd.Commands()
		expectedSubcommands := []string{"save", "ingest", "capture", "list", "show"}

		if len(subcommands) != len(expectedSubcommands) {
			t.Errorf("Expected %d subcommands, got %d", len(expectedSubcommands), len(subcommands))
		}

		foundCommands := make(map[string]bool)
		for _, subcmd := range subcommands {
			foundCommands[subcmd.Use] = true
		}

		for _, expected := range expectedSubcommands {
			if !foundCommands[expected] && !foundCommands[expected+" <file>"] && !foundCommands[expected+" <directory>"] && !foundCommands[expected+" <session-id>"] {
				t.Errorf("Expected subcommand '%s' not found", expected)
			}
		}
	})

	t.Run("SessionSave", func(t *testing.T) {
		// Create a test session YAML file
		session := models.NewCapturedSession("TEST-001")
		session.TaskID = "TASK-001"
		session.Summary = "Test session"
		session.Finalize()

		sessionFile := filepath.Join(tmpDir, "test-session.yaml")
		data, err := yaml.Marshal(session)
		if err != nil {
			t.Fatalf("Failed to marshal session: %v", err)
		}

		if err := os.WriteFile(sessionFile, data, 0o644); err != nil {
			t.Fatalf("Failed to write session file: %v", err)
		}

		// Create save command
		cmd := newSessionSaveCmd()
		cmd.SetArgs([]string{sessionFile})

		// Execute command
		if err := cmd.Execute(); err != nil {
			t.Errorf("Session save failed: %v", err)
		}

		// Verify session was saved
		sessionStore := app.GetSessionStore()
		savedSession, err := sessionStore.GetSession("TEST-001")
		if err != nil {
			t.Errorf("Failed to retrieve saved session: %v", err)
		}

		if savedSession.ID != "TEST-001" {
			t.Errorf("Expected session ID 'TEST-001', got '%s'", savedSession.ID)
		}
	})

	t.Run("SessionCapture", func(t *testing.T) {
		cmd := newSessionCaptureCmd()
		cmd.SetArgs([]string{"--task", "TASK-002", "--summary", "Test capture"})

		if err := cmd.Execute(); err != nil {
			t.Errorf("Session capture failed: %v", err)
		}

		// List sessions to verify capture
		sessions, err := app.GetSessionStore().ListSessions()
		if err != nil {
			t.Errorf("Failed to list sessions: %v", err)
		}

		// Should have at least the captured session (and possibly the saved one)
		if len(sessions) == 0 {
			t.Error("Expected at least one captured session")
		}
	})

	t.Run("SessionList", func(t *testing.T) {
		cmd := newSessionListCmd()
		cmd.SetArgs([]string{})

		// Capture output is redirected, so we just check it executes without error
		if err := cmd.Execute(); err != nil {
			t.Errorf("Session list failed: %v", err)
		}
	})

	t.Run("SessionShow", func(t *testing.T) {
		// First ensure we have a session
		sessionStore := app.GetSessionStore()
		sessions, _ := sessionStore.ListSessions()
		if len(sessions) == 0 {
			t.Skip("No sessions to show")
		}

		sessionID := sessions[0].ID

		cmd := newSessionShowCmd()
		cmd.SetArgs([]string{sessionID})

		if err := cmd.Execute(); err != nil {
			t.Errorf("Session show failed: %v", err)
		}
	})

	t.Run("SessionIngest", func(t *testing.T) {
		// Create a test directory structure
		ingestDir := filepath.Join(tmpDir, "ingest-test")
		os.MkdirAll(ingestDir, 0o755)

		// Create a dummy summary file
		summaryFile := filepath.Join(ingestDir, "summary.md")
		os.WriteFile(summaryFile, []byte("Test summary content"), 0o644)

		cmd := newSessionIngestCmd()
		cmd.SetArgs([]string{ingestDir, "--task", "TASK-003"})

		if err := cmd.Execute(); err != nil {
			t.Errorf("Session ingest failed: %v", err)
		}

		// Verify session was ingested
		sessions, err := app.GetSessionStore().ListSessions()
		if err != nil {
			t.Errorf("Failed to list sessions after ingest: %v", err)
		}

		// Should have multiple sessions now
		if len(sessions) == 0 {
			t.Error("Expected ingested session")
		}
	})
}

func TestSessionCmd_ErrorCases(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Initialize App for testing
	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("Failed to initialize app: %v", err)
	}
	defer app.Cleanup()

	// Set the global App variable
	App = app

	t.Run("SaveNonexistentFile", func(t *testing.T) {
		cmd := newSessionSaveCmd()
		cmd.SetArgs([]string{"/nonexistent/file.yaml"})

		if err := cmd.Execute(); err == nil {
			t.Error("Expected error when saving nonexistent file")
		}
	})

	t.Run("SaveInvalidYAML", func(t *testing.T) {
		invalidFile := filepath.Join(tmpDir, "invalid.yaml")
		os.WriteFile(invalidFile, []byte("invalid: yaml: content: {"), 0o644)

		cmd := newSessionSaveCmd()
		cmd.SetArgs([]string{invalidFile})

		if err := cmd.Execute(); err == nil {
			t.Error("Expected error when saving invalid YAML")
		}
	})

	t.Run("ShowNonexistentSession", func(t *testing.T) {
		cmd := newSessionShowCmd()
		cmd.SetArgs([]string{"NONEXISTENT-999"})

		if err := cmd.Execute(); err == nil {
			t.Error("Expected error when showing nonexistent session")
		}
	})

	t.Run("IngestNonexistentDirectory", func(t *testing.T) {
		cmd := newSessionIngestCmd()
		cmd.SetArgs([]string{"/nonexistent/directory"})

		if err := cmd.Execute(); err == nil {
			t.Error("Expected error when ingesting nonexistent directory")
		}
	})
}
