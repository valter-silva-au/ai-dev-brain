package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	ManifestSchema = "aidb.workspace/v1"
	ManifestKind   = "Workspace"
)

type Roles struct {
	Config        string `yaml:"config" json:"config"`
	State         string `yaml:"state" json:"state"`
	Events        string `yaml:"events" json:"events"`
	Cache         string `yaml:"cache" json:"cache"`
	Organizations string `yaml:"organizations" json:"organizations"`
}

type Manifest struct {
	SchemaVersion string    `yaml:"schema_version" json:"schema_version"`
	Kind          string    `yaml:"kind" json:"kind"`
	ID            string    `yaml:"id" json:"id"`
	Name          string    `yaml:"name" json:"name"`
	CreatedAt     time.Time `yaml:"created_at" json:"created_at"`
	Roles         Roles     `yaml:"roles" json:"roles"`
}

func DefaultRoles() Roles {
	return Roles{
		Config:        ".aidb/config.yaml",
		State:         ".aidb/state.sqlite",
		Events:        ".aidb/events",
		Cache:         ".aidb/cache",
		Organizations: "organizations",
	}
}

func NewManifest(id string, name string, createdAt time.Time) Manifest {
	return Manifest{
		SchemaVersion: ManifestSchema,
		Kind:          ManifestKind,
		ID:            id,
		Name:          name,
		CreatedAt:     createdAt.UTC(),
		Roles:         DefaultRoles(),
	}
}

func (manifest Manifest) Validate(layout Layout) error {
	if manifest.SchemaVersion != ManifestSchema {
		return fmt.Errorf(
			"unsupported workspace manifest schema %q",
			manifest.SchemaVersion,
		)
	}
	if manifest.Kind != ManifestKind {
		return fmt.Errorf("workspace manifest kind must be %q", ManifestKind)
	}
	if manifest.ID == "" {
		return errors.New("workspace manifest id is required")
	}
	if manifest.Name == "" {
		return errors.New("workspace manifest name is required")
	}
	if manifest.CreatedAt.IsZero() {
		return errors.New("workspace manifest created_at is required")
	}

	roles := map[string]string{
		"config":        manifest.Roles.Config,
		"state":         manifest.Roles.State,
		"events":        manifest.Roles.Events,
		"cache":         manifest.Roles.Cache,
		"organizations": manifest.Roles.Organizations,
	}
	for name, role := range roles {
		if _, err := layout.ResolveRole(role); err != nil {
			return fmt.Errorf("validate workspace role %s: %w", name, err)
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
		return fmt.Errorf("encode workspace manifest: %w", err)
	}
	return nil
}

func DecodeManifest(reader io.Reader) (Manifest, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode workspace manifest: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New("workspace manifest contains multiple documents")
		}
		return Manifest{}, fmt.Errorf("decode workspace manifest trailer: %w", err)
	}

	return manifest, nil
}

func ReadManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("open workspace manifest %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	manifest, err := DecodeManifest(file)
	if err != nil {
		return Manifest{}, fmt.Errorf("read workspace manifest %q: %w", path, err)
	}
	return manifest, nil
}
