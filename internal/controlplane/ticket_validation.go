package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func validateTicketProjection(projection TicketProjection) error {
	switch {
	case strings.TrimSpace(projection.ID) == "":
		return errors.New("ticket projection id is required")
	case strings.TrimSpace(projection.OrganizationID) == "":
		return errors.New("ticket projection organization id is required")
	case strings.TrimSpace(projection.VisibleKey) == "":
		return errors.New("ticket projection visible key is required")
	case strings.TrimSpace(projection.Path) == "":
		return errors.New("ticket projection path is required")
	case strings.TrimSpace(projection.Status) == "":
		return errors.New("ticket projection status is required")
	case strings.TrimSpace(projection.Type) == "":
		return errors.New("ticket projection type is required")
	case strings.TrimSpace(projection.Priority) == "":
		return errors.New("ticket projection priority is required")
	case strings.TrimSpace(projection.ProfileID) == "":
		return errors.New("ticket projection profile id is required")
	case strings.TrimSpace(projection.ProfileVersion) == "":
		return errors.New("ticket projection profile version is required")
	case strings.TrimSpace(projection.ManifestHash) == "":
		return errors.New("ticket projection manifest hash is required")
	case !validTicketArchiveState(projection.ArchiveState):
		return fmt.Errorf(
			"unsupported ticket archive state %q",
			projection.ArchiveState,
		)
	case projection.ObservedAt.IsZero():
		return errors.New("ticket projection observed_at is required")
	}
	if err := validateTicketAliases(projection); err != nil {
		return err
	}
	if err := validateArtifactProjections(projection); err != nil {
		return err
	}
	return validateSourceProjections(projection)
}

func validateTicketScope(scope TicketScope) error {
	if strings.TrimSpace(scope.OrganizationID) == "" {
		return errors.New("ticket organization id is required")
	}
	return nil
}

func validTicketArchiveState(state TicketArchiveState) bool {
	return state == TicketArchiveStateActive ||
		state == TicketArchiveStateArchived
}

func validateTicketAliases(projection TicketProjection) error {
	if err := validateAliases(projection.Aliases); err != nil {
		return err
	}
	seen := map[string]struct{}{
		projection.VisibleKey: {},
		projection.Path:       {},
	}
	if projection.VisibleKey == projection.Path {
		return errors.New("ticket visible key and path must differ")
	}
	for _, alias := range projection.Aliases {
		if _, ok := seen[alias]; ok {
			return fmt.Errorf(
				"ticket alias %q duplicates the current identity",
				alias,
			)
		}
		seen[alias] = struct{}{}
	}
	return nil
}

func validateArtifactProjections(projection TicketProjection) error {
	roles := make(map[string]struct{}, len(projection.Artifacts))
	paths := make(map[string]struct{}, len(projection.Artifacts))
	for _, artifact := range projection.Artifacts {
		switch {
		case artifact.TicketID != "" &&
			artifact.TicketID != projection.ID:
			return fmt.Errorf(
				"artifact ticket id %q does not match ticket %q",
				artifact.TicketID,
				projection.ID,
			)
		case strings.TrimSpace(artifact.Role) == "":
			return errors.New("artifact semantic role is required")
		case strings.TrimSpace(artifact.Path) == "":
			return fmt.Errorf(
				"artifact %q path is required",
				artifact.Role,
			)
		case strings.TrimSpace(artifact.Authority) == "":
			return fmt.Errorf(
				"artifact %q authority is required",
				artifact.Role,
			)
		case strings.TrimSpace(artifact.SearchPolicy) == "":
			return fmt.Errorf(
				"artifact %q search policy is required",
				artifact.Role,
			)
		}
		if _, ok := roles[artifact.Role]; ok {
			return fmt.Errorf(
				"duplicate artifact semantic role %q",
				artifact.Role,
			)
		}
		roles[artifact.Role] = struct{}{}
		if _, ok := paths[artifact.Path]; ok {
			return fmt.Errorf("duplicate artifact path %q", artifact.Path)
		}
		paths[artifact.Path] = struct{}{}
	}
	return nil
}

