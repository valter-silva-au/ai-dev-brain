package controlplane

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ticketChildTables is the closed allowlist of tables whose rows a ticket
// projection owns and replaces wholesale. It exists as a named package-level
// constant list, rather than inline, because it is the *only* thing that ever
// reaches the DELETE statement's table position: a table name cannot be a bound
// parameter in SQL, so this is the one spelling of the statement that has to be
// assembled by concatenation, and keeping the identifiers here makes the closed
// set auditable in one place. Nothing derived from a TicketProjection, a
// selector, or any other caller input is interpolated — only the ticket id, and
// that goes through a ? placeholder.
var ticketChildTables = []string{
	"source_dependencies",
	"source_observations",
	"ticket_artifacts",
	"ticket_aliases",
}

func replaceTicketChildren(
	ctx context.Context,
	transaction *sql.Tx,
	projection TicketProjection,
) error {
	for _, table := range ticketChildTables {
		if _, err := transaction.ExecContext(
			ctx,
			// #nosec G202 -- table is an element of ticketChildTables, a
			// package-level list of literal identifiers; no caller input reaches
			// the concatenation. The bound value (the ticket id) uses ?.
			"DELETE FROM "+table+" WHERE ticket_id = ?",
			projection.ID,
		); err != nil {
			return fmt.Errorf(
				"replace ticket %q %s: %w",
				projection.ID,
				table,
				err,
			)
		}
	}

	for ordinal, alias := range projection.Aliases {
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO ticket_aliases (
				ticket_id, alias, ordinal
			) VALUES (?, ?, ?)`,
			projection.ID,
			alias,
			ordinal,
		); err != nil {
			return fmt.Errorf(
				"record ticket %q alias %q: %w",
				projection.ID,
				alias,
				err,
			)
		}
	}

	for _, artifact := range projection.Artifacts {
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO ticket_artifacts (
				ticket_id, semantic_role, path_uri, authority,
				search_policy, rendered_hash, source_hash
			) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			projection.ID,
			artifact.Role,
			artifact.Path,
			artifact.Authority,
			artifact.SearchPolicy,
			artifact.RenderedHash,
			artifact.SourceHash,
		); err != nil {
			return fmt.Errorf(
				"record ticket %q artifact %q: %w",
				projection.ID,
				artifact.Role,
				err,
			)
		}
	}

	for _, source := range projection.Sources {
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO source_observations (
				ticket_id, canonical_source, authority_class, content_hash,
				observed_at, source_updated_at, ttl_nanoseconds,
				refresh_policy, state, last_success_at, last_error
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			projection.ID,
			source.CanonicalSource,
			source.AuthorityClass,
			source.ContentHash,
			formatProjectionTime(source.ObservedAt),
			optionalProjectionTime(source.SourceUpdatedAt),
			int64(source.TTL),
			source.RefreshPolicy,
			source.State,
			optionalProjectionTime(source.LastSuccessAt),
			source.LastError,
		); err != nil {
			return fmt.Errorf(
				"record ticket %q source %q: %w",
				projection.ID,
				source.CanonicalSource,
				err,
			)
		}
	}

	for _, dependency := range projection.Dependencies {
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO source_dependencies (
				ticket_id, canonical_source, depends_on_source,
				dependency_type
			) VALUES (?, ?, ?, ?)`,
			projection.ID,
			dependency.CanonicalSource,
			dependency.DependsOnSource,
			dependency.Type,
		); err != nil {
			return fmt.Errorf(
				"record ticket %q dependency %q -> %q: %w",
				projection.ID,
				dependency.CanonicalSource,
				dependency.DependsOnSource,
				err,
			)
		}
	}
	return nil
}

func optionalProjectionTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return formatProjectionTime(value)
}

func populateTicketChildren(
	ctx context.Context,
	queryer ticketQueryer,
	projection *TicketProjection,
) error {
	var err error
	projection.Aliases, err = ticketAliases(ctx, queryer, projection.ID)
	if err != nil {
		return err
	}
	projection.Artifacts, err = ticketArtifacts(ctx, queryer, projection.ID)
	if err != nil {
		return err
	}
	projection.Sources, err = ticketSources(ctx, queryer, projection.ID)
	if err != nil {
		return err
	}
	projection.Dependencies, err = ticketDependencies(
		ctx,
		queryer,
		projection.ID,
	)
	return err
}

func ticketAliases(
	ctx context.Context,
	queryer ticketQueryer,
	ticketID string,
) ([]string, error) {
	rows, err := queryer.QueryContext(
		ctx,
		`SELECT alias
		   FROM ticket_aliases
		  WHERE ticket_id = ?
		  ORDER BY ordinal`,
		ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("read ticket %q aliases: %w", ticketID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	aliases := make([]string, 0)
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, fmt.Errorf("scan ticket %q alias: %w", ticketID, err)
		}
		aliases = append(aliases, alias)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticket %q aliases: %w", ticketID, err)
	}
	return aliases, nil
}

