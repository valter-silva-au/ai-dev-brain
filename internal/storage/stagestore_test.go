package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestFileStageStore_Organizations_CRUD(t *testing.T) {
	store := NewFileStageStore(t.TempDir())

	// Empty registry lists nothing and finds nothing.
	orgs, err := store.ListOrganizations()
	if err != nil {
		t.Fatalf("ListOrganizations on empty store: %v", err)
	}
	if len(orgs) != 0 {
		t.Fatalf("empty store returned %d orgs, want 0", len(orgs))
	}
	if _, found, err := store.GetOrganization("acme"); err != nil || found {
		t.Fatalf("GetOrganization on empty store = found %v, err %v; want not found", found, err)
	}

	org := models.Organization{ID: "acme", Name: "Acme", GitHost: "github.com", Created: time.Now().UTC()}
	if err := store.CreateOrganization(org); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	got, found, err := store.GetOrganization("acme")
	if err != nil || !found {
		t.Fatalf("GetOrganization after create = found %v, err %v", found, err)
	}
	if got.Name != "Acme" || got.GitHost != "github.com" {
		t.Errorf("GetOrganization returned %+v, want Name=Acme GitHost=github.com", got)
	}

	// Duplicate ID is rejected.
	if err := store.CreateOrganization(org); err == nil {
		t.Error("CreateOrganization with duplicate ID should error")
	}
}

func TestFileStageStore_Persistence_AcrossInstances(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStageStore(dir)
	if err := store.CreateOrganization(models.Organization{ID: "acme", Name: "Acme"}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	// A fresh store over the same dir must see the persisted org.
	reopened := NewFileStageStore(dir)
	orgs, err := reopened.ListOrganizations()
	if err != nil {
		t.Fatalf("ListOrganizations after reopen: %v", err)
	}
	if len(orgs) != 1 || orgs[0].ID != "acme" {
		t.Fatalf("reopened store = %+v, want [acme]", orgs)
	}

	// The registry lives at orgs/index.yaml under the workspace root.
	if _, err := os.Stat(filepath.Join(dir, "orgs", "index.yaml")); err != nil {
		t.Errorf("orgs/index.yaml not written: %v", err)
	}
}

func TestFileStageStore_Initiatives_CRUD_And_Update(t *testing.T) {
	store := NewFileStageStore(t.TempDir())

	init := models.Initiative{ID: "widget", Name: "Widget", OrgID: "acme", Stage: models.StageIdea, Created: time.Now().UTC()}
	if err := store.CreateInitiative(init); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}

	if err := store.CreateInitiative(init); err == nil {
		t.Error("CreateInitiative with duplicate ID should error")
	}

	// Update the stage.
	init.Stage = models.StageMVP
	if err := store.UpdateInitiative(init); err != nil {
		t.Fatalf("UpdateInitiative: %v", err)
	}
	got, found, err := store.GetInitiative("widget")
	if err != nil || !found {
		t.Fatalf("GetInitiative = found %v, err %v", found, err)
	}
	if got.Stage != models.StageMVP {
		t.Errorf("stage after update = %q, want MVP", got.Stage)
	}

	// Updating a non-existent initiative errors.
	if err := store.UpdateInitiative(models.Initiative{ID: "ghost"}); err == nil {
		t.Error("UpdateInitiative on missing initiative should error")
	}
}