func validateSourceProjections(projection TicketProjection) error {
	sources := make(map[string]struct{}, len(projection.Sources))
	for _, source := range projection.Sources {
		switch {
		case source.TicketID != "" && source.TicketID != projection.ID:
			return fmt.Errorf(
				"source ticket id %q does not match ticket %q",
				source.TicketID,
				projection.ID,
			)
		case strings.TrimSpace(source.CanonicalSource) == "":
			return errors.New("canonical source is required")
		case !validSourceAuthorityClass(source.AuthorityClass):
			return fmt.Errorf(
				"unsupported source authority class %q",
				source.AuthorityClass,
			)
		case strings.TrimSpace(source.ContentHash) == "":
			return fmt.Errorf(
				"source %q content hash is required",
				source.CanonicalSource,
			)
		case source.ObservedAt.IsZero():
			return fmt.Errorf(
				"source %q observed_at is required",
				source.CanonicalSource,
			)
		case source.TTL < 0:
			return fmt.Errorf(
				"source %q ttl cannot be negative",
				source.CanonicalSource,
			)
		case strings.TrimSpace(source.RefreshPolicy) == "":
			return fmt.Errorf(
				"source %q refresh policy is required",
				source.CanonicalSource,
			)
		case !validSourceState(source.State):
			return fmt.Errorf(
				"unsupported source state %q",
				source.State,
			)
		}
		if _, ok := sources[source.CanonicalSource]; ok {
			return fmt.Errorf(
				"duplicate canonical source %q",
				source.CanonicalSource,
			)
		}
		sources[source.CanonicalSource] = struct{}{}
	}

	dependencies := make(map[string]struct{}, len(projection.Dependencies))
	for _, dependency := range projection.Dependencies {
		switch {
		case dependency.TicketID != "" &&
			dependency.TicketID != projection.ID:
			return fmt.Errorf(
				"dependency ticket id %q does not match ticket %q",
				dependency.TicketID,
				projection.ID,
			)
		case strings.TrimSpace(dependency.CanonicalSource) == "":
			return errors.New("dependency canonical source is required")
		case strings.TrimSpace(dependency.DependsOnSource) == "":
			return errors.New("dependency source target is required")
		case dependency.CanonicalSource == dependency.DependsOnSource:
			return fmt.Errorf(
				"source %q cannot depend on itself",
				dependency.CanonicalSource,
			)
		case strings.TrimSpace(dependency.Type) == "":
			return fmt.Errorf(
				"dependency %q -> %q type is required",
				dependency.CanonicalSource,
				dependency.DependsOnSource,
			)
		}
		if _, ok := sources[dependency.CanonicalSource]; !ok {
			return fmt.Errorf(
				"dependency source %q is not observed",
				dependency.CanonicalSource,
			)
		}
		if _, ok := sources[dependency.DependsOnSource]; !ok {
			return fmt.Errorf(
				"dependency target %q is not observed",
				dependency.DependsOnSource,
			)
		}
		key := dependency.CanonicalSource + "\x00" +
			dependency.DependsOnSource + "\x00" + dependency.Type
		if _, ok := dependencies[key]; ok {
			return fmt.Errorf(
				"duplicate dependency %q -> %q (%s)",
				dependency.CanonicalSource,
				dependency.DependsOnSource,
				dependency.Type,
			)
		}
		dependencies[key] = struct{}{}
	}
	return nil
}

func validSourceAuthorityClass(value SourceAuthorityClass) bool {
	switch value {
	case SourceAuthoritySource,
		SourceAuthorityProjection,
		SourceAuthorityCache,
		SourceAuthorityInference,
		SourceAuthorityExternalMirror:
		return true
	default:
		return false
	}
}

func validSourceState(value SourceState) bool {
	switch value {
	case SourceStateCurrent,
		SourceStateStale,
		SourceStateIncorrect,
		SourceStateContested,
		SourceStateSuperseded,
		SourceStateIgnored:
		return true
	default:
		return false
	}
}