func ticketArtifacts(
	ctx context.Context,
	queryer ticketQueryer,
	ticketID string,
) ([]ArtifactProjection, error) {
	rows, err := queryer.QueryContext(
		ctx,
		`SELECT semantic_role, path_uri, authority, search_policy,
		        rendered_hash, source_hash
		   FROM ticket_artifacts
		  WHERE ticket_id = ?
		  ORDER BY semantic_role, path_uri`,
		ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("read ticket %q artifacts: %w", ticketID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	artifacts := make([]ArtifactProjection, 0)
	for rows.Next() {
		artifact := ArtifactProjection{TicketID: ticketID}
		if err := rows.Scan(
			&artifact.Role,
			&artifact.Path,
			&artifact.Authority,
			&artifact.SearchPolicy,
			&artifact.RenderedHash,
			&artifact.SourceHash,
		); err != nil {
			return nil, fmt.Errorf(
				"scan ticket %q artifact: %w",
				ticketID,
				err,
			)
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticket %q artifacts: %w", ticketID, err)
	}
	return artifacts, nil
}

func ticketSources(
	ctx context.Context,
	queryer ticketQueryer,
	ticketID string,
) ([]SourceObservation, error) {
	rows, err := queryer.QueryContext(
		ctx,
		`SELECT canonical_source, authority_class, content_hash, observed_at,
		        source_updated_at, ttl_nanoseconds, refresh_policy, state,
		        last_success_at, last_error
		   FROM source_observations
		  WHERE ticket_id = ?
		  ORDER BY canonical_source`,
		ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("read ticket %q sources: %w", ticketID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	sources := make([]SourceObservation, 0)
	for rows.Next() {
		source := SourceObservation{TicketID: ticketID}
		var observedAt string
		var sourceUpdatedAt sql.NullString
		var lastSuccessAt sql.NullString
		var ttl int64
		if err := rows.Scan(
			&source.CanonicalSource,
			&source.AuthorityClass,
			&source.ContentHash,
			&observedAt,
			&sourceUpdatedAt,
			&ttl,
			&source.RefreshPolicy,
			&source.State,
			&lastSuccessAt,
			&source.LastError,
		); err != nil {
			return nil, fmt.Errorf(
				"scan ticket %q source: %w",
				ticketID,
				err,
			)
		}
		parsed, err := parseProjectionTime(
			observedAt,
			fmt.Sprintf(
				"ticket %q source %q observed_at",
				ticketID,
				source.CanonicalSource,
			),
		)
		if err != nil {
			return nil, err
		}
		source.ObservedAt = parsed
		if sourceUpdatedAt.Valid {
			source.SourceUpdatedAt, err = parseProjectionTime(
				sourceUpdatedAt.String,
				fmt.Sprintf(
					"ticket %q source %q source_updated_at",
					ticketID,
					source.CanonicalSource,
				),
			)
			if err != nil {
				return nil, err
			}
		}
		if lastSuccessAt.Valid {
			source.LastSuccessAt, err = parseProjectionTime(
				lastSuccessAt.String,
				fmt.Sprintf(
					"ticket %q source %q last_success_at",
					ticketID,
					source.CanonicalSource,
				),
			)
			if err != nil {
				return nil, err
			}
		}
		source.TTL = time.Duration(ttl)
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticket %q sources: %w", ticketID, err)
	}
	return sources, nil
}

func ticketDependencies(
	ctx context.Context,
	queryer ticketQueryer,
	ticketID string,
) ([]SourceDependency, error) {
	rows, err := queryer.QueryContext(
		ctx,
		`SELECT canonical_source, depends_on_source, dependency_type
		   FROM source_dependencies
		  WHERE ticket_id = ?
		  ORDER BY canonical_source, depends_on_source, dependency_type`,
		ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"read ticket %q dependencies: %w",
			ticketID,
			err,
		)
	}
	defer func() {
		_ = rows.Close()
	}()

	dependencies := make([]SourceDependency, 0)
	for rows.Next() {
		dependency := SourceDependency{TicketID: ticketID}
		if err := rows.Scan(
			&dependency.CanonicalSource,
			&dependency.DependsOnSource,
			&dependency.Type,
		); err != nil {
			return nil, fmt.Errorf(
				"scan ticket %q dependency: %w",
				ticketID,
				err,
			)
		}
		dependencies = append(dependencies, dependency)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate ticket %q dependencies: %w",
			ticketID,
			err,
		)
	}
	return dependencies, nil
}
