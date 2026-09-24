package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type EntityStatus string

const (
	EntityStatusActive   EntityStatus = "active"
	EntityStatusArchived EntityStatus = "archived"
)

type OrganizationProjection struct {
	ID           string
	Slug         string
	Path         string
	DisplayName  string
	ParentID     string
	ManifestHash string
	Status       EntityStatus
	Aliases      []string
	ObservedAt   time.Time
}

type RepositoryProjection struct {
	ID              string
	OrganizationID  string
	Host            string
	Owner           string
	Name            string
	Path            string
	ManifestHash    string
	CanonicalRemote string
	Status          EntityStatus
	Aliases         []string
	ObservedAt      time.Time
}

func (store *Store) ObserveOrganization(
	ctx context.Context,
	projection OrganizationProjection,
) (err error) {
	if err = validateOrganizationProjection(projection); err != nil {
		return err
	}

	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin organization projection: %w", err)
	}
	committed := false
	defer unwindTransaction(
		transaction,
		&committed,
		"organization projection",
		&err,
	)

	if err := checkOrganizationNameConflicts(
		ctx,
		transaction,
		projection,
	); err != nil {
		return err
	}

	var parentID any
	if projection.ParentID != "" {
		parentID = projection.ParentID
	}
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO organization_projection (
			organization_id, slug, path_uri, display_name, parent_id,
			manifest_hash, status, observed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(organization_id) DO UPDATE SET
			slug = excluded.slug,
			path_uri = excluded.path_uri,
			display_name = excluded.display_name,
			parent_id = excluded.parent_id,
			manifest_hash = excluded.manifest_hash,
			status = excluded.status,
			observed_at = excluded.observed_at`,
		projection.ID,
		projection.Slug,
		projection.Path,
		projection.DisplayName,
		parentID,
		projection.ManifestHash,
		projection.Status,
		projection.ObservedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record organization projection: %w", err)
	}

	if _, err := transaction.ExecContext(
		ctx,
		"DELETE FROM organization_aliases WHERE organization_id = ?",
		projection.ID,
	); err != nil {
		return fmt.Errorf("replace organization aliases: %w", err)
	}
	for ordinal, alias := range uniqueAliases(projection.Aliases) {
		if alias == projection.Slug || alias == projection.Path {
			continue
		}
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO organization_aliases (
				organization_id, alias, ordinal
			) VALUES (?, ?, ?)`,
			projection.ID,
			alias,
			ordinal,
		); err != nil {
			return fmt.Errorf("record organization alias %q: %w", alias, err)
		}
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit organization projection: %w", err)
	}
	committed = true
	return nil
}

