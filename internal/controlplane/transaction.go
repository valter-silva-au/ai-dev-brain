package controlplane

import (
	"database/sql"
	"errors"
	"fmt"
)

// unwindTransaction rolls transaction back unless the operation committed, and
// folds a rollback failure into failure rather than discarding it.
//
// A failed rollback is worth reporting. database/sql returns the connection to
// the pool whether or not the driver's ROLLBACK took, so a SQLite connection
// whose rollback failed can still hold the write transaction — and the next
// statement handed that connection sees uncommitted projection state. The error
// that caused the unwind is still the primary cause, so it stays first and stays
// unwrappable; the rollback failure rides alongside it, because the returned
// error is the only channel a Store has to a caller.
//
// sql.ErrTxDone is ignored: it means only that the transaction had already
// finished, which is not a lost rollback.
//
// Call it deferred, with pointers, so the deferred call observes the final
// values of both the commit flag and the named error return:
//
//	defer unwindTransaction(transaction, &committed, "ticket projection", &err)
func unwindTransaction(
	transaction *sql.Tx,
	committed *bool,
	operation string,
	failure *error,
) {
	if *committed {
		return
	}
	err := transaction.Rollback()
	if err == nil || errors.Is(err, sql.ErrTxDone) {
		return
	}
	if *failure == nil {
		*failure = fmt.Errorf("roll back %s: %w", operation, err)
		return
	}
	// Two %w verbs so both errors stay unwrappable, on one line: errors.Join
	// would separate them with a newline, and a Store error is rendered inline
	// by the CLI.
	*failure = fmt.Errorf(
		"%w (roll back %s: %w)",
		*failure,
		operation,
		err,
	)
}
