package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestNewFileCommunicationManager(t *testing.T) {
	tempDir := t.TempDir()

	fcm := NewFileCommunicationManager(tempDir)
	if fcm == nil {
		t.Fatal("NewFileCommunicationManager returned nil")
	}
	if fcm.baseDir != tempDir {
		t.Errorf("Expected baseDir %s, got %s", tempDir, fcm.baseDir)
	}
}

func TestFileCommunicationManager_SaveCommunication(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	comm := models.NewCommunication("COMM-001", "TASK-001", "Test communication content")
	comm.Subject = "Test Subject"
	comm.From = "user@example.com"
	comm.To = []string{"dev@example.com"}
	comm.Channel = "email"

	err := fcm.SaveCommunication(comm)
	if err != nil {
		t.Fatalf("SaveCommunication() failed: %v", err)
	}

	// Verify directory was created
	commDir := filepath.Join(tempDir, "TASK-001", "communications")
	if _, err := os.Stat(commDir); os.IsNotExist(err) {
		t.Fatal("Communications directory was not created")
	}

	// Verify file was created
	files, err := os.ReadDir(commDir)
	if err != nil {
		t.Fatalf("Failed to read communications directory: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("Expected 1 file, found %d", len(files))
	}

	// Verify file has .md extension
	if !strings.HasSuffix(files[0].Name(), ".md") {
		t.Errorf("Expected .md file, got %s", files[0].Name())
	}

	// Verify file is present, regular, and owner-read/writable.
	assertOwnerReadWritableFile(t, filepath.Join(commDir, files[0].Name()))
}

func TestFileCommunicationManager_SaveCommunication_NilCheck(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	err := fcm.SaveCommunication(nil)
	if err == nil {
		t.Error("SaveCommunication() should fail with nil communication")
	}
}

func TestFileCommunicationManager_SaveCommunication_NoTaskID(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	comm := models.NewCommunication("COMM-001", "", "Test content")
	err := fcm.SaveCommunication(comm)
	if err == nil {
		t.Error("SaveCommunication() should fail without task ID")
	}
}

func TestFileCommunicationManager_GetCommunication(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	// Save a communication
	originalComm := models.NewCommunication("COMM-001", "TASK-001", "Test communication content")
	originalComm.Subject = "Test Subject"
	originalComm.From = "user@example.com"
	originalComm.To = []string{"dev@example.com"}
	originalComm.Channel = "email"
	originalComm.AddTag(models.CommunicationTagQuestion)

	err := fcm.SaveCommunication(originalComm)
	if err != nil {
		t.Fatalf("SaveCommunication() failed: %v", err)
	}

	// List communications to get the filename
	filenames, err := fcm.ListCommunications("TASK-001")
	if err != nil {
		t.Fatalf("ListCommunications() failed: %v", err)
	}
	if len(filenames) != 1 {
		t.Fatalf("Expected 1 communication file, found %d", len(filenames))
	}

	// Retrieve the communication
	retrievedComm, err := fcm.GetCommunication("TASK-001", filenames[0])
	if err != nil {
		t.Fatalf("GetCommunication() failed: %v", err)
	}

	// Verify fields
	if retrievedComm.ID != originalComm.ID {
		t.Errorf("Expected ID %s, got %s", originalComm.ID, retrievedComm.ID)
	}
	if retrievedComm.TaskID != originalComm.TaskID {
		t.Errorf("Expected TaskID %s, got %s", originalComm.TaskID, retrievedComm.TaskID)
	}
	if retrievedComm.Subject != originalComm.Subject {
		t.Errorf("Expected Subject %s, got %s", originalComm.Subject, retrievedComm.Subject)
	}
	if retrievedComm.From != originalComm.From {
		t.Errorf("Expected From %s, got %s", originalComm.From, retrievedComm.From)
	}
	if retrievedComm.Channel != originalComm.Channel {
		t.Errorf("Expected Channel %s, got %s", originalComm.Channel, retrievedComm.Channel)
	}
}

func TestFileCommunicationManager_GetCommunication_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	_, err := fcm.GetCommunication("TASK-001", "nonexistent.md")
	if err == nil {
		t.Error("GetCommunication() should fail for non-existent file")
	}
}

