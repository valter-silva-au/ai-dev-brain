package controlplane

import (
	"regexp"
	"testing"
)

// bareSQLIdentifier is deliberately strict: lowercase, digits and underscores
// only. Anything a caller could influence — a quote, a space, a semicolon, a
// parenthesis — fails it.
var bareSQLIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// TestTicketChildTablesAreBareIdentifiers guards the one statement in this
// package that has to be assembled by string concatenation: a table name cannot
// be a bound parameter, so replaceTicketChildren interpolates the loop variable
// into "DELETE FROM <table> WHERE ticket_id = ?".
//
// That is safe only for as long as every element of ticketChildTables is a
// literal identifier. This test is what keeps it that way: an element derived
// from a projection field, a selector, or any other caller input would not be a
// bare identifier, and the whole reason the list was hoisted to a named
// package-level variable is so this assertion has something to point at.
func TestTicketChildTablesAreBareIdentifiers(t *testing.T) {
	t.Parallel()

	if len(ticketChildTables) == 0 {
		t.Fatal("ticketChildTables is empty; the guard would pass vacuously")
	}
	for _, table := range ticketChildTables {
		if !bareSQLIdentifier.MatchString(table) {
			t.Errorf(
				"ticketChildTables entry %q is not a bare SQL identifier; "+
					"it is concatenated into a DELETE statement",
				table,
			)
		}
	}
}
