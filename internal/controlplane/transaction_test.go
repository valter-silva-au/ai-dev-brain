package controlplane

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeRollbackConnector is the smallest driver that can fail a ROLLBACK on
// demand. modernc.org/sqlite cannot be made to fail one deterministically, and a
// failed rollback is the whole case unwindTransaction exists for: the connection
// goes back to the pool either way, so a rollback that did not take is only ever
// visible through the error the Store returns.
type fakeRollbackConnector struct {
	rollbackErr error
	rollbacks   int
}

func (connector *fakeRollbackConnector) Connect(
	context.Context,
) (driver.Conn, error) {
	return &fakeRollbackConn{connector: connector}, nil
}

func (connector *fakeRollbackConnector) Driver() driver.Driver {
	return fakeRollbackDriver{}
}

type fakeRollbackDriver struct{}

func (fakeRollbackDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("fake driver does not open by name")
}

type fakeRollbackConn struct {
	connector *fakeRollbackConnector
}

func (*fakeRollbackConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("fake connection does not prepare statements")
}

func (*fakeRollbackConn) Close() error { return nil }

func (conn *fakeRollbackConn) Begin() (driver.Tx, error) {
	return &fakeRollbackTx{connector: conn.connector}, nil
}

type fakeRollbackTx struct {
	connector *fakeRollbackConnector
}

func (*fakeRollbackTx) Commit() error { return nil }

func (tx *fakeRollbackTx) Rollback() error {
	tx.connector.rollbacks++
	return tx.connector.rollbackErr
}

func beginFakeTx(
	t *testing.T,
	rollbackErr error,
) (*sql.Tx, *fakeRollbackConnector) {
	t.Helper()

	connector := &fakeRollbackConnector{rollbackErr: rollbackErr}
	database := sql.OpenDB(connector)
	t.Cleanup(func() {
		// The fake connection can be closed without a rollback verdict; the
		// pool's own Close is not what is under test here.
		_ = database.Close()
	})
	transaction, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin fake transaction: %v", err)
	}
	return transaction, connector
}

func TestUnwindTransactionReportsRollbackFailureAlongsideCause(t *testing.T) {
	t.Parallel()

	rollbackFailure := errors.New("disk I/O error")
	transaction, connector := beginFakeTx(t, rollbackFailure)

	cause := errors.New("record ticket projection failed")
	committed := false
	failure := cause
	unwindTransaction(transaction, &committed, "ticket projection", &failure)

	if connector.rollbacks != 1 {
		t.Fatalf("rollback calls = %d, want 1", connector.rollbacks)
	}
	if !errors.Is(failure, cause) {
		t.Fatalf("failure %v no longer unwraps to the original cause", failure)
	}
	if !errors.Is(failure, rollbackFailure) {
		t.Fatalf("failure %v does not report the rollback failure", failure)
	}
	if !strings.Contains(failure.Error(), "roll back ticket projection") {
		t.Fatalf("failure %q does not name the rolled-back operation", failure)
	}
	if strings.Contains(failure.Error(), "\n") {
		t.Fatalf("failure %q spans lines; it is rendered on one CLI line", failure)
	}
}

func TestUnwindTransactionReportsRollbackFailureWithoutACause(t *testing.T) {
	t.Parallel()

	rollbackFailure := errors.New("disk I/O error")
	transaction, _ := beginFakeTx(t, rollbackFailure)

	committed := false
	var failure error
	unwindTransaction(transaction, &committed, "ticket projection", &failure)

	if failure == nil {
		t.Fatal("a failed rollback with no prior cause was dropped")
	}
	if !errors.Is(failure, rollbackFailure) {
		t.Fatalf("failure %v does not report the rollback failure", failure)
	}
}

func TestUnwindTransactionIgnoresAlreadyFinishedTransaction(t *testing.T) {
	t.Parallel()

	// A rollback that reports the transaction already finished lost nothing, so
	// it must not manufacture an error on top of a clean unwind path.
	transaction, _ := beginFakeTx(t, sql.ErrTxDone)

	committed := false
	var failure error
	unwindTransaction(transaction, &committed, "ticket projection", &failure)

	if failure != nil {
		t.Fatalf("sql.ErrTxDone surfaced as %v, want no error", failure)
	}
}

func TestUnwindTransactionSkipsCommittedTransaction(t *testing.T) {
	t.Parallel()

	transaction, connector := beginFakeTx(t, io.ErrUnexpectedEOF)

	committed := true
	var failure error
	unwindTransaction(transaction, &committed, "ticket projection", &failure)

	if connector.rollbacks != 0 {
		t.Fatalf("rollback calls = %d, want 0 after a commit", connector.rollbacks)
	}
	if failure != nil {
		t.Fatalf("committed unwind surfaced %v, want no error", failure)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatalf("commit fake transaction: %v", err)
	}
}