func (store *Store) Organization(
	ctx context.Context,
	selector string,
) (OrganizationProjection, error) {
	if selector == "" {
		return OrganizationProjection{}, errors.New(
			"organization selector is required",
		)
	}

	var projection OrganizationProjection
	var parentID sql.NullString
	var observedAt string
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT organization_id, slug, path_uri, display_name, parent_id,
		        manifest_hash, status, observed_at
		   FROM organization_projection
		  WHERE organization_id = ?
		     OR slug = ?
		     OR path_uri = ?
		     OR organization_id = (
				SELECT organization_id
				  FROM organization_aliases
				 WHERE alias = ?
		     )`,
		selector,
		selector,
		selector,
		selector,
	).Scan(
		&projection.ID,
		&projection.Slug,
		&projection.Path,
		&projection.DisplayName,
		&parentID,
		&projection.ManifestHash,
		&projection.Status,
		&observedAt,
	); err != nil {
		return OrganizationProjection{}, fmt.Errorf(
			"read organization projection %q: %w",
			selector,
			err,
		)
	}
	if parentID.Valid {
		projection.ParentID = parentID.String
	}
	parsed, err := time.Parse(time.RFC3339Nano, observedAt)
	if err != nil {
		return OrganizationProjection{}, fmt.Errorf(
			"parse organization %q observed_at: %w",
			projection.ID,
			err,
		)
	}
	projection.ObservedAt = parsed
	projection.Aliases, err = store.organizationAliases(ctx, projection.ID)
	if err != nil {
		return OrganizationProjection{}, err
	}
	return projection, nil
}

func (store *Store) Organizations(
	ctx context.Context,
) ([]OrganizationProjection, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT organization_id
		   FROM organization_projection
		  ORDER BY slug, organization_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list organization projections: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan organization projection: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organization projections: %w", err)
	}

	projections := make([]OrganizationProjection, 0, len(ids))
	for _, id := range ids {
		projection, err := store.Organization(ctx, id)
		if err != nil {
			return nil, err
		}
		projections = append(projections, projection)
	}
	return projections, nil
}

func (store *Store) ObserveRepository(
	ctx context.Context,
	projection RepositoryProjection,
) (err error) {
	if err = validateRepositoryProjection(projection); err != nil {
		return err
	}

	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin repository projection: %w", err)
	}
	committed := false
	defer unwindTransaction(
		transaction,
		&committed,
		"repository projection",
		&err,
	)

	if err := checkRepositoryNameConflicts(
		ctx,
		transaction,
		projection,
	); err != nil {
		return err
	}

	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO repository_projection (
			repository_id, organization_id, host, owner, name, path_uri,
			manifest_hash, canonical_remote, status, observed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id) DO UPDATE SET
			organization_id = excluded.organization_id,
			host = excluded.host,
			owner = excluded.owner,
			name = excluded.name,
			path_uri = excluded.path_uri,
			manifest_hash = excluded.manifest_hash,
			canonical_remote = excluded.canonical_remote,
			status = excluded.status,
			observed_at = excluded.observed_at`,
		projection.ID,
		projection.OrganizationID,
		projection.Host,
		projection.Owner,
		projection.Name,
		projection.Path,
		projection.ManifestHash,
		projection.CanonicalRemote,
		projection.Status,
		projection.ObservedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record repository projection: %w", err)
	}

	if _, err := transaction.ExecContext(
		ctx,
		"DELETE FROM repository_aliases WHERE repository_id = ?",
		projection.ID,
	); err != nil {
		return fmt.Errorf("replace repository aliases: %w", err)
	}
	canonical := repositoryKey(
		projection.Host,
		projection.Owner,
		projection.Name,
	)
	for ordinal, alias := range uniqueAliases(projection.Aliases) {
		if alias == canonical || alias == projection.Path {
			continue
		}
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO repository_aliases (
				repository_id, organization_id, alias, ordinal
			) VALUES (?, ?, ?, ?)`,
			projection.ID,
			projection.OrganizationID,
			alias,
			ordinal,
		); err != nil {
			return fmt.Errorf("record repository alias %q: %w", alias, err)
		}
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit repository projection: %w", err)
	}
	committed = true
	return nil
}

func (store *Store) Repository(
	ctx context.Context,
	organizationID string,
	selector string,
) (RepositoryProjection, error) {
	if organizationID == "" {
		return RepositoryProjection{}, errors.New(
			"repository organization id is required",
		)
	}
	if selector == "" {
		return RepositoryProjection{}, errors.New(
			"repository selector is required",
		)
	}

	var projection RepositoryProjection
	var observedAt string
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT repository_id, organization_id, host, owner, name, path_uri,
		        manifest_hash, canonical_remote, status, observed_at
		   FROM repository_projection
		  WHERE organization_id = ?
		    AND (
				repository_id = ?
				OR path_uri = ?
				OR host || '/' || owner || '/' || name = ?
				OR repository_id = (
					SELECT repository_id
					  FROM repository_aliases
					 WHERE organization_id = ? AND alias = ?
				)
		    )`,
		organizationID,
		selector,
		selector,
		selector,
		organizationID,
		selector,
	).Scan(
		&projection.ID,
		&projection.OrganizationID,
		&projection.Host,
		&projection.Owner,
		&projection.Name,
		&projection.Path,
		&projection.ManifestHash,
		&projection.CanonicalRemote,
		&projection.Status,
		&observedAt,
	); err != nil {
		return RepositoryProjection{}, fmt.Errorf(
			"read repository projection %q: %w",
			selector,
			err,
		)
	}
	parsed, err := time.Parse(time.RFC3339Nano, observedAt)
	if err != nil {
		return RepositoryProjection{}, fmt.Errorf(
			"parse repository %q observed_at: %w",
			projection.ID,
			err,
		)
	}
	projection.ObservedAt = parsed
	projection.Aliases, err = store.repositoryAliases(ctx, projection.ID)
	if err != nil {
		return RepositoryProjection{}, err
	}
	return projection, nil
}

