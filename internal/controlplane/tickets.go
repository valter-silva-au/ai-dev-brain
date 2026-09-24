package controlplane

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type TicketArchiveState string

const (
	TicketArchiveStateActive   TicketArchiveState = "active"
	TicketArchiveStateArchived TicketArchiveState = "archived"
)

type SourceAuthorityClass string

const (
	SourceAuthoritySource         SourceAuthorityClass = "source"
	SourceAuthorityProjection     SourceAuthorityClass = "projection"
	SourceAuthorityCache          SourceAuthorityClass = "cache"
	SourceAuthorityInference      SourceAuthorityClass = "inference"
	SourceAuthorityExternalMirror SourceAuthorityClass = "external_mirror"
)

type SourceState string

const (
	SourceStateCurrent    SourceState = "current"
	SourceStateStale      SourceState = "stale"
	SourceStateIncorrect  SourceState = "incorrect"
	SourceStateContested  SourceState = "contested"
	SourceStateSuperseded SourceState = "superseded"
	SourceStateIgnored    SourceState = "ignored"
)

type TicketScope struct {
	OrganizationID string
	RepositoryID   string
}

type TicketFilter struct {
	Scope        TicketScope
	Status       string
	Type         string
	Priority     string
	ArchiveState TicketArchiveState
}

type TicketProjection struct {
	ID             string
	OrganizationID string
	RepositoryID   string
	VisibleKey     string
	Path           string
	Status         string
	Type           string
	Priority       string
	ProfileID      string
	ProfileVersion string
	ManifestHash   string
	ArchiveState   TicketArchiveState
	ObservedAt     time.Time
	Aliases        []string
	Artifacts      []ArtifactProjection
	Sources        []SourceObservation
	Dependencies   []SourceDependency
}

type ArtifactProjection struct {
	TicketID     string
	Role         string
	Path         string
	Authority    string
	SearchPolicy string
	RenderedHash string
	SourceHash   string
}

type SourceObservation struct {
	TicketID        string
	CanonicalSource string
	AuthorityClass  SourceAuthorityClass
	ContentHash     string
	ObservedAt      time.Time
	SourceUpdatedAt time.Time
	TTL             time.Duration
	RefreshPolicy   string
	State           SourceState
	LastSuccessAt   time.Time
	LastError       string
}

type SourceDependency struct {
	TicketID        string
	CanonicalSource string
	DependsOnSource string
	Type            string
}

type ticketQueryer interface {
	QueryContext(
		context.Context,
		string,
		...any,
	) (*sql.Rows, error)
	QueryRowContext(
		context.Context,
		string,
		...any,
	) *sql.Row
}

