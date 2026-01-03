package database

import (
	"database/sql"
	"encoding/json"

	_ "github.com/mattn/go-sqlite3"
	"github.com/jamo/immich-albums/internal/models"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS assets (
		id TEXT PRIMARY KEY,
		device_asset_id TEXT,
		owner_id TEXT,
		device_id TEXT,
		type TEXT,
		original_path TEXT,
		original_filename TEXT,
		file_created_at TIMESTAMP,
		file_modified_at TIMESTAMP,
		local_datetime TIMESTAMP,
		duration TEXT,
		make TEXT,
		model TEXT,
		exif_image_width INTEGER,
		exif_image_height INTEGER,
		orientation TEXT,
		lens_model TEXT,
		f_number REAL,
		focal_length REAL,
		iso INTEGER,
		exposure_time TEXT,
		latitude REAL,
		longitude REAL,
		city TEXT,
		state TEXT,
		country TEXT,
		inferred_latitude REAL,
		inferred_longitude REAL,
		location_confidence REAL,
		location_source TEXT
	);

	CREATE TABLE IF NOT EXISTS devices (
		id TEXT PRIMARY KEY,
		make TEXT,
		model TEXT,
		photo_count INTEGER,
		photographer TEXT
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		start_time TIMESTAMP,
		end_time TIMESTAMP,
		asset_ids TEXT,
		center_lat REAL,
		center_lon REAL,
		radius REAL,
		photographer TEXT
	);

	CREATE TABLE IF NOT EXISTS trips (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		start_time TIMESTAMP,
		end_time TIMESTAMP,
		home_distance REAL,
		total_distance REAL,
		center_lat REAL,
		center_lon REAL,
		asset_ids TEXT,
		photographers TEXT,
		session_count INTEGER,
		album_id TEXT
	);

	CREATE TABLE IF NOT EXISTS home_locations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		latitude REAL,
		longitude REAL,
		radius REAL
	);

	CREATE INDEX IF NOT EXISTS idx_assets_datetime ON assets(local_datetime);
	CREATE INDEX IF NOT EXISTS idx_assets_device ON assets(make, model);
	CREATE INDEX IF NOT EXISTS idx_assets_location ON assets(latitude, longitude);
	`

	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}

	// Migration: Add album_id column to trips table if it doesn't exist
	migrations := []string{
		`ALTER TABLE trips ADD COLUMN album_id TEXT`,
		`ALTER TABLE trips ADD COLUMN exclude_from_album INTEGER DEFAULT 0`,
	}

	for _, migration := range migrations {
		// Ignore errors for migrations (column may already exist)
		db.conn.Exec(migration)
	}

	return nil
}

func (db *DB) StoreAssets(assets []models.Asset) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO assets (
			id, device_asset_id, owner_id, device_id, type, original_path, original_filename,
			file_created_at, file_modified_at, local_datetime, duration,
			make, model, exif_image_width, exif_image_height, orientation, lens_model,
			f_number, focal_length, iso, exposure_time,
			latitude, longitude, city, state, country
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, asset := range assets {
		_, err := stmt.Exec(
			asset.ID, asset.DeviceAssetID, asset.OwnerID, asset.DeviceID, asset.Type,
			asset.OriginalPath, asset.OriginalFileName,
			asset.FileCreatedAt, asset.FileModifiedAt, asset.LocalDateTime, asset.Duration,
			asset.Make, asset.Model, asset.ExifImageWidth, asset.ExifImageHeight,
			asset.Orientation, asset.LensModel, asset.FNumber, asset.FocalLength,
			asset.ISO, asset.ExposureTime,
			asset.Latitude, asset.Longitude, asset.City, asset.State, asset.Country,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) StoreDevices(devices []models.Device) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO devices (id, make, model, photo_count, photographer)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, device := range devices {
		_, err := stmt.Exec(
			device.ID, device.Make, device.Model,
			device.PhotoCount, device.Photographer,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) GetDevices() ([]models.Device, error) {
	rows, err := db.conn.Query(`
		SELECT id, make, model, photo_count, photographer
		FROM devices
		ORDER BY photo_count DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var d models.Device
		var photographer sql.NullString
		err := rows.Scan(&d.ID, &d.Make, &d.Model, &d.PhotoCount, &photographer)
		if err != nil {
			return nil, err
		}
		if photographer.Valid {
			d.Photographer = photographer.String
		}
		devices = append(devices, d)
	}

	return devices, nil
}