func (store *Store) Repositories(
	ctx context.Context,
	organizationID string,
) ([]RepositoryProjection, error) {
	if organizationID == "" {
		return nil, errors.New("repository organization id is required")
	}

	rows, err := store.db.QueryContext(
		ctx,
		`SELECT repository_id
		   FROM repository_projection
		  WHERE organization_id = ?
		  ORDER BY host, owner, name, repository_id`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list repository projections: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan repository projection: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repository projections: %w", err)
	}

	projections := make([]RepositoryProjection, 0, len(ids))
	for _, id := range ids {
		projection, err := store.Repository(ctx, organizationID, id)
		if err != nil {
			return nil, err
		}
		projections = append(projections, projection)
	}
	return projections, nil
}

func validateOrganizationProjection(
	projection OrganizationProjection,
) error {
	switch {
	case projection.ID == "":
		return errors.New("organization projection id is required")
	case projection.Slug == "":
		return errors.New("organization projection slug is required")
	case projection.Path == "":
		return errors.New("organization projection path is required")
	case projection.DisplayName == "":
		return errors.New("organization projection display name is required")
	case projection.ParentID == projection.ID:
		return errors.New("organization projection cannot parent itself")
	case projection.ManifestHash == "":
		return errors.New("organization projection manifest hash is required")
	case !validEntityStatus(projection.Status):
		return fmt.Errorf(
			"unsupported organization projection status %q",
			projection.Status,
		)
	case projection.ObservedAt.IsZero():
		return errors.New("organization projection observed_at is required")
	}
	return validateAliases(projection.Aliases)
}

func validateRepositoryProjection(
	projection RepositoryProjection,
) error {
	switch {
	case projection.ID == "":
		return errors.New("repository projection id is required")
	case projection.OrganizationID == "":
		return errors.New("repository projection organization id is required")
	case projection.Host == "":
		return errors.New("repository projection host is required")
	case projection.Owner == "":
		return errors.New("repository projection owner is required")
	case projection.Name == "":
		return errors.New("repository projection name is required")
	case projection.Path == "":
		return errors.New("repository projection path is required")
	case projection.ManifestHash == "":
		return errors.New("repository projection manifest hash is required")
	case projection.CanonicalRemote == "":
		return errors.New("repository projection canonical remote is required")
	case !validEntityStatus(projection.Status):
		return fmt.Errorf(
			"unsupported repository projection status %q",
			projection.Status,
		)
	case projection.ObservedAt.IsZero():
		return errors.New("repository projection observed_at is required")
	}
	return validateAliases(projection.Aliases)
}

func validEntityStatus(status EntityStatus) bool {
	return status == EntityStatusActive || status == EntityStatusArchived
}

func validateAliases(aliases []string) error {
	seen := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		if strings.TrimSpace(alias) == "" {
			return errors.New("projection alias cannot be empty")
		}
		if _, ok := seen[alias]; ok {
			return fmt.Errorf("duplicate projection alias %q", alias)
		}
		seen[alias] = struct{}{}
	}
	return nil
}

