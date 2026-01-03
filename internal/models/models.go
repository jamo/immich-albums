package models

import "time"

// Asset represents a photo or video from Immich
type Asset struct {
	ID               string    `json:"id"`
	DeviceAssetID    string    `json:"device_asset_id"`
	OwnerID          string    `json:"owner_id"`
	DeviceID         string    `json:"device_id"`
	Type             string    `json:"type"`
	OriginalPath     string    `json:"original_path"`
	OriginalFileName string    `json:"original_file_name"`
	FileCreatedAt    time.Time `json:"file_created_at"`
	FileModifiedAt   time.Time `json:"file_modified_at"`
	LocalDateTime    time.Time `json:"local_date_time"`
	Duration         string    `json:"duration"`

	// EXIF data
	Make            string   `json:"make"`
	Model           string   `json:"model"`
	ExifImageWidth  int      `json:"exif_image_width"`
	ExifImageHeight int      `json:"exif_image_height"`
	Orientation     string   `json:"orientation"`
	LensModel       string   `json:"lens_model"`
	FNumber         float64  `json:"f_number"`
	FocalLength     float64  `json:"focal_length"`
	ISO             int      `json:"iso"`
	ExposureTime    string   `json:"exposure_time"`
	Latitude        *float64 `json:"latitude"`
	Longitude       *float64 `json:"longitude"`
	City            string   `json:"city"`
	State           string   `json:"state"`
	Country         string   `json:"country"`
}

// Device represents a camera or phone
type Device struct {
	ID           string `json:"id"`
	Make         string `json:"make"`
	Model        string `json:"model"`
	PhotoCount   int    `json:"photo_count"`
	Photographer string `json:"photographer"`
}

// Location represents a geographic point with confidence
type Location struct {
	Latitude   float64
	Longitude  float64
	Confidence float64  // 0.0 to 1.0
	Source     string   // "exif", "inferred", "interpolated"
	Timestamp  time.Time
}

// Session represents a group of photos taken in proximity (time and space)
type Session struct {
	ID           int64     `json:"id"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	AssetIDs     []string  `json:"asset_ids"`
	CenterLat    float64   `json:"center_lat"`
	CenterLon    float64   `json:"center_lon"`
	Radius       float64   `json:"radius"` // meters
	Photographer string    `json:"photographer"`
}

// Trip represents a collection of sessions that form a journey
type Trip struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	Sessions         []Session `json:"sessions"`
	HomeDistance     float64   `json:"home_distance"`  // Distance from home in km
	TotalDistance    float64   `json:"total_distance"` // Total travel distance in km
	CenterLat        float64   `json:"center_lat"`     // Trip center point
	CenterLon        float64   `json:"center_lon"`
	AssetIDs         []string  `json:"asset_ids"`
	Photographers    string    `json:"photographers"`
	SessionCount     int       `json:"session_count"`
	AlbumID          string    `json:"album_id"`            // Immich album ID
	ExcludeFromAlbum bool      `json:"exclude_from_album"` // If true, don't create album for this trip
}

// HomeLocation represents a user-defined home base
type HomeLocation struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Radius    float64 `json:"radius"` // meters
}