func TestFileCommunicationManager_ListCommunications(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	// Save multiple communications
	for i := 1; i <= 3; i++ {
		comm := models.NewCommunication(
			"COMM-00"+string(rune('0'+i)),
			"TASK-001",
			"Communication content "+string(rune('0'+i)),
		)
		comm.Subject = "Subject " + string(rune('0'+i))
		comm.Date = time.Now().Add(time.Duration(i) * time.Hour)

		err := fcm.SaveCommunication(comm)
		if err != nil {
			t.Fatalf("SaveCommunication() failed: %v", err)
		}
	}

	// List communications
	filenames, err := fcm.ListCommunications("TASK-001")
	if err != nil {
		t.Fatalf("ListCommunications() failed: %v", err)
	}

	if len(filenames) != 3 {
		t.Errorf("Expected 3 communications, found %d", len(filenames))
	}

	// Verify all filenames end with .md
	for _, filename := range filenames {
		if !strings.HasSuffix(filename, ".md") {
			t.Errorf("Expected .md file, got %s", filename)
		}
	}
}

func TestFileCommunicationManager_ListCommunications_Empty(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	filenames, err := fcm.ListCommunications("TASK-999")
	if err != nil {
		t.Fatalf("ListCommunications() failed: %v", err)
	}

	if len(filenames) != 0 {
		t.Errorf("Expected 0 communications for non-existent task, found %d", len(filenames))
	}
}

func TestFileCommunicationManager_GetAllCommunications(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	// Save multiple communications with different dates
	dates := []time.Time{
		time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 12, 14, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC),
	}

	for i, date := range dates {
		comm := models.NewCommunication(
			"COMM-00"+string(rune('1'+i)),
			"TASK-001",
			"Communication content",
		)
		comm.Subject = "Subject " + string(rune('1'+i))
		comm.Date = date

		err := fcm.SaveCommunication(comm)
		if err != nil {
			t.Fatalf("SaveCommunication() failed: %v", err)
		}
	}

	// Get all communications
	communications, err := fcm.GetAllCommunications("TASK-001")
	if err != nil {
		t.Fatalf("GetAllCommunications() failed: %v", err)
	}

	if len(communications) != 3 {
		t.Errorf("Expected 3 communications, got %d", len(communications))
	}

	// Verify sorted by date (newest first)
	if !communications[0].Date.After(communications[1].Date) {
		t.Error("Communications not sorted correctly (newest first)")
	}
	if !communications[1].Date.After(communications[2].Date) {
		t.Error("Communications not sorted correctly (newest first)")
	}
}

func TestFileCommunicationManager_GetAllCommunications_Empty(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	communications, err := fcm.GetAllCommunications("TASK-999")
	if err != nil {
		t.Fatalf("GetAllCommunications() failed: %v", err)
	}

	if len(communications) != 0 {
		t.Errorf("Expected 0 communications for non-existent task, found %d", len(communications))
	}
}

func TestFileCommunicationManager_MultipleTasks(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	// Save communications for different tasks
	tasks := []string{"TASK-001", "TASK-002", "TASK-003"}
	for _, taskID := range tasks {
		comm := models.NewCommunication("COMM-"+taskID, taskID, "Content for "+taskID)
		comm.Subject = "Subject for " + taskID

		err := fcm.SaveCommunication(comm)
		if err != nil {
			t.Fatalf("SaveCommunication() failed for %s: %v", taskID, err)
		}
	}

	// Verify each task has its own communications
	for _, taskID := range tasks {
		filenames, err := fcm.ListCommunications(taskID)
		if err != nil {
			t.Fatalf("ListCommunications() failed for %s: %v", taskID, err)
		}
		if len(filenames) != 1 {
			t.Errorf("Expected 1 communication for %s, found %d", taskID, len(filenames))
		}
	}
}

