package database

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jamo/immich-albums/internal/models"
)

// StoreTrips saves trips to the database
func (db *DB) StoreTrips(trips []models.Trip) error {
	// Clear existing trips
	if _, err := db.conn.Exec("DELETE FROM trips"); err != nil {
		return err
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO trips (
			name, start_time, end_time, home_distance, total_distance,
			center_lat, center_lon, asset_ids, photographers, session_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, trip := range trips {
		assetIDs, _ := json.Marshal(trip.AssetIDs)

		_, err := stmt.Exec(
			trip.Name,
			trip.StartTime,
			trip.EndTime,
			trip.HomeDistance,
			trip.TotalDistance,
			trip.CenterLat,
			trip.CenterLon,
			string(assetIDs),
			trip.Photographers,
			trip.SessionCount,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetTrips retrieves all trips from the database
func (db *DB) GetTrips() ([]models.Trip, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, start_time, end_time, home_distance, total_distance,
			center_lat, center_lon, asset_ids, photographers, session_count,
			COALESCE(album_id, ''), COALESCE(exclude_from_album, 0)
		FROM trips
		ORDER BY start_time DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trips []models.Trip
	for rows.Next() {
		var trip models.Trip
		var assetIDsJSON string
		var excludeInt int

		err := rows.Scan(
			&trip.ID,
			&trip.Name,
			&trip.StartTime,
			&trip.EndTime,
			&trip.HomeDistance,
			&trip.TotalDistance,
			&trip.CenterLat,
			&trip.CenterLon,
			&assetIDsJSON,
			&trip.Photographers,
			&trip.SessionCount,
			&trip.AlbumID,
			&excludeInt,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(assetIDsJSON), &trip.AssetIDs)
		trip.ExcludeFromAlbum = excludeInt == 1
		trips = append(trips, trip)
	}

	return trips, nil
}

// GetTrip retrieves a single trip by ID
func (db *DB) GetTrip(id int64) (*models.Trip, error) {
	var trip models.Trip
	var assetIDsJSON string
	var excludeInt int

	err := db.conn.QueryRow(`
		SELECT id, name, start_time, end_time, home_distance, total_distance,
			center_lat, center_lon, asset_ids, photographers, session_count,
			COALESCE(album_id, ''), COALESCE(exclude_from_album, 0)
		FROM trips
		WHERE id = ?
	`, id).Scan(
		&trip.ID,
		&trip.Name,
		&trip.StartTime,
		&trip.EndTime,
		&trip.HomeDistance,
		&trip.TotalDistance,
		&trip.CenterLat,
		&trip.CenterLon,
		&assetIDsJSON,
		&trip.Photographers,
		&trip.SessionCount,
		&trip.AlbumID,
		&excludeInt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("trip not found")
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(assetIDsJSON), &trip.AssetIDs)
	trip.ExcludeFromAlbum = excludeInt == 1

	return &trip, nil
}

// UpdateTripAlbumID updates the album_id for a trip
func (db *DB) UpdateTripAlbumID(tripID int64, albumID string) error {
	_, err := db.conn.Exec(`
		UPDATE trips SET album_id = ? WHERE id = ?
	`, albumID, tripID)
	return err
}

// UpdateTrip updates trip details
func (db *DB) UpdateTrip(trip *models.Trip) error {
	assetIDs, _ := json.Marshal(trip.AssetIDs)
	excludeInt := 0
	if trip.ExcludeFromAlbum {
		excludeInt = 1
	}

	_, err := db.conn.Exec(`
		UPDATE trips
		SET name = ?, start_time = ?, end_time = ?,
			home_distance = ?, total_distance = ?,
			center_lat = ?, center_lon = ?,
			asset_ids = ?, photographers = ?, session_count = ?,
			album_id = ?, exclude_from_album = ?
		WHERE id = ?
	`, trip.Name, trip.StartTime, trip.EndTime,
		trip.HomeDistance, trip.TotalDistance,
		trip.CenterLat, trip.CenterLon,
		string(assetIDs), trip.Photographers, trip.SessionCount,
		trip.AlbumID, excludeInt, trip.ID)
	return err
}
