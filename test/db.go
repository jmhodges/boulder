package test

import (
	"database/sql"
	"fmt"
	"io"
	"testing"
)

var (
	_ CleanUpDB = &sql.DB{}
)

// CleanUpDB is an interface with only what is needed to delete all
// rows in all tables in a database plus close the database
// connection. It is satisfied by *sql.DB.
type CleanUpDB interface {
	Begin() (*sql.Tx, error)
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)

	io.Closer
}

// ResetSATestDatabase deletes all rows in all tables in the SA DB.
// If fails the tests if that errors and returns a clean up function
// that will delete all rows again and close the database.
// "Tables available" means all tables that can be seen in the MariaDB
// configuration by the database user except for ones that are
// configuration only like goose_db_version (for migrations) or
// the ones describing the internal configuration of the server. To be
// used only in test code.
// func ResetSATestDatabase(t *testing.T) func() {
// 	return resetTestDatabase(t, "sa")
// }

// // ResetPolicyTestDatabase deletes all rows in all tables in the Policy DB. It
// // acts the same as ResetSATestDatabase.
// func ResetPolicyTestDatabase(t *testing.T) func() {
// 	return resetTestDatabase(t, "policy")
// }

func ResetTestDatabase(t *testing.T, db *sql.DB) func() {
	if err := deleteEverythingInAllTables(db); err != nil {
		t.Fatalf("Failed to delete everything: %s", err)
	}
	return func() {
		if err := deleteEverythingInAllTables(db); err != nil {
			db.Close()
			t.Fatalf("Failed to truncate tables after the test: %s", err)
		}
		db.Close()
	}
}

// clearEverythingInAllTables deletes all rows in the tables
// available to the CleanUpDB passed in and resets the autoincrement
// counters. See allTableNamesInDB for what is meant by "all tables
// available". To be used only in test code.
func deleteEverythingInAllTables(db CleanUpDB) error {
	ts, err := allTableNamesInDB(db)
	if err != nil {
		return err
	}

	ts, err = reorderTablesByForeignRefConstraints(db, ts)
	if err != nil {
		return err
	}
	// _, err = db.Exec("lock tables `" + tn + "` write")
	// if err != nil {
	// 	return err // FIXME
	// }
	// _, err = db.Exec("unlock tables")
	// if err != nil {
	// 	return err // FIXME
	// }

	tx, err := db.Begin()
	if err != nil {
		return err // FIXME
	}

	for _, tn := range ts {
		fmt.Println("deleting", tn)
		// 1 = 1 here prevents the MariaDB i_am_a_dummy setting from
		// rejecting the DELETE for not having a WHERE clause.
		_, err = tx.Exec("delete from `" + tn + "` where 1 = 1")
		if err != nil {
			return fmt.Errorf("unable to delete all rows from table %#v: %s", tn, err)
		}

	}
	err = tx.Commit()
	if err != nil {
		return err // FIXME
	}
	return err
}

// allTableNamesInDB returns the names of the tables available to the
// CleanUpDB passed in. "Tables available" means all tables that can
// be seen in the MariaDB configuration by the database user except
// for ones that are configuration only like goose_db_version (for
// migrations) or the ones describing the internal configuration of
// the server. To be used only in test code.
func allTableNamesInDB(db CleanUpDB) ([]string, error) {
	r, err := db.Query("select table_name from information_schema.tables t where t.table_schema = DATABASE() and t.table_name != 'goose_db_version';")
	if err != nil {
		return nil, err
	}
	var ts []string
	for r.Next() {
		tableName := ""
		err = r.Scan(&tableName)
		if err != nil {
			return nil, err
		}
		ts = append(ts, tableName)
	}
	return ts, r.Err()
}

func reorderTablesByForeignRefConstraints(db CleanUpDB, ts []string) ([]string, error) {
	// Find all the foreign key references so that we clear out the
	// table_name before the referenced_table_name (which will throw
	// an error). This code assumes there are no tables that have a
	// constraint on a second table that has a constraint on
	// a third table (which may be the first table).
	r, err := db.Query("select table_name, referenced_table_name from INFORMATION_SCHEMA.KEY_COLUMN_USAGE where referenced_table_name is not null and constraint_schema = DATABASE()")
	if err != nil {
		return nil, fmt.Errorf("unable to gather foreign key constraints to make deletion possible: %s", err)
	}
	forRefs := make(map[string][]string)
	for r.Next() {
		tableName := ""
		refTableName := ""
		err = r.Scan(&tableName, &refTableName)
		if err != nil {
			return nil, err
		}
		fmt.Println("tableName:", tableName, "refTableName:", refTableName)
		refs := forRefs[refTableName]
		refs = append(refs, tableName)
		forRefs[refTableName] = refs
	}
	if r.Err() != nil {
		return nil, fmt.Errorf("unable to finish gathering foreign key constraints: %s", r.Err())
	}
	seen := make(map[string]bool)
	newTS := []string{}
	fmt.Println(forRefs)

	for _, tn := range ts {
		if seen[tn] {
			continue
		}
		refs := forRefs[tn]
		for _, ref := range refs {
			if seen[ref] {
				continue
			}
			fmt.Println("adding ref", ref)
			newTS = append(newTS, ref)
			seen[ref] = true
		}
		fmt.Println("adding tn", tn)
		newTS = append(newTS, tn)
		seen[tn] = true
	}
	fmt.Println("wtf", newTS)
	return newTS, nil
}
