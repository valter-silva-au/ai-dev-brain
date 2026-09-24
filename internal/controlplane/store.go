package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	ApplicationID        = 0x41494442
	CurrentSchemaVersion = 3
)

var (
	ErrSchemaTooNew    = errors.New("control-plane schema is newer than this binary")
	ErrSchemaOutdated  = errors.New("control-plane schema is older than this binary")
	ErrForeignDatabase = errors.New("database is not an AI Dev Brain control plane")
	ErrMigrationDrift  = errors.New("control-plane migration drift")
)

type Store struct {
	db *sql.DB
}

type Operation struct {
	ID        string
	Kind      string
	Status    string
	UpdatedAt time.Time
}

type CheckResult struct {
	Integrity            string
	SchemaVersion        int
	IncompleteOperations []Operation
}

type WorkspaceProjection struct {
	ID           string
	Root         string
	ManifestHash string
	ObservedAt   time.Time
}

func Open(ctx context.Context, path string) (*Store, error) {
	return openWithMigrations(ctx, path, currentMigrations)
}

func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("control-plane path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect control plane %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("control plane %q is not a regular file", path)
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve control-plane path %q: %w", path, err)
	}
	hasSidecars, err := sqliteSidecarsExist(absolute)
	if err != nil {
		return nil, err
	}
	database, err := sql.Open(
		"sqlite",
		readOnlyDSN(
			absolute,
			filepath.VolumeName(absolute) != "",
			hasSidecars,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("open read-only control plane %q: %w", path, err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	store := &Store{db: database}
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("connect read-only control plane %q: %w", path, err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("configure read-only control plane: %w", err)
	}

	applicationID, err := store.applicationID(ctx)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	if applicationID != ApplicationID {
		_ = database.Close()
		return nil, fmt.Errorf(
			"%w: application_id=%#x",
			ErrForeignDatabase,
			applicationID,
		)
	}
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	switch {
	case version > CurrentSchemaVersion:
		_ = database.Close()
		return nil, fmt.Errorf(
			"%w: database=%d binary=%d",
			ErrSchemaTooNew,
			version,
			CurrentSchemaVersion,
		)
	case version < CurrentSchemaVersion:
		_ = database.Close()
		return nil, fmt.Errorf(
			"%w: database=%d binary=%d",
			ErrSchemaOutdated,
			version,
			CurrentSchemaVersion,
		)
	}
	if err := store.verifyMigrations(ctx, currentMigrations); err != nil {
		_ = database.Close()
		return nil, err
	}

	return store, nil
}

func readOnlyURIPath(path string, hasVolume bool) string {
	normalized := path
	if hasVolume {
		normalized = strings.ReplaceAll(normalized, `\`, "/")
		if !strings.HasPrefix(normalized, "/") {
			normalized = "/" + normalized
		}
	}
	return normalized
}

func readOnlyDSN(path string, hasVolume bool, hasSidecars bool) string {
	query := url.Values{"mode": []string{"ro"}}
	if !hasSidecars {
		query.Set("immutable", "1")
	}
	location := url.URL{
		Scheme:   "file",
		Path:     readOnlyURIPath(path, hasVolume),
		RawQuery: query.Encode(),
	}
	return location.String()
}

func sqliteSidecarsExist(path string) (bool, error) {
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := path + suffix
		if _, err := os.Stat(sidecar); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf(
				"inspect control-plane sidecar %q: %w",
				sidecar,
				err,
			)
		}
	}
	return false, nil
}

func openWithMigrations(
	ctx context.Context,
	path string,
	migrations []migration,
) (*Store, error) {
	if path == "" {
		return nil, errors.New("control-plane path is required")
	}
	for index, item := range migrations {
		if item.Version != index+1 {
			return nil, fmt.Errorf(
				"control-plane migrations must be consecutive: index %d has version %d",
				index,
				item.Version,
			)
		}
		if item.Name == "" || item.SQL == "" {
			return nil, fmt.Errorf(
				"control-plane migration %d requires name and SQL",
				item.Version,
			)
		}
	}

	database, err := sql.Open("sqlite", filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open control plane %q: %w", path, err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	store := &Store{db: database}
	if _, err := store.validateDatabaseIdentity(
		ctx,
		len(migrations),
	); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := store.configure(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := store.migrate(ctx, migrations); err != nil {
		_ = database.Close()
		return nil, err
	}

	return store, nil
}

func (store *Store) configure(ctx context.Context) error {
	if err := store.db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect control plane: %w", err)
	}

	var journalMode string
	if err := store.db.QueryRowContext(
		ctx,
		"PRAGMA journal_mode = WAL",
	).Scan(&journalMode); err != nil {
		return fmt.Errorf("enable control-plane WAL: %w", err)
	}
	if journalMode != "wal" {
		return fmt.Errorf("control-plane journal mode = %q, want wal", journalMode)
	}

	for _, statement := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = FULL",
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure control plane with %q: %w", statement, err)
		}
	}

	return nil
}

func (store *Store) migrate(
	ctx context.Context,
	migrations []migration,
) error {
	version, err := store.validateDatabaseIdentity(ctx, len(migrations))
	if err != nil {
		return err
	}

	if version > 0 {
		if err := store.verifyMigrations(ctx, migrations[:version]); err != nil {
			return err
		}
	}

	for _, item := range migrations {
		if item.Version <= version {
			continue
		}
		if err := store.applyMigration(ctx, item); err != nil {
			return err
		}
	}

	return store.verifyMigrations(ctx, migrations)
}

func (store *Store) validateDatabaseIdentity(
	ctx context.Context,
	currentVersion int,
) (int, error) {
	applicationID, err := store.applicationID(ctx)
	if err != nil {
		return 0, err
	}
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		return 0, err
	}
	if version > currentVersion {
		return 0, fmt.Errorf(
			"%w: database=%d binary=%d",
			ErrSchemaTooNew,
			version,
			currentVersion,
		)
	}
	if applicationID != 0 && applicationID != ApplicationID {
		return 0, fmt.Errorf(
			"%w: application_id=%#x",
			ErrForeignDatabase,
			applicationID,
		)
	}
	if version > 0 && applicationID != ApplicationID {
		return 0, fmt.Errorf(
			"%w: schema version %d has application_id=%#x",
			ErrForeignDatabase,
			version,
			applicationID,
		)
	}
	return version, nil
}

func (store *Store) applyMigration(
	ctx context.Context,
	item migration,
) (err error) {
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf(
			"begin control-plane migration %d: %w",
			item.Version,
			err,
		)
	}
	committed := false
	defer unwindTransaction(
		transaction,
		&committed,
		fmt.Sprintf("control-plane migration %d", item.Version),
		&err,
	)

	if _, err := transaction.ExecContext(ctx, item.SQL); err != nil {
		return fmt.Errorf(
			"apply control-plane migration %d %q: %w",
			item.Version,
			item.Name,
			err,
		)
	}
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (
			version, name, checksum, applied_at
		) VALUES (?, ?, ?, ?)`,
		item.Version,
		item.Name,
		item.checksum(),
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf(
			"record control-plane migration %d: %w",
			item.Version,
			err,
		)
	}
	if item.Version == 1 {
		if _, err := transaction.ExecContext(
			ctx,
			fmt.Sprintf("PRAGMA application_id = %d", ApplicationID),
		); err != nil {
			return fmt.Errorf("set control-plane application id: %w", err)
		}
	}
	if _, err := transaction.ExecContext(
		ctx,
		fmt.Sprintf("PRAGMA user_version = %d", item.Version),
	); err != nil {
		return fmt.Errorf(
			"set control-plane schema version %d: %w",
			item.Version,
			err,
		)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf(
			"commit control-plane migration %d: %w",
			item.Version,
			err,
		)
	}
	committed = true

	return nil
}

func (store *Store) verifyMigrations(
	ctx context.Context,
	migrations []migration,
) error {
	if len(migrations) == 0 {
		return nil
	}

	rows, err := store.db.QueryContext(
		ctx,
		`SELECT version, name, checksum
		 FROM schema_migrations
		 ORDER BY version`,
	)
	if err != nil {
		return fmt.Errorf("read control-plane migrations: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	index := 0
	for rows.Next() {
		if index >= len(migrations) {
			return fmt.Errorf("%w: unexpected migration row", ErrMigrationDrift)
		}

		var version int
		var name string
		var checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return fmt.Errorf("scan control-plane migration: %w", err)
		}

		expected := migrations[index]
		if version != expected.Version ||
			name != expected.Name ||
			checksum != expected.checksum() {
			return fmt.Errorf(
				"%w: version %d does not match embedded migration",
				ErrMigrationDrift,
				expected.Version,
			)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate control-plane migrations: %w", err)
	}
	if index != len(migrations) {
		return fmt.Errorf(
			"%w: recorded=%d embedded=%d",
			ErrMigrationDrift,
			index,
			len(migrations),
		)
	}

	return nil
}

func (store *Store) applicationID(ctx context.Context) (int, error) {
	var applicationID int
	if err := store.db.QueryRowContext(
		ctx,
		"PRAGMA application_id",
	).Scan(&applicationID); err != nil {
		return 0, fmt.Errorf("read control-plane application id: %w", err)
	}
	return applicationID, nil
}

func (store *Store) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := store.db.QueryRowContext(
		ctx,
		"PRAGMA user_version",
	).Scan(&version); err != nil {
		return 0, fmt.Errorf("read control-plane schema version: %w", err)
	}
	return version, nil
}

func (store *Store) Check(ctx context.Context) (CheckResult, error) {
	var integrity string
	if err := store.db.QueryRowContext(
		ctx,
		"PRAGMA quick_check",
	).Scan(&integrity); err != nil {
		return CheckResult{}, fmt.Errorf("check control-plane integrity: %w", err)
	}

	version, err := store.SchemaVersion(ctx)
	if err != nil {
		return CheckResult{}, err
	}

	rows, err := store.db.QueryContext(
		ctx,
		`SELECT operation_id, kind, status, updated_at
		 FROM operations
		 WHERE status NOT IN ('committed', 'failed')
		 ORDER BY updated_at, operation_id`,
	)
	if err != nil {
		return CheckResult{}, fmt.Errorf(
			"read incomplete control-plane operations: %w",
			err,
		)
	}
	defer func() {
		_ = rows.Close()
	}()

	operations := make([]Operation, 0)
	for rows.Next() {
		var operation Operation
		var updatedAt string
		if err := rows.Scan(
			&operation.ID,
			&operation.Kind,
			&operation.Status,
			&updatedAt,
		); err != nil {
			return CheckResult{}, fmt.Errorf(
				"scan incomplete control-plane operation: %w",
				err,
			)
		}
		parsed, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return CheckResult{}, fmt.Errorf(
				"parse operation %q updated_at: %w",
				operation.ID,
				err,
			)
		}
		operation.UpdatedAt = parsed
		operations = append(operations, operation)
	}
	if err := rows.Err(); err != nil {
		return CheckResult{}, fmt.Errorf(
			"iterate incomplete control-plane operations: %w",
			err,
		)
	}

	return CheckResult{
		Integrity:            integrity,
		SchemaVersion:        version,
		IncompleteOperations: operations,
	}, nil
}

func (store *Store) ObserveWorkspace(
	ctx context.Context,
	projection WorkspaceProjection,
) error {
	if projection.ID == "" {
		return errors.New("workspace projection id is required")
	}
	if projection.Root == "" {
		return errors.New("workspace projection root is required")
	}
	if projection.ManifestHash == "" {
		return errors.New("workspace projection manifest hash is required")
	}
	if projection.ObservedAt.IsZero() {
		return errors.New("workspace projection observed_at is required")
	}

	if _, err := store.db.ExecContext(
		ctx,
		`INSERT INTO workspace_projection (
			singleton, workspace_id, root_uri, manifest_hash, observed_at
		) VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(singleton) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			root_uri = excluded.root_uri,
			manifest_hash = excluded.manifest_hash,
			observed_at = excluded.observed_at`,
		projection.ID,
		projection.Root,
		projection.ManifestHash,
		projection.ObservedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record workspace projection: %w", err)
	}
	return nil
}

func (store *Store) Workspace(
	ctx context.Context,
) (WorkspaceProjection, error) {
	var projection WorkspaceProjection
	var observedAt string
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT workspace_id, root_uri, manifest_hash, observed_at
		 FROM workspace_projection
		 WHERE singleton = 1`,
	).Scan(
		&projection.ID,
		&projection.Root,
		&projection.ManifestHash,
		&observedAt,
	); err != nil {
		return WorkspaceProjection{}, fmt.Errorf(
			"read workspace projection: %w",
			err,
		)
	}

	parsed, err := time.Parse(time.RFC3339Nano, observedAt)
	if err != nil {
		return WorkspaceProjection{}, fmt.Errorf(
			"parse workspace projection observed_at: %w",
			err,
		)
	}
	projection.ObservedAt = parsed
	return projection, nil
}

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	if err := store.db.Close(); err != nil {
		return fmt.Errorf("close control plane: %w", err)
	}
	return nil
}
