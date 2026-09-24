package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
)

const foundationSchema = `
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TEXT NOT NULL
);

CREATE TABLE workspace_projection (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    workspace_id TEXT NOT NULL,
    root_uri TEXT NOT NULL,
    manifest_hash TEXT NOT NULL,
    observed_at TEXT NOT NULL
);

CREATE TABLE operations (
    operation_id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL,
    plan_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    finished_at TEXT,
    last_error TEXT
);

CREATE TABLE operation_steps (
    operation_id TEXT NOT NULL REFERENCES operations(operation_id),
    ordinal INTEGER NOT NULL,
    action TEXT NOT NULL,
    target_uri TEXT NOT NULL,
    before_hash TEXT,
    after_hash TEXT,
    status TEXT NOT NULL,
    last_error TEXT,
    PRIMARY KEY (operation_id, ordinal)
);

CREATE TABLE checkpoints (
    consumer TEXT PRIMARY KEY,
    cursor TEXT NOT NULL,
    source_hash TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const entityProjectionSchema = `
CREATE TABLE organization_projection (
    organization_id TEXT PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    path_uri TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    parent_id TEXT REFERENCES organization_projection(organization_id),
    manifest_hash TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
    observed_at TEXT NOT NULL,
    CHECK (parent_id IS NULL OR parent_id <> organization_id)
);

CREATE TABLE organization_aliases (
    organization_id TEXT NOT NULL
        REFERENCES organization_projection(organization_id) ON DELETE CASCADE,
    alias TEXT NOT NULL UNIQUE,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (organization_id, alias),
    UNIQUE (organization_id, ordinal)
);

CREATE TABLE repository_projection (
    repository_id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL
        REFERENCES organization_projection(organization_id),
    host TEXT NOT NULL,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    path_uri TEXT NOT NULL UNIQUE,
    manifest_hash TEXT NOT NULL,
    canonical_remote TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
    observed_at TEXT NOT NULL,
    UNIQUE (organization_id, host, owner, name)
);

CREATE TABLE repository_aliases (
    repository_id TEXT NOT NULL
        REFERENCES repository_projection(repository_id) ON DELETE CASCADE,
    organization_id TEXT NOT NULL
        REFERENCES organization_projection(organization_id),
    alias TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (repository_id, alias),
    UNIQUE (repository_id, ordinal),
    UNIQUE (organization_id, alias)
);

CREATE INDEX repository_projection_organization
    ON repository_projection (organization_id, host, owner, name);
`

const ticketProjectionSchema = `
CREATE UNIQUE INDEX repository_projection_id_organization
    ON repository_projection (repository_id, organization_id);

CREATE TABLE ticket_projection (
    ticket_id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL
        REFERENCES organization_projection(organization_id),
    repository_id TEXT,
    visible_key TEXT NOT NULL,
    path_uri TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    ticket_type TEXT NOT NULL,
    priority TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    profile_version TEXT NOT NULL,
    manifest_hash TEXT NOT NULL,
    archive_state TEXT NOT NULL
        CHECK (archive_state IN ('active', 'archived')),
    observed_at TEXT NOT NULL,
    FOREIGN KEY (repository_id, organization_id)
        REFERENCES repository_projection(repository_id, organization_id)
);

CREATE UNIQUE INDEX ticket_projection_organization_key
    ON ticket_projection (organization_id, visible_key)
    WHERE repository_id IS NULL;

CREATE UNIQUE INDEX ticket_projection_repository_key
    ON ticket_projection (repository_id, visible_key)
    WHERE repository_id IS NOT NULL;

CREATE INDEX ticket_projection_scope_order
    ON ticket_projection (
        organization_id, repository_id, visible_key, ticket_id
    );

CREATE TABLE ticket_aliases (
    ticket_id TEXT NOT NULL
        REFERENCES ticket_projection(ticket_id) ON DELETE CASCADE,
    alias TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (ticket_id, alias),
    UNIQUE (ticket_id, ordinal)
);

CREATE INDEX ticket_aliases_selector
    ON ticket_aliases (alias, ticket_id);

CREATE TABLE ticket_artifacts (
    ticket_id TEXT NOT NULL
        REFERENCES ticket_projection(ticket_id) ON DELETE CASCADE,
    semantic_role TEXT NOT NULL,
    path_uri TEXT NOT NULL,
    authority TEXT NOT NULL,
    search_policy TEXT NOT NULL,
    rendered_hash TEXT NOT NULL,
    source_hash TEXT NOT NULL,
    PRIMARY KEY (ticket_id, semantic_role),
    UNIQUE (ticket_id, path_uri)
);

CREATE INDEX ticket_artifacts_order
    ON ticket_artifacts (ticket_id, semantic_role, path_uri);

CREATE TABLE source_observations (
    ticket_id TEXT NOT NULL
        REFERENCES ticket_projection(ticket_id) ON DELETE CASCADE,
    canonical_source TEXT NOT NULL,
    authority_class TEXT NOT NULL
        CHECK (
            authority_class IN (
                'source', 'projection', 'cache', 'inference',
                'external_mirror'
            )
        ),
    content_hash TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    source_updated_at TEXT,
    ttl_nanoseconds INTEGER NOT NULL CHECK (ttl_nanoseconds >= 0),
    refresh_policy TEXT NOT NULL,
    state TEXT NOT NULL
        CHECK (
            state IN (
                'current', 'stale', 'incorrect', 'contested',
                'superseded', 'ignored'
            )
        ),
    last_success_at TEXT,
    last_error TEXT NOT NULL,
    PRIMARY KEY (ticket_id, canonical_source)
);

CREATE INDEX source_observations_order
    ON source_observations (ticket_id, canonical_source);

CREATE TABLE source_dependencies (
    ticket_id TEXT NOT NULL,
    canonical_source TEXT NOT NULL,
    depends_on_source TEXT NOT NULL,
    dependency_type TEXT NOT NULL,
    PRIMARY KEY (
        ticket_id, canonical_source, depends_on_source, dependency_type
    ),
    FOREIGN KEY (ticket_id, canonical_source)
        REFERENCES source_observations(ticket_id, canonical_source)
        ON DELETE CASCADE,
    FOREIGN KEY (ticket_id, depends_on_source)
        REFERENCES source_observations(ticket_id, canonical_source)
        ON DELETE CASCADE,
    CHECK (canonical_source <> depends_on_source)
);

CREATE INDEX source_dependencies_order
    ON source_dependencies (
        ticket_id, canonical_source, depends_on_source, dependency_type
    );
`

type migration struct {
	Version int
	Name    string
	SQL     string
}

func (item migration) checksum() string {
	sum := sha256.Sum256([]byte(item.SQL))
	return hex.EncodeToString(sum[:])
}

var currentMigrations = []migration{
	{
		Version: 1,
		Name:    "foundation",
		SQL:     foundationSchema,
	},
	{
		Version: 2,
		Name:    "entity-projections",
		SQL:     entityProjectionSchema,
	},
	{
		Version: 3,
		Name:    "ticket-artifact-freshness-projections",
		SQL:     ticketProjectionSchema,
	},
}
