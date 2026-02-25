package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

var migrateArchiveDryRun bool

var migrateArchiveCmd = &cobra.Command{
	Use:   "migrate-archive",
	Short: "Move archived task folders into tickets/_archived/",
	Long: `Scan the backlog for archived tasks whose ticket folders are still in
tickets/ and move them to tickets/_archived/. This is a one-time migration
command for transitioning to the new directory structure.

Use --dry-run to preview which tasks would be moved without making changes.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if TaskMgr == nil {
			return fmt.Errorf("task manager not initialized")
		}

		tasks, err := TaskMgr.GetTasksByStatus(models.StatusArchived)
		if err != nil {
			return fmt.Errorf("listing archived tasks: %w", err)
		}

		if len(tasks) == 0 {
			fmt.Println("No archived tasks found.")
			return nil
		}

		archivedBaseDir := filepath.Join(BasePath, "tickets", "_archived")
		moved := 0

		for _, task := range tasks {
			activeDir := filepath.Join(BasePath, "tickets", task.ID)

			// Only move if the ticket is still in the active location.
			if _, err := os.Stat(activeDir); os.IsNotExist(err) {
				continue
			}

			destDir := filepath.Join(archivedBaseDir, task.ID)

			if migrateArchiveDryRun {
				fmt.Printf("  [dry-run] %s -> tickets/_archived/%s\n", task.ID, task.ID)
				moved++
				continue
			}

			if err := os.MkdirAll(archivedBaseDir, 0o755); err != nil {
				fmt.Printf("  Warning: failed to create _archived directory: %v\n", err)
				continue
			}

			if err := os.Rename(activeDir, destDir); err != nil {
				fmt.Printf("  Warning: failed to move %s: %v\n", task.ID, err)
				continue
			}

			// Update TicketPath in the moved status.yaml.
			task.TicketPath = destDir
			statusPath := filepath.Join(destDir, "status.yaml")
			if data, marshalErr := yaml.Marshal(task); marshalErr == nil {
				_ = os.WriteFile(statusPath, data, 0o600)
			}

			fmt.Printf("  Moved %s -> tickets/_archived/%s\n", task.ID, task.ID)
			moved++
		}

		if moved == 0 {
			fmt.Println("All archived tasks are already in tickets/_archived/.")
		} else if migrateArchiveDryRun {
			fmt.Printf("\n%d task(s) would be moved. Run without --dry-run to apply.\n", moved)
		} else {
			fmt.Printf("\nMoved %d task(s) to tickets/_archived/.\n", moved)
		}

		return nil
	},
}

func init() {
	migrateArchiveCmd.Hidden = true
	migrateArchiveCmd.Flags().BoolVar(&migrateArchiveDryRun, "dry-run", false, "Preview changes without moving any files")
	rootCmd.AddCommand(migrateArchiveCmd)
}