func TestFileCommunicationManager_FilenameGeneration(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	tests := []struct {
		name            string
		subject         string
		expectedPattern string
	}{
		{
			name:            "simple subject",
			subject:         "Meeting Notes",
			expectedPattern: "_meeting_notes.md",
		},
		{
			name:            "subject with special characters",
			subject:         "Q&A Session #1",
			expectedPattern: "_q_a_session__1.md",
		},
		{
			name:            "empty subject",
			subject:         "",
			expectedPattern: "_COMM-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comm := models.NewCommunication("COMM-"+tt.name, "TASK-001", "Test content")
			comm.Subject = tt.subject

			err := fcm.SaveCommunication(comm)
			if err != nil {
				t.Fatalf("SaveCommunication() failed: %v", err)
			}

			filenames, err := fcm.ListCommunications("TASK-001")
			if err != nil {
				t.Fatalf("ListCommunications() failed: %v", err)
			}

			// Find the file with our pattern
			found := false
			for _, filename := range filenames {
				if strings.Contains(filename, tt.expectedPattern) {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Expected filename pattern %s not found in %v", tt.expectedPattern, filenames)
			}

			// Clean up for next test
			commDir := filepath.Join(tempDir, "TASK-001", "communications")
			os.RemoveAll(commDir)
		})
	}
}

func TestFileCommunicationManager_CommunicationWithActionItems(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	comm := models.NewCommunication("COMM-001", "TASK-001", "Meeting notes with action items")
	comm.Subject = "Sprint Planning"

	// Add action items
	actionItem1 := models.NewActionItem("Complete task implementation")
	actionItem1.Assignee = "developer@example.com"
	comm.AddActionItem(actionItem1)

	actionItem2 := models.NewActionItem("Review pull request")
	actionItem2.Assignee = "reviewer@example.com"
	comm.AddActionItem(actionItem2)

	err := fcm.SaveCommunication(comm)
	if err != nil {
		t.Fatalf("SaveCommunication() failed: %v", err)
	}

	// Retrieve and verify
	filenames, err := fcm.ListCommunications("TASK-001")
	if err != nil {
		t.Fatalf("ListCommunications() failed: %v", err)
	}

	retrievedComm, err := fcm.GetCommunication("TASK-001", filenames[0])
	if err != nil {
		t.Fatalf("GetCommunication() failed: %v", err)
	}

	if len(retrievedComm.ActionItems) != 2 {
		t.Errorf("Expected 2 action items, got %d", len(retrievedComm.ActionItems))
	}
}

func TestFileCommunicationManager_CommunicationWithTags(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	comm := models.NewCommunication("COMM-001", "TASK-001", "Important blocker")
	comm.Subject = "Blocker Issue"
	comm.AddTag(models.CommunicationTagBlocker)
	comm.AddTag(models.CommunicationTagQuestion)

	err := fcm.SaveCommunication(comm)
	if err != nil {
		t.Fatalf("SaveCommunication() failed: %v", err)
	}

	// Retrieve and verify
	filenames, err := fcm.ListCommunications("TASK-001")
	if err != nil {
		t.Fatalf("ListCommunications() failed: %v", err)
	}

	retrievedComm, err := fcm.GetCommunication("TASK-001", filenames[0])
	if err != nil {
		t.Fatalf("GetCommunication() failed: %v", err)
	}

	if len(retrievedComm.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(retrievedComm.Tags))
	}

	if !retrievedComm.HasTag(models.CommunicationTagBlocker) {
		t.Error("Expected blocker tag")
	}
	if !retrievedComm.HasTag(models.CommunicationTagQuestion) {
		t.Error("Expected question tag")
	}
}

func TestFileCommunicationManager_DirectoryPermissions(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileCommunicationManager(tempDir)

	comm := models.NewCommunication("COMM-001", "TASK-001", "Test")
	err := fcm.SaveCommunication(comm)
	if err != nil {
		t.Fatalf("SaveCommunication() failed: %v", err)
	}

	// Verify communications directory is present + owner-accessible.
	commDir := filepath.Join(tempDir, "TASK-001", "communications")
	assertOwnerAccessibleDir(t, commDir)
}
