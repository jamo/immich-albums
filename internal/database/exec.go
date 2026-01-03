package database

import "database/sql"

// Exec executes a SQL statement
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.conn.Exec(query, args...)
}
