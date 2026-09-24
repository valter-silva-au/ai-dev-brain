package organization

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	ManifestSchema = "aidb.organization/v1"
	ManifestKind   = "Organization"
	StatusActive   = "active"
	StatusArchived = "archived"
)

type Roles struct {
	Config       string `yaml:"config" json:"config"`
	Agents       string `yaml:"agents" json:"agents"`
	Knowledge    string `yaml:"knowledge" json:"knowledge"`
	Stakeholders string `yaml:"stakeholders" json:"stakeholders"`
	Tickets      string `yaml:"tickets" json:"tickets"`
	Repositories string `yaml:"repositories" json:"repositories"`
}

type Provenance struct {
	OperationID string `yaml:"operation_id" json:"operation_id"`
	ActorType   string `yaml:"actor_type" json:"actor_type"`
	ActorID     string `yaml:"actor_id,omitempty" json:"actor_id,omitempty"`
	Tool        string `yaml:"tool" json:"tool"`
}

type Manifest struct {
	SchemaVersion string     `yaml:"schema_version" json:"schema_version"`
	Kind          string     `yaml:"kind" json:"kind"`
	ID            string     `yaml:"id" json:"id"`
	Slug          string     `yaml:"slug" json:"slug"`
	DisplayName   string     `yaml:"display_name" json:"display_name"`
	ParentID      string     `yaml:"parent_id,omitempty" json:"parent_id,omitempty"`
	Owner         string     `yaml:"owner,omitempty" json:"owner,omitempty"`
	Description   string     `yaml:"description,omitempty" json:"description,omitempty"`
	Trust         string     `yaml:"trust,omitempty" json:"trust,omitempty"`
	Profile       string     `yaml:"profile,omitempty" json:"profile,omitempty"`
	CreatedAt     time.Time  `yaml:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `yaml:"updated_at" json:"updated_at"`
	Status        string     `yaml:"status" json:"status"`
	ArchivedAt    *time.Time `yaml:"archived_at,omitempty" json:"archived_at,omitempty"`
	Aliases       []string   `yaml:"aliases" json:"aliases"`
	Provenance    Provenance `yaml:"provenance" json:"provenance"`
	LastMutation  Provenance `yaml:"last_mutation" json:"last_mutation"`
	Roles         Roles      `yaml:"roles" json:"roles"`
}

func DefaultRoles() Roles {
	return Roles{
		Config:       ".aidb/config.yaml",
		Agents:       "AGENTS.md",
		Knowledge:    "knowledge",
		Stakeholders: "stakeholders",
		Tickets:      "tickets",
		Repositories: "repos",
	}
}

func NewManifest(
	id string,
	slug string,
	displayName string,
	createdAt time.Time,
	provenance Provenance,
) Manifest {
	return Manifest{
		SchemaVersion: ManifestSchema,
		Kind:          ManifestKind,
		ID:            id,
		Slug:          slug,
		DisplayName:   displayName,
		CreatedAt:     createdAt.UTC(),
		UpdatedAt:     createdAt.UTC(),
		Status:        StatusActive,
		Aliases:       []string{},
		Provenance:    provenance,
		LastMutation:  provenance,
		Roles:         DefaultRoles(),
	}
}

func (manifest Manifest) Validate(layout Layout) error {
	switch {
	case manifest.SchemaVersion != ManifestSchema:
		return fmt.Errorf(
			"unsupported organization manifest schema %q",
			manifest.SchemaVersion,
		)
	case manifest.Kind != ManifestKind:
		return fmt.Errorf(
			"organization manifest kind must be %q",
			ManifestKind,
		)
	case manifest.ID == "":
		return errors.New("organization manifest id is required")
	case manifest.Slug != layout.Slug():
		return fmt.Errorf(
			"organization manifest slug %q does not match path slug %q",
			manifest.Slug,
			layout.Slug(),
		)
	case ValidateSlug(manifest.Slug) != nil:
		return ValidateSlug(manifest.Slug)
	case manifest.DisplayName == "":
		return errors.New("organization display name is required")
	case manifest.ParentID == manifest.ID:
		return errors.New("organization cannot parent itself")
	case manifest.CreatedAt.IsZero():
		return errors.New("organization created_at is required")
	case manifest.UpdatedAt.IsZero():
		return errors.New("organization updated_at is required")
	case manifest.UpdatedAt.Before(manifest.CreatedAt):
		return errors.New("organization updated_at precedes created_at")
	case manifest.Status != StatusActive && manifest.Status != StatusArchived:
		return fmt.Errorf(
			"unsupported organization status %q",
			manifest.Status,
		)
	case manifest.Status == StatusActive && manifest.ArchivedAt != nil:
		return errors.New("active organization cannot have archived_at")
	case manifest.Status == StatusArchived && manifest.ArchivedAt == nil:
		return errors.New("archived organization requires archived_at")
	case manifest.Provenance.OperationID == "":
		return errors.New("organization provenance operation_id is required")
	case manifest.Provenance.ActorType == "":
		return errors.New("organization provenance actor_type is required")
	case manifest.Provenance.Tool == "":
		return errors.New("organization provenance tool is required")
	case manifest.LastMutation.OperationID == "":
		return errors.New(
			"organization last_mutation operation_id is required",
		)
	case manifest.LastMutation.ActorType == "":
		return errors.New("organization last_mutation actor_type is required")
	case manifest.LastMutation.Tool == "":
		return errors.New("organization last_mutation tool is required")
	}

	seenAliases := make(map[string]struct{}, len(manifest.Aliases))
	for _, alias := range manifest.Aliases {
		if alias == "" {
			return errors.New("organization alias cannot be empty")
		}
		if alias == manifest.Slug {
			return fmt.Errorf(
				"organization alias %q duplicates the current slug",
				alias,
			)
		}
		if _, ok := seenAliases[alias]; ok {
			return fmt.Errorf("duplicate organization alias %q", alias)
		}
		seenAliases[alias] = struct{}{}
	}

	roles := map[string]string{
		"config":       manifest.Roles.Config,
		"agents":       manifest.Roles.Agents,
		"knowledge":    manifest.Roles.Knowledge,
		"stakeholders": manifest.Roles.Stakeholders,
		"tickets":      manifest.Roles.Tickets,
		"repositories": manifest.Roles.Repositories,
	}
	for name, role := range roles {
		if _, err := layout.ResolveRole(role); err != nil {
			return fmt.Errorf("validate organization role %s: %w", name, err)
		}
	}
	return nil
}

func EncodeManifest(writer io.Writer, manifest Manifest) error {
	encoder := yaml.NewEncoder(writer)
	encoder.SetIndent(2)
	defer func() {
		_ = encoder.Close()
	}()

	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("encode organization manifest: %w", err)
	}
	return nil
}

func DecodeManifest(reader io.Reader) (Manifest, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode organization manifest: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New(
				"organization manifest contains multiple documents",
			)
		}
		return Manifest{}, fmt.Errorf(
			"decode organization manifest trailer: %w",
			err,
		)
	}
	return manifest, nil
}

func ReadManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf(
			"open organization manifest %q: %w",
			path,
			err,
		)
	}
	defer func() {
		_ = file.Close()
	}()

	manifest, err := DecodeManifest(file)
	if err != nil {
		return Manifest{}, fmt.Errorf(
			"read organization manifest %q: %w",
			path,
			err,
		)
	}
	return manifest, nil
}
