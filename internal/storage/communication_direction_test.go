package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TestFileCommunicationManager_DirectionRoundTrip verifies the direction field
// (issue #121) survives a save → list → get round-trip.
func TestFileCommunicationManager_DirectionRoundTrip(t *testing.T) {
	mgr := NewFileCommunicationManager(t.TempDir())

	in := models.NewCommunication("comm-1", "TASK-00001", "they asked about retention")
	in.Direction = models.DirectionInbound
	in.Subject = "retention"
	in.From = "pm@acme.com"
	if err := mgr.SaveCommunication(in); err != nil {
		t.Fatalf("SaveCommunication error = %v", err)
	}

	out := models.NewCommunication("comm-2", "TASK-00001", "we replied")
	out.Direction = models.DirectionOutbound
	out.Subject = "re: retention"
	if err := mgr.SaveCommunication(out); err != nil {
		t.Fatalf("SaveCommunication error = %v", err)
	}

	all, err := mgr.GetAllCommunications("TASK-00001")
	if err != nil {
		t.Fatalf("GetAllCommunications error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d communications, want 2", len(all))
	}
	// Every stored communication kept its direction.
	dirs := map[models.CommunicationDirection]bool{}
	for _, c := range all {
		if !c.Direction.IsValid() {
			t.Fatalf("communication %q lost its direction: %q", c.ID, c.Direction)
		}
		dirs[c.Direction] = true
	}
	if !dirs[models.DirectionInbound] || !dirs[models.DirectionOutbound] {
		t.Fatalf("directions round-tripped = %v, want both inbound and outbound", dirs)
	}
}

// TestFileCommunicationManager_ResolverPlacesInNestedDir verifies the ticket-dir
// resolver (issue #121 fix): communications land inside the resolved nested
// ticket dir, not the flat tickets/<id>/ path.
func TestFileCommunicationManager_ResolverPlacesInNestedDir(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "github.com", "org", "repo", "TASK-00001-slug")
	mgr := NewFileCommunicationManager(base)
	mgr.SetTicketDirResolver(func(taskID string) (string, bool) {
		if taskID == "TASK-00001" {
			return nested, true
		}
		return "", false
	})
	c := models.NewCommunication("comm-1", "TASK-00001", "hi")
	c.Direction = models.DirectionInbound
	if err := mgr.SaveCommunication(c); err != nil {
		t.Fatal(err)
	}
	// The communication lives under the resolved nested dir, not base/TASK-00001.
	if entries, err := os.ReadDir(filepath.Join(nested, "communications")); err != nil || len(entries) == 0 {
		t.Fatalf("expected a communication under the nested dir, err=%v entries=%d", err, len(entries))
	}
	if _, err := os.Stat(filepath.Join(base, "TASK-00001", "communications")); !os.IsNotExist(err) {
		t.Fatalf("flat tickets/<id>/communications should NOT exist when a resolver is wired (err=%v)", err)
	}
	// And it is still retrievable through the manager.
	all, err := mgr.GetAllCommunications("TASK-00001")
	if err != nil || len(all) != 1 {
		t.Fatalf("GetAllCommunications after resolver = %d (err %v), want 1", len(all), err)
	}
}
