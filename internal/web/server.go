package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/jamo/immich-albums/internal/models"
	"github.com/jamo/immich-albums/internal/processor"
)

//go:embed templates/*
var templatesFS embed.FS

type Server struct {
	db           *database.DB
	templates    *template.Template
	mux          *http.ServeMux
	immichURL    string
	immichAPIKey string
}

func NewServer(db *database.DB, immichURL, immichAPIKey string) *Server {
	s := &Server{
		db:           db,
		mux:          http.NewServeMux(),
		immichURL:    immichURL,
		immichAPIKey: immichAPIKey,
	}

	// Parse templates
	var err error
	s.templates, err = template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	// Register routes
	s.mux.HandleFunc("/", s.handleDashboard)
	s.mux.HandleFunc("/sessions", s.handleSessions)
	s.mux.HandleFunc("/heatmap", s.handleHeatmap)
	s.mux.HandleFunc("/homes", s.handleHomes)
	s.mux.HandleFunc("/trips", s.handleTrips)
	s.mux.HandleFunc("/coverage", s.handleCoverage)
	s.mux.HandleFunc("/devices", s.handleDevices)

	// API endpoints
	s.mux.HandleFunc("/api/sessions", s.handleAPISessions)
	s.mux.HandleFunc("/api/assets", s.handleAPIAssets)
	s.mux.HandleFunc("/api/heatmap-data", s.handleAPIHeatmapData)
	s.mux.HandleFunc("/api/homes", s.handleAPIHomes)
	s.mux.HandleFunc("/api/homes/add", s.handleAPIAddHome)
	s.mux.HandleFunc("/api/homes/delete", s.handleAPIDeleteHome)
	s.mux.HandleFunc("/api/trips", s.handleAPITrips)
	s.mux.HandleFunc("/api/trips/update", s.handleAPIUpdateTrip)
	s.mux.HandleFunc("/api/trips/exclude", s.handleAPIExcludeTrip)
	s.mux.HandleFunc("/api/devices", s.handleAPIDevices)
	s.mux.HandleFunc("/api/devices/label", s.handleAPILabelDevice)
	s.mux.HandleFunc("/api/immich-proxy/", s.handleImmichProxy)

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Get stats
	sessions, err := s.db.GetSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	assets, err := s.db.GetAssets()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	devices, err := s.db.GetDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	homes, err := s.db.GetHomeLocations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	trips, err := s.db.GetTrips()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Count assets with GPS
	assetsWithGPS := 0
	assetsWithInferred := 0
	for _, asset := range assets {
		if asset.Latitude != nil {
			assetsWithGPS++
		}
		// TODO: check inferred locations
	}

	data := struct {
		TotalSessions      int
		TotalAssets        int
		AssetsWithGPS      int
		AssetsWithInferred int
		TotalDevices       int
		TotalHomes         int
		TotalTrips         int
	}{
		TotalSessions:      len(sessions),
		TotalAssets:        len(assets),
		AssetsWithGPS:      assetsWithGPS,
		AssetsWithInferred: assetsWithInferred,
		TotalDevices:       len(devices),
		TotalHomes:         len(homes),
		TotalTrips:         len(trips),
	}

	if err := s.templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if err := s.templates.ExecuteTemplate(w, "sessions.html", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleHeatmap(w http.ResponseWriter, r *http.Request) {
	if err := s.templates.ExecuteTemplate(w, "heatmap.html", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleHomes(w http.ResponseWriter, r *http.Request) {
	if err := s.templates.ExecuteTemplate(w, "homes.html", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleTrips(w http.ResponseWriter, r *http.Request) {
	if err := s.templates.ExecuteTemplate(w, "trips.html", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// API Handlers

func (s *Server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.db.GetSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func (s *Server) handleAPIAssets(w http.ResponseWriter, r *http.Request) {
	assets, err := s.db.GetAssets()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assets)
}

func (s *Server) handleAPIHeatmapData(w http.ResponseWriter, r *http.Request) {
	assets, err := s.db.GetAssets()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build heatmap data
	type HeatmapPoint struct {
		Lat       float64 `json:"lat"`
		Lon       float64 `json:"lon"`
		Intensity int     `json:"intensity"`
	}

	// Group by location
	locationCounts := make(map[string]int)
	locationCoords := make(map[string][2]float64)

	for _, asset := range assets {
		if asset.Latitude != nil && asset.Longitude != nil {
			// Round to reduce granularity
			key := roundLocation(*asset.Latitude, *asset.Longitude, 3)
			locationCounts[key]++
			locationCoords[key] = [2]float64{*asset.Latitude, *asset.Longitude}
		}
	}

	// Convert to array
	var points []HeatmapPoint
	for key, count := range locationCounts {
		coords := locationCoords[key]
		points = append(points, HeatmapPoint{
			Lat:       coords[0],
			Lon:       coords[1],
			Intensity: count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}

func (s *Server) handleAPIHomes(w http.ResponseWriter, r *http.Request) {
	homes, err := s.db.GetHomeLocations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(homes)
}

func (s *Server) handleAPIAddHome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var home models.HomeLocation
	if err := json.NewDecoder(r.Body).Decode(&home); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.db.StoreHomeLocation(home); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleAPIDeleteHome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.db.DeleteHomeLocation(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleAPITrips(w http.ResponseWriter, r *http.Request) {
	trips, err := s.db.GetTrips()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Load all sessions to enrich trip data
	sessions, err := s.db.GetSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create session map for quick lookup
	sessionMap := make(map[int64]models.Session)
	for _, session := range sessions {
		sessionMap[session.ID] = session
	}

	// Enrich trips with session details
	for i := range trips {
		// Find sessions that belong to this trip by checking if their time falls within trip bounds
		var tripSessions []models.Session
		for _, session := range sessions {
			if !session.StartTime.Before(trips[i].StartTime) && !session.EndTime.After(trips[i].EndTime) {
				// Check if any asset IDs match
				sessionMatches := false
				for _, assetID := range session.AssetIDs {
					for _, tripAssetID := range trips[i].AssetIDs {
						if assetID == tripAssetID {
							sessionMatches = true
							break
						}
					}
					if sessionMatches {
						break
					}
				}
				if sessionMatches {
					tripSessions = append(tripSessions, session)
				}
			}
		}
		trips[i].Sessions = tripSessions
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trips)
}

func (s *Server) handleCoverage(w http.ResponseWriter, r *http.Request) {
	// Load all data
	sessions, err := s.db.GetSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	trips, err := s.db.GetTrips()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	homes, err := s.db.GetHomeLocations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	assets, err := s.db.GetAssets()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create asset map for quick lookup
	assetMap := make(map[string]models.Asset)
	for _, asset := range assets {
		assetMap[asset.ID] = asset
	}

	// Build set of session IDs that are in trips
	sessionIDsInTrips := make(map[int64]bool)
	for _, trip := range trips {
		// Find sessions that belong to this trip by matching asset IDs
		for _, session := range sessions {
			if !session.StartTime.Before(trip.StartTime) && !session.EndTime.After(trip.EndTime) {
				// Check if any asset IDs match
				for _, assetID := range session.AssetIDs {
					for _, tripAssetID := range trip.AssetIDs {
						if assetID == tripAssetID {
							sessionIDsInTrips[session.ID] = true
							break
						}
					}
					if sessionIDsInTrips[session.ID] {
						break
					}
				}
			}
		}
	}

	// Enrich trips with their sessions for visualization
	for i := range trips {
		var tripSessions []models.Session
		for _, session := range sessions {
			if sessionIDsInTrips[session.ID] {
				// Check if this session belongs to this trip
				if !session.StartTime.Before(trips[i].StartTime) && !session.EndTime.After(trips[i].EndTime) {
					for _, assetID := range session.AssetIDs {
						matches := false
						for _, tripAssetID := range trips[i].AssetIDs {
							if assetID == tripAssetID {
								matches = true
								break
							}
						}
						if matches {
							tripSessions = append(tripSessions, session)
							break
						}
					}
				}
			}
		}
		trips[i].Sessions = tripSessions
	}

	// Categorize sessions and count statistics
	var tripsCount, homeCount, orphanCount, otherCount int
	var photosInTrips, photosAtHome, orphanPhotos int

	// Enrich sessions with at_home flag
	type EnrichedSession struct {
		models.Session
		AtHome bool `json:"at_home"`
	}
	enrichedSessions := make([]EnrichedSession, 0, len(sessions))

	for _, session := range sessions {
		// Check if session is at home
		atHome := false
		if len(homes) > 0 {
			for _, home := range homes {
				distance := processor.CalculateDistance(
					session.CenterLat, session.CenterLon,
					home.Latitude, home.Longitude,
				)
				if distance <= home.Radius {
					atHome = true
					break
				}
			}
		}

		enrichedSessions = append(enrichedSessions, EnrichedSession{
			Session: session,
			AtHome:  atHome,
		})

		// Categorize session
		isInTrip := sessionIDsInTrips[session.ID]

		if isInTrip {
			tripsCount++
			photosInTrips += len(session.AssetIDs)
		} else if atHome {
			homeCount++
			photosAtHome += len(session.AssetIDs)
		} else if len(homes) > 0 {
			// Only count as orphan if we have home locations defined
			orphanCount++
			orphanPhotos += len(session.AssetIDs)
		} else {
			otherCount++
		}
	}

	// Calculate percentages
	totalPhotos := photosInTrips + photosAtHome + orphanPhotos
	if totalPhotos == 0 {
		totalPhotos = 1 // Avoid division by zero
	}

	photosInTripsPercent := (photosInTrips * 100) / totalPhotos
	photosAtHomePercent := (photosAtHome * 100) / totalPhotos
	orphanPhotosPercent := (orphanPhotos * 100) / totalPhotos

	// Marshal data to JSON for the JavaScript
	sessionsJSON, err := json.Marshal(enrichedSessions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tripsJSON, err := json.Marshal(trips)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	homesJSON, err := json.Marshal(homes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Prepare data for template
	data := struct {
		TripsCount            int
		HomeCount             int
		OrphanCount           int
		OtherCount            int
		HomesCount            int
		TotalSessions         int
		PhotosInTrips         int
		PhotosInTripsPercent  int
		PhotosAtHome          int
		PhotosAtHomePercent   int
		OrphanPhotos          int
		OrphanPhotosPercent   int
		SessionsJSON          template.JS
		TripsJSON             template.JS
		HomesJSON             template.JS
	}{
		TripsCount:           tripsCount,
		HomeCount:            homeCount,
		OrphanCount:          orphanCount,
		OtherCount:           otherCount,
		HomesCount:           len(homes),
		TotalSessions:        len(sessions),
		PhotosInTrips:        photosInTrips,
		PhotosInTripsPercent: photosInTripsPercent,
		PhotosAtHome:         photosAtHome,
		PhotosAtHomePercent:  photosAtHomePercent,
		OrphanPhotos:         orphanPhotos,
		OrphanPhotosPercent:  orphanPhotosPercent,
		SessionsJSON:         template.JS(sessionsJSON),
		TripsJSON:            template.JS(tripsJSON),
		HomesJSON:            template.JS(homesJSON),
	}

	if err := s.templates.ExecuteTemplate(w, "coverage.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAPIUpdateTrip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var updateData struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get the existing trip
	trip, err := s.db.GetTrip(updateData.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Update only the name
	trip.Name = updateData.Name

	// Save the updated trip
	if err := s.db.UpdateTrip(trip); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleAPIExcludeTrip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var updateData struct {
		ID              int64 `json:"id"`
		ExcludeFromAlbum bool  `json:"exclude_from_album"`
	}

	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get the existing trip
	trip, err := s.db.GetTrip(updateData.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Update the exclude flag
	trip.ExcludeFromAlbum = updateData.ExcludeFromAlbum

	// Save the updated trip
	if err := s.db.UpdateTrip(trip); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Helper function to round location for heatmap grouping
func roundLocation(lat, lon float64, decimals int) string {
	factor := 1.0
	for i := 0; i < decimals; i++ {
		factor *= 10
	}
	lat = float64(int(lat*factor)) / factor
	lon = float64(int(lon*factor)) / factor
	return strconv.FormatFloat(lat, 'f', decimals, 64) + "," + strconv.FormatFloat(lon, 'f', decimals, 64)
}

// handleDevices renders the device labeling page
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	data := struct {
		ImmichURL    string
		ImmichAPIKey string
	}{
		ImmichURL:    s.immichURL,
		ImmichAPIKey: s.immichAPIKey,
	}

	if err := s.templates.ExecuteTemplate(w, "devices.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleAPIDevices returns devices with sample photos for labeling
func (s *Server) handleAPIDevices(w http.ResponseWriter, r *http.Request) {
	// Get devices from database
	devices, err := s.db.GetDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get all assets to find samples for each device
	assets, err := s.db.GetAssets()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Group assets by device
	deviceAssets := make(map[string][]models.Asset)
	for _, asset := range assets {
		if asset.Make == "" && asset.Model == "" {
			continue
		}
		deviceID := processor.FindMatchingDevice(asset, devices)
		if deviceID != "" {
			deviceAssets[deviceID] = append(deviceAssets[deviceID], asset)
		}
	}

	// Build response with sample photos (up to 5 per device)
	type DeviceWithSamples struct {
		models.Device
		SampleAssets []models.Asset `json:"sample_assets"`
	}

	var response []DeviceWithSamples
	for _, device := range devices {
		samples := deviceAssets[device.ID]
		if len(samples) > 5 {
			samples = samples[:5]
		}
		response = append(response, DeviceWithSamples{
			Device:       device,
			SampleAssets: samples,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAPILabelDevice updates the photographer label for a device
func (s *Server) handleAPILabelDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		DeviceID     string `json:"device_id"`
		Photographer string `json:"photographer"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update device photographer
	if err := s.db.UpdateDevicePhotographer(request.DeviceID, request.Photographer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleImmichProxy proxies requests to Immich with authentication
func (s *Server) handleImmichProxy(w http.ResponseWriter, r *http.Request) {
	// Extract the path after /api/immich-proxy/
	immichPath := r.URL.Path[len("/api/immich-proxy"):]

	// Build Immich URL
	immichURL := s.immichURL + immichPath
	if r.URL.RawQuery != "" {
		immichURL += "?" + r.URL.RawQuery
	}

	// Create request to Immich
	req, err := http.NewRequest(r.Method, immichURL, nil)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Add API key header
	req.Header.Set("x-api-key", s.immichAPIKey)

	// Make request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to fetch from Immich", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Stream the response
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
		}
		if err != nil {
			break
		}
	}
}
