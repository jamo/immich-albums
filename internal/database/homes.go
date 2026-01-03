package database

// DeleteHomeLocation removes a home location by ID
func (db *DB) DeleteHomeLocation(id int64) error {
	_, err := db.conn.Exec("DELETE FROM home_locations WHERE id = ?", id)
	return err
}