func (store *Store) ObserveTicket(
	ctx context.Context,
	projection TicketProjection,
) (err error) {
	if err = validateTicketProjection(projection); err != nil {
		return fmt.Errorf("validate ticket projection: %w", err)
	}

	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ticket projection: %w", err)
	}
	committed := false
	defer unwindTransaction(
		transaction,
		&committed,
		"ticket projection",
		&err,
	)

	scope := TicketScope{
		OrganizationID: projection.OrganizationID,
		RepositoryID:   projection.RepositoryID,
	}
	if err := verifyTicketScope(ctx, transaction, scope); err != nil {
		return err
	}
	if err := checkTicketOwnership(ctx, transaction, projection); err != nil {
		return err
	}
	if err := checkTicketSelectorConflicts(
		ctx,
		transaction,
		projection,
	); err != nil {
		return err
	}

	var repositoryID any
	if projection.RepositoryID != "" {
		repositoryID = projection.RepositoryID
	}
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO ticket_projection (
			ticket_id, organization_id, repository_id, visible_key, path_uri,
			status, ticket_type, priority, profile_id, profile_version,
			manifest_hash, archive_state, observed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(ticket_id) DO UPDATE SET
			visible_key = excluded.visible_key,
			path_uri = excluded.path_uri,
			status = excluded.status,
			ticket_type = excluded.ticket_type,
			priority = excluded.priority,
			profile_id = excluded.profile_id,
			profile_version = excluded.profile_version,
			manifest_hash = excluded.manifest_hash,
			archive_state = excluded.archive_state,
			observed_at = excluded.observed_at`,
		projection.ID,
		projection.OrganizationID,
		repositoryID,
		projection.VisibleKey,
		projection.Path,
		projection.Status,
		projection.Type,
		projection.Priority,
		projection.ProfileID,
		projection.ProfileVersion,
		projection.ManifestHash,
		projection.ArchiveState,
		formatProjectionTime(projection.ObservedAt),
	); err != nil {
		return fmt.Errorf("record ticket projection %q: %w", projection.ID, err)
	}

	if err := replaceTicketChildren(ctx, transaction, projection); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit ticket projection %q: %w", projection.ID, err)
	}
	committed = true
	return nil
}

func (store *Store) Ticket(
	ctx context.Context,
	scope TicketScope,
	selector string,
) (_ TicketProjection, err error) {
	transaction, err := store.db.BeginTx(
		ctx,
		&sql.TxOptions{ReadOnly: true},
	)
	if err != nil {
		return TicketProjection{}, fmt.Errorf(
			"begin ticket projection read: %w",
			err,
		)
	}
	committed := false
	defer unwindTransaction(
		transaction,
		&committed,
		"ticket projection read",
		&err,
	)

	projection, err := readTicket(ctx, transaction, scope, selector)
	if err != nil {
		return TicketProjection{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TicketProjection{}, fmt.Errorf(
			"commit ticket projection read: %w",
			err,
		)
	}
	committed = true
	return projection, nil
}

// RemoveTicketProjection deletes one ticket projection selected within scope.
func (store *Store) RemoveTicketProjection(
	ctx context.Context,
	scope TicketScope,
	selector string,
) (err error) {
	if err = validateTicketScope(scope); err != nil {
		return err
	}
	if strings.TrimSpace(selector) == "" {
		return fmt.Errorf("ticket selector is required")
	}

	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ticket projection removal: %w", err)
	}
	committed := false
	defer unwindTransaction(
		transaction,
		&committed,
		"ticket projection removal",
		&err,
	)

	ticketID, err := scopedTicketProjectionID(
		ctx,
		transaction,
		scope,
		selector,
	)
	if err != nil {
		return fmt.Errorf("remove ticket projection %q: %w", selector, err)
	}

	result, err := transaction.ExecContext(
		ctx,
		`DELETE FROM ticket_projection
		  WHERE ticket_id = ?
		    AND organization_id = ?
		    AND (
				(? = '' AND repository_id IS NULL)
				OR repository_id = ?
		    )`,
		ticketID,
		scope.OrganizationID,
		scope.RepositoryID,
		scope.RepositoryID,
	)
	if err != nil {
		return fmt.Errorf("remove ticket projection %q: %w", ticketID, err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf(
			"count removed ticket projection %q: %w",
			ticketID,
			err,
		)
	}
	if removed != 1 {
		return fmt.Errorf(
			"remove ticket projection %q: affected %d rows, want 1",
			ticketID,
			removed,
		)
	}
	if err := validateTicketProjectionCleanup(
		ctx,
		transaction,
		ticketID,
	); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf(
			"commit ticket projection removal %q: %w",
			ticketID,
			err,
		)
	}
	committed = true
	return nil
}

func scopedTicketProjectionID(
	ctx context.Context,
	queryer ticketQueryer,
	scope TicketScope,
	selector string,
) (string, error) {
	var ticketID string
	if err := queryer.QueryRowContext(
		ctx,
		`SELECT ticket_id
		   FROM ticket_projection
		  WHERE organization_id = ?
		    AND (
				(? = '' AND repository_id IS NULL)
				OR repository_id = ?
		    )
		    AND (
				ticket_id = ?
				OR visible_key = ?
				OR path_uri = ?
				OR EXISTS (
					SELECT 1
					  FROM ticket_aliases
					 WHERE ticket_aliases.ticket_id =
					       ticket_projection.ticket_id
					   AND alias = ?
				)
		    )`,
		scope.OrganizationID,
		scope.RepositoryID,
		scope.RepositoryID,
		selector,
		selector,
		selector,
		selector,
	).Scan(&ticketID); err != nil {
		return "", err
	}
	return ticketID, nil
}

func validateTicketProjectionCleanup(
	ctx context.Context,
	queryer ticketQueryer,
	ticketID string,
) error {
	for _, table := range []string{
		"ticket_aliases",
		"ticket_artifacts",
		"source_observations",
		"source_dependencies",
	} {
		var remaining int
		if err := queryer.QueryRowContext(
			ctx,
			"SELECT COUNT(*) FROM "+table+" WHERE ticket_id = ?",
			ticketID,
		).Scan(&remaining); err != nil {
			return fmt.Errorf(
				"validate ticket projection %q cleanup in %s: %w",
				ticketID,
				table,
				err,
			)
		}
		if remaining != 0 {
			return fmt.Errorf(
				"ticket projection %q cleanup incomplete: "+
					"%d rows remain in %s",
				ticketID,
				remaining,
				table,
			)
		}
	}
	return nil
}

func readTicket(
	ctx context.Context,
	queryer ticketQueryer,
	scope TicketScope,
	selector string,
) (TicketProjection, error) {
	if err := validateTicketScope(scope); err != nil {
		return TicketProjection{}, err
	}
	if strings.TrimSpace(selector) == "" {
		return TicketProjection{}, fmt.Errorf("ticket selector is required")
	}

	var projection TicketProjection
	var repositoryID sql.NullString
	var observedAt string
	if err := queryer.QueryRowContext(
		ctx,
		`SELECT ticket_id, organization_id, repository_id, visible_key,
		        path_uri, status, ticket_type, priority, profile_id,
		        profile_version, manifest_hash, archive_state, observed_at
		   FROM ticket_projection
		  WHERE organization_id = ?
		    AND (
				(? = '' AND repository_id IS NULL)
				OR repository_id = ?
		    )
		    AND (
				ticket_id = ?
				OR visible_key = ?
				OR path_uri = ?
				OR EXISTS (
					SELECT 1
					  FROM ticket_aliases
					 WHERE ticket_aliases.ticket_id =
					       ticket_projection.ticket_id
					   AND alias = ?
				)
		    )`,
		scope.OrganizationID,
		scope.RepositoryID,
		scope.RepositoryID,
		selector,
		selector,
		selector,
		selector,
	).Scan(
		&projection.ID,
		&projection.OrganizationID,
		&repositoryID,
		&projection.VisibleKey,
		&projection.Path,
		&projection.Status,
		&projection.Type,
		&projection.Priority,
		&projection.ProfileID,
		&projection.ProfileVersion,
		&projection.ManifestHash,
		&projection.ArchiveState,
		&observedAt,
	); err != nil {
		return TicketProjection{}, fmt.Errorf(
			"read ticket projection %q: %w",
			selector,
			err,
		)
	}
	if repositoryID.Valid {
		projection.RepositoryID = repositoryID.String
	}
	parsed, err := parseProjectionTime(
		observedAt,
		fmt.Sprintf("ticket %q observed_at", projection.ID),
	)
	if err != nil {
		return TicketProjection{}, err
	}
	projection.ObservedAt = parsed
	if err := populateTicketChildren(ctx, queryer, &projection); err != nil {
		return TicketProjection{}, err
	}
	return projection, nil
}

func (store *Store) Tickets(
	ctx context.Context,
	filter TicketFilter,
) (_ []TicketProjection, err error) {
	transaction, err := store.db.BeginTx(
		ctx,
		&sql.TxOptions{ReadOnly: true},
	)
	if err != nil {
		return nil, fmt.Errorf("begin ticket projection list: %w", err)
	}
	committed := false
	defer unwindTransaction(
		transaction,
		&committed,
		"ticket projection list",
		&err,
	)

	projections, err := listTickets(ctx, transaction, filter)
	if err != nil {
		return nil, err
	}
	if err := transaction.Commit(); err != nil {
		return nil, fmt.Errorf("commit ticket projection list: %w", err)
	}
	committed = true
	return projections, nil
}

func listTickets(
	ctx context.Context,
	queryer ticketQueryer,
	filter TicketFilter,
) ([]TicketProjection, error) {
	if err := validateTicketScope(filter.Scope); err != nil {
		return nil, err
	}
	if filter.ArchiveState != "" &&
		!validTicketArchiveState(filter.ArchiveState) {
		return nil, fmt.Errorf(
			"unsupported ticket archive state filter %q",
			filter.ArchiveState,
		)
	}

	query := `SELECT ticket_id
		   FROM ticket_projection
		  WHERE organization_id = ?`
	arguments := []any{filter.Scope.OrganizationID}
	if filter.Scope.RepositoryID == "" {
		query += " AND repository_id IS NULL"
	} else {
		query += " AND repository_id = ?"
		arguments = append(arguments, filter.Scope.RepositoryID)
	}
	for _, item := range []struct {
		column string
		value  string
	}{
		{column: "status", value: filter.Status},
		{column: "ticket_type", value: filter.Type},
		{column: "priority", value: filter.Priority},
		{column: "archive_state", value: string(filter.ArchiveState)},
	} {
		if item.value != "" {
			query += " AND " + item.column + " = ?"
			arguments = append(arguments, item.value)
		}
	}
	query += " ORDER BY visible_key, ticket_id"

	rows, err := queryer.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list ticket projections: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan ticket projection: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate ticket projections: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close ticket projection rows: %w", err)
	}

	projections := make([]TicketProjection, 0, len(ids))
	for _, id := range ids {
		projection, err := readTicket(ctx, queryer, filter.Scope, id)
		if err != nil {
			return nil, err
		}
		projections = append(projections, projection)
	}
	return projections, nil
}

func formatProjectionTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseProjectionTime(value string, field string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", field, err)
	}
	return parsed.UTC(), nil
}