func (db *DB) UpdateDevicePhotographer(deviceID, photographer string) error {
	_, err := db.conn.Exec(`
		UPDATE devices SET photographer = ? WHERE id = ?
	`, photographer, deviceID)
	return err
}

func (db *DB) GetAssets() ([]models.Asset, error) {
	rows, err := db.conn.Query(`
		SELECT id, device_asset_id, owner_id, device_id, type, original_path, original_filename,
			file_created_at, file_modified_at, local_datetime, duration,
			make, model, exif_image_width, exif_image_height, orientation, lens_model,
			f_number, focal_length, iso, exposure_time,
			latitude, longitude, city, state, country,
			inferred_latitude, inferred_longitude, location_confidence, location_source
		FROM assets
		ORDER BY local_datetime
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []models.Asset
	for rows.Next() {
		var a models.Asset
		var lat, lon, inferredLat, inferredLon, confidence sql.NullFloat64
		var locationSource sql.NullString

		err := rows.Scan(
			&a.ID, &a.DeviceAssetID, &a.OwnerID, &a.DeviceID, &a.Type,
			&a.OriginalPath, &a.OriginalFileName,
			&a.FileCreatedAt, &a.FileModifiedAt, &a.LocalDateTime, &a.Duration,
			&a.Make, &a.Model, &a.ExifImageWidth, &a.ExifImageHeight,
			&a.Orientation, &a.LensModel, &a.FNumber, &a.FocalLength,
			&a.ISO, &a.ExposureTime,
			&lat, &lon, &a.City, &a.State, &a.Country,
			&inferredLat, &inferredLon, &confidence, &locationSource,
		)
		if err != nil {
			return nil, err
		}

		if lat.Valid {
			a.Latitude = &lat.Float64
		}
		if lon.Valid {
			a.Longitude = &lon.Float64
		}

		assets = append(assets, a)
	}

	return assets, nil
}

func (db *DB) StoreSessions(sessions []models.Session) error {
	// Clear existing sessions first
	if _, err := db.conn.Exec("DELETE FROM sessions"); err != nil {
		return err
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO sessions (start_time, end_time, asset_ids, center_lat, center_lon, radius, photographer)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, session := range sessions {
		assetIDs, _ := json.Marshal(session.AssetIDs)
		_, err := stmt.Exec(
			session.StartTime, session.EndTime, string(assetIDs),
			session.CenterLat, session.CenterLon, session.Radius, session.Photographer,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) GetSessions() ([]models.Session, error) {
	rows, err := db.conn.Query(`
		SELECT id, start_time, end_time, asset_ids, center_lat, center_lon, radius, photographer
		FROM sessions
		ORDER BY start_time
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		var s models.Session
		var assetIDsJSON string
		err := rows.Scan(&s.ID, &s.StartTime, &s.EndTime, &assetIDsJSON,
			&s.CenterLat, &s.CenterLon, &s.Radius, &s.Photographer)
		if err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(assetIDsJSON), &s.AssetIDs)
		sessions = append(sessions, s)
	}

	return sessions, nil
}

func (db *DB) StoreHomeLocation(home models.HomeLocation) error {
	_, err := db.conn.Exec(`
		INSERT INTO home_locations (name, latitude, longitude, radius)
		VALUES (?, ?, ?, ?)
	`, home.Name, home.Latitude, home.Longitude, home.Radius)
	return err
}

func (db *DB) GetHomeLocations() ([]models.HomeLocation, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, latitude, longitude, radius
		FROM home_locations
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var homes []models.HomeLocation
	for rows.Next() {
		var h models.HomeLocation
		if err := rows.Scan(&h.ID, &h.Name, &h.Latitude, &h.Longitude, &h.Radius); err != nil {
			return nil, err
		}
		homes = append(homes, h)
	}

	return homes, nil
}