func verifyTicketScope(
	ctx context.Context,
	transaction *sql.Tx,
	scope TicketScope,
) error {
	var organizationID string
	if err := transaction.QueryRowContext(
		ctx,
		`SELECT organization_id
		   FROM organization_projection
		  WHERE organization_id = ?`,
		scope.OrganizationID,
	).Scan(&organizationID); err != nil {
		return fmt.Errorf(
			"verify ticket organization %q: %w",
			scope.OrganizationID,
			err,
		)
	}
	if scope.RepositoryID == "" {
		return nil
	}

	var repositoryOrganizationID string
	if err := transaction.QueryRowContext(
		ctx,
		`SELECT organization_id
		   FROM repository_projection
		  WHERE repository_id = ?`,
		scope.RepositoryID,
	).Scan(&repositoryOrganizationID); err != nil {
		return fmt.Errorf(
			"verify ticket repository %q: %w",
			scope.RepositoryID,
			err,
		)
	}
	if repositoryOrganizationID != scope.OrganizationID {
		return fmt.Errorf(
			"ticket repository %q belongs to organization %q, not %q",
			scope.RepositoryID,
			repositoryOrganizationID,
			scope.OrganizationID,
		)
	}
	return nil
}

func checkTicketOwnership(
	ctx context.Context,
	transaction *sql.Tx,
	projection TicketProjection,
) error {
	var organizationID string
	var repositoryID string
	err := transaction.QueryRowContext(
		ctx,
		`SELECT organization_id, COALESCE(repository_id, '')
		   FROM ticket_projection
		  WHERE ticket_id = ?`,
		projection.ID,
	).Scan(&organizationID, &repositoryID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf(
			"read ticket %q ownership: %w",
			projection.ID,
			err,
		)
	case organizationID != projection.OrganizationID ||
		repositoryID != projection.RepositoryID:
		return fmt.Errorf(
			"ticket %q ownership is immutable: have %q/%q, got %q/%q",
			projection.ID,
			organizationID,
			repositoryID,
			projection.OrganizationID,
			projection.RepositoryID,
		)
	default:
		return nil
	}
}

func checkTicketSelectorConflicts(
	ctx context.Context,
	transaction *sql.Tx,
	projection TicketProjection,
) error {
	var pathOwner string
	err := transaction.QueryRowContext(
		ctx,
		`SELECT ticket_id
		   FROM ticket_projection
		  WHERE path_uri = ? AND ticket_id <> ?`,
		projection.Path,
		projection.ID,
	).Scan(&pathOwner)
	if err == nil {
		return fmt.Errorf(
			"ticket path %q is owned by %q",
			projection.Path,
			pathOwner,
		)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check ticket path collision: %w", err)
	}

	query := `SELECT ticket_projection.ticket_id,
	                ticket_projection.visible_key,
	                ticket_projection.path_uri,
	                ticket_aliases.alias
	           FROM ticket_projection
	           LEFT JOIN ticket_aliases
	             ON ticket_aliases.ticket_id = ticket_projection.ticket_id
	          WHERE ticket_projection.organization_id = ?
	            AND ticket_projection.ticket_id <> ?`
	arguments := []any{projection.OrganizationID, projection.ID}
	if projection.RepositoryID == "" {
		query += " AND ticket_projection.repository_id IS NULL"
	} else {
		query += " AND ticket_projection.repository_id = ?"
		arguments = append(arguments, projection.RepositoryID)
	}

	rows, err := transaction.QueryContext(ctx, query, arguments...)
	if err != nil {
		return fmt.Errorf("check ticket selector collisions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	owners := make(map[string]string)
	for rows.Next() {
		var ticketID string
		var visibleKey string
		var path string
		var alias sql.NullString
		if err := rows.Scan(
			&ticketID,
			&visibleKey,
			&path,
			&alias,
		); err != nil {
			return fmt.Errorf("scan ticket selector collision: %w", err)
		}
		owners[ticketID] = ticketID
		owners[visibleKey] = ticketID
		owners[path] = ticketID
		if alias.Valid {
			owners[alias.String] = ticketID
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate ticket selector collisions: %w", err)
	}

	candidates := append(
		[]string{projection.ID, projection.VisibleKey, projection.Path},
		projection.Aliases...,
	)
	for _, candidate := range candidates {
		if owner, ok := owners[candidate]; ok {
			return fmt.Errorf(
				"ticket selector %q is owned by %q",
				candidate,
				owner,
			)
		}
	}
	return nil
}
