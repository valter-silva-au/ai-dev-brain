package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAddManagedFlag(t *testing.T) {
	t.Run("adds managed flag to existing settings", func(t *testing.T) {
		dir := t.TempDir()
		settingsPath := filepath.Join(dir, "settings.json")

		// Write initial settings.
		initial := map[string]interface{}{
			"permissions": map[string]interface{}{
				"allow": []interface{}{"Bash(git:*)"},
			},
		}
		data, _ := json.MarshalIndent(initial, "", "  ")
		if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
			t.Fatal(err)
		}

		// Add managed flag.
		if err := addManagedFlag(settingsPath); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Read back and verify.
		result, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatal(err)
		}

		var settings map[string]interface{}
		if err := json.Unmarshal(result, &settings); err != nil {
			t.Fatal(err)
		}

		managed, ok := settings["managed"].(bool)
		if !ok || !managed {
			t.Errorf("expected managed: true, got %v", settings["managed"])
		}

		// Verify original content preserved.
		if _, ok := settings["permissions"]; !ok {
			t.Error("expected permissions key to be preserved")
		}
	})

	t.Run("fails for nonexistent file", func(t *testing.T) {
		err := addManagedFlag("/nonexistent/path/settings.json")
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
	})
}