func uniqueAliases(aliases []string) []string {
	result := make([]string, 0, len(aliases))
	seen := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		result = append(result, alias)
	}
	return result
}

func checkOrganizationNameConflicts(
	ctx context.Context,
	transaction *sql.Tx,
	projection OrganizationProjection,
) error {
	for _, candidate := range []string{projection.Slug, projection.Path} {
		var owner string
		err := transaction.QueryRowContext(
			ctx,
			`SELECT organization_id
			   FROM organization_aliases
			  WHERE alias = ? AND organization_id <> ?`,
			candidate,
			projection.ID,
		).Scan(&owner)
		if err == nil {
			return fmt.Errorf(
				"organization name %q is an alias of %q",
				candidate,
				owner,
			)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check organization alias collision: %w", err)
		}
	}

	for _, alias := range projection.Aliases {
		var owner string
		err := transaction.QueryRowContext(
			ctx,
			`SELECT organization_id
			   FROM organization_projection
			  WHERE organization_id <> ?
			    AND (slug = ? OR path_uri = ?)`,
			projection.ID,
			alias,
			alias,
		).Scan(&owner)
		if err == nil {
			return fmt.Errorf(
				"organization alias %q is owned by %q",
				alias,
				owner,
			)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check organization name collision: %w", err)
		}
	}
	return nil
}

func checkRepositoryNameConflicts(
	ctx context.Context,
	transaction *sql.Tx,
	projection RepositoryProjection,
) error {
	canonical := repositoryKey(
		projection.Host,
		projection.Owner,
		projection.Name,
	)
	for _, candidate := range []string{canonical, projection.Path} {
		var owner string
		err := transaction.QueryRowContext(
			ctx,
			`SELECT repository_id
			   FROM repository_aliases
			  WHERE organization_id = ?
			    AND alias = ?
			    AND repository_id <> ?`,
			projection.OrganizationID,
			candidate,
			projection.ID,
		).Scan(&owner)
		if err == nil {
			return fmt.Errorf(
				"repository name %q is an alias of %q",
				candidate,
				owner,
			)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check repository alias collision: %w", err)
		}
	}

	for _, alias := range projection.Aliases {
		var owner string
		err := transaction.QueryRowContext(
			ctx,
			`SELECT repository_id
			   FROM repository_projection
			  WHERE organization_id = ?
			    AND repository_id <> ?
			    AND (
					path_uri = ?
					OR host || '/' || owner || '/' || name = ?
			    )`,
			projection.OrganizationID,
			projection.ID,
			alias,
			alias,
		).Scan(&owner)
		if err == nil {
			return fmt.Errorf(
				"repository alias %q is owned by %q",
				alias,
				owner,
			)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check repository name collision: %w", err)
		}
	}
	return nil
}

func (store *Store) organizationAliases(
	ctx context.Context,
	organizationID string,
) ([]string, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT alias
		   FROM organization_aliases
		  WHERE organization_id = ?
		  ORDER BY ordinal`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("read organization aliases: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	return scanAliases(rows, "organization")
}

func (store *Store) repositoryAliases(
	ctx context.Context,
	repositoryID string,
) ([]string, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT alias
		   FROM repository_aliases
		  WHERE repository_id = ?
		  ORDER BY ordinal`,
		repositoryID,
	)
	if err != nil {
		return nil, fmt.Errorf("read repository aliases: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	return scanAliases(rows, "repository")
}

func scanAliases(rows *sql.Rows, kind string) ([]string, error) {
	aliases := make([]string, 0)
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, fmt.Errorf("scan %s alias: %w", kind, err)
		}
		aliases = append(aliases, alias)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s aliases: %w", kind, err)
	}
	return aliases, nil
}

func repositoryKey(host string, owner string, name string) string {
	return host + "/" + owner + "/" + name
}
