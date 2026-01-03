package processor

import (
	"fmt"
	"sort"

	"github.com/jamo/immich-albums/internal/models"
)

// ClusteringParams contains parameters for session detection
type ClusteringParams struct {
	MaxTimeGapHours  float64 // Maximum time between photos in same session
	MaxDistanceKM    float64 // Maximum distance between photos in same session
	MinPhotosInSession int   // Minimum photos to form a session
	MinConfidence    float64 // Minimum confidence for inferred locations
}

// DefaultClusteringParams returns sensible defaults
func DefaultClusteringParams() ClusteringParams {
	return ClusteringParams{
		MaxTimeGapHours:    6.0,  // 6 hours
		MaxDistanceKM:      5.0,  // 5 km
		MinPhotosInSession: 2,    // At least 2 photos
		MinConfidence:      0.3,  // Accept moderate confidence locations
	}
}

// AssetWithLocation wraps an asset with its effective location
type AssetWithLocation struct {
	Asset       models.Asset
	Latitude    float64
	Longitude   float64
	Confidence  float64
	HasLocation bool
}

// DetectSessions groups photos into sessions based on time and location
func DetectSessions(assets []models.Asset, inferences map[string]LocationInference, devices map[string]models.Device, params ClusteringParams) []models.Session {
	// Prepare assets with effective locations
	fmt.Println("Filtering assets with valid locations...")
	var located []AssetWithLocation
	const progressInterval = 5000 // Report every 5000 assets for clustering
	for i, asset := range assets {
		// Progress indicator
		if i > 0 && i%progressInterval == 0 {
			fmt.Printf("  Progress: %d/%d (%.1f%%)\r", i, len(assets), float64(i)*100/float64(len(assets)))
		}

		lat, lon, hasLoc, conf := GetEffectiveLocation(asset, inferences)
		if hasLoc && conf >= params.MinConfidence {
			located = append(located, AssetWithLocation{
				Asset:       asset,
				Latitude:    lat,
				Longitude:   lon,
				Confidence:  conf,
				HasLocation: true,
			})
		}
	}
	fmt.Printf("  Progress: %d/%d (100.0%%)  \n", len(assets), len(assets))

	fmt.Printf("Assets with valid locations: %d\n", len(located))

	// Sort by time
	sort.Slice(located, func(i, j int) bool {
		return located[i].Asset.LocalDateTime.Before(located[j].Asset.LocalDateTime)
	})

	// Group by photographer - match assets to devices by make/model/filename pattern
	photographerAssets := make(map[string][]AssetWithLocation)
	for _, asset := range located {
		if asset.Asset.Make == "" && asset.Asset.Model == "" {
			continue // Skip assets without device info
		}
		// Find matching device for this asset
		deviceID := findMatchingDeviceMap(asset.Asset, devices)
		if deviceID != "" {
			if device, exists := devices[deviceID]; exists && device.Photographer != "" {
				photographerAssets[device.Photographer] = append(photographerAssets[device.Photographer], asset)
			}
		}
	}

	fmt.Printf("Photographers with located assets: %d\n", len(photographerAssets))

	// Cluster each photographer's assets separately
	var allSessions []models.Session
	fmt.Println("Clustering assets into sessions by photographer...")
	photoCount := 0
	for photographer, assets := range photographerAssets {
		photoCount++
		fmt.Printf("  Processing %s (%d assets)...\n", photographer, len(assets))
		sessions := clusterAssetsIntoSessions(assets, photographer, params)
		fmt.Printf("    Found %d sessions\n", len(sessions))
		allSessions = append(allSessions, sessions...)
	}

	fmt.Printf("Detected %d sessions\n", len(allSessions))

	return allSessions
}

func clusterAssetsIntoSessions(assets []AssetWithLocation, photographer string, params ClusteringParams) []models.Session {
	if len(assets) == 0 {
		return nil
	}

	var sessions []models.Session
	var currentSession []AssetWithLocation
	currentSession = append(currentSession, assets[0])

	for i := 1; i < len(assets); i++ {
		prev := assets[i-1]
		curr := assets[i]

		// Calculate time gap in hours
		timeGap := curr.Asset.LocalDateTime.Sub(prev.Asset.LocalDateTime).Hours()

		// Calculate distance
		distance := CalculateDistance(
			prev.Latitude, prev.Longitude,
			curr.Latitude, curr.Longitude,
		)

		// Check if current asset belongs to current session
		withinTime := timeGap <= params.MaxTimeGapHours
		withinDistance := distance <= params.MaxDistanceKM

		if withinTime && withinDistance {
			// Add to current session
			currentSession = append(currentSession, curr)
		} else {
			// Finalize current session if it meets minimum size
			if len(currentSession) >= params.MinPhotosInSession {
				session := createSessionFromAssets(currentSession, photographer)
				sessions = append(sessions, session)
			}

			// Start new session
			currentSession = []AssetWithLocation{curr}
		}
	}

	// Don't forget the last session
	if len(currentSession) >= params.MinPhotosInSession {
		session := createSessionFromAssets(currentSession, photographer)
		sessions = append(sessions, session)
	}

	return sessions
}

func createSessionFromAssets(assets []AssetWithLocation, photographer string) models.Session {
	// Calculate session bounds
	startTime := assets[0].Asset.LocalDateTime
	endTime := assets[len(assets)-1].Asset.LocalDateTime

	// Calculate center point (simple average)
	var sumLat, sumLon float64
	assetIDs := make([]string, len(assets))

	for i, asset := range assets {
		sumLat += asset.Latitude
		sumLon += asset.Longitude
		assetIDs[i] = asset.Asset.ID
	}

	centerLat := sumLat / float64(len(assets))
	centerLon := sumLon / float64(len(assets))

	// Calculate radius (max distance from center)
	maxRadius := 0.0
	for _, asset := range assets {
		dist := CalculateDistance(centerLat, centerLon, asset.Latitude, asset.Longitude)
		if dist > maxRadius {
			maxRadius = dist
		}
	}

	return models.Session{
		StartTime:    startTime,
		EndTime:      endTime,
		AssetIDs:     assetIDs,
		CenterLat:    centerLat,
		CenterLon:    centerLon,
		Radius:       maxRadius,
		Photographer: photographer,
	}
}

// MergeSessions attempts to merge nearby sessions from different photographers
// This is useful when multiple people take photos at the same event
func MergeSessions(sessions []models.Session, maxTimeGapHours float64, maxDistanceKM float64) []models.Session {
	const maxMergedSessionRadius = 50.0 // Maximum radius in km for a merged session
	if len(sessions) <= 1 {
		return sessions
	}

	// Sort by start time
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.Before(sessions[j].StartTime)
	})

	var merged []models.Session
	var currentGroup []models.Session
	currentGroup = append(currentGroup, sessions[0])

	for i := 1; i < len(sessions); i++ {
		// Early termination: check time gap from earliest session in group
		// If too large, no point checking others (sessions are time-sorted)
		earliestEndTime := currentGroup[0].EndTime
		timeGapFromEarliest := sessions[i].StartTime.Sub(earliestEndTime).Hours()

		if timeGapFromEarliest > maxTimeGapHours {
			// Time gap too large - finalize current group and start new one
			if len(currentGroup) == 1 {
				merged = append(merged, currentGroup[0])
			} else {
				mergedSession := combineSessionGroup(currentGroup)
				merged = append(merged, mergedSession)
			}
			currentGroup = []models.Session{sessions[i]}
			continue
		}

		// Check if current session can merge with any in the current group
		canMerge := false
		for _, groupSession := range currentGroup {
			timeGap := sessions[i].StartTime.Sub(groupSession.EndTime).Hours()
			distance := CalculateDistance(
				groupSession.CenterLat, groupSession.CenterLon,
				sessions[i].CenterLat, sessions[i].CenterLon,
			)

			if timeGap <= maxTimeGapHours && distance <= maxDistanceKM {
				canMerge = true
				break
			}
		}

		// Additional check: would the merged group exceed max radius?
		if canMerge && len(currentGroup) > 0 {
			// Calculate approximate merged radius
			var sumLat, sumLon float64
			for _, s := range currentGroup {
				sumLat += s.CenterLat
				sumLon += s.CenterLon
			}
			sumLat += sessions[i].CenterLat
			sumLon += sessions[i].CenterLon
			approxCenterLat := sumLat / float64(len(currentGroup)+1)
			approxCenterLon := sumLon / float64(len(currentGroup)+1)

			// Check if any session would be too far from the new center
			maxDist := 0.0
			for _, s := range currentGroup {
				dist := CalculateDistance(approxCenterLat, approxCenterLon, s.CenterLat, s.CenterLon) + s.Radius
				if dist > maxDist {
					maxDist = dist
				}
			}
			distNew := CalculateDistance(approxCenterLat, approxCenterLon, sessions[i].CenterLat, sessions[i].CenterLon) + sessions[i].Radius
			if distNew > maxDist {
				maxDist = distNew
			}

			if maxDist > maxMergedSessionRadius {
				canMerge = false // Would make the session too dispersed
			}
		}

		if canMerge {
			currentGroup = append(currentGroup, sessions[i])
		} else {
			// Finalize current group
			if len(currentGroup) == 1 {
				merged = append(merged, currentGroup[0])
			} else {
				mergedSession := combineSessionGroup(currentGroup)
				merged = append(merged, mergedSession)
			}

			// Start new group
			currentGroup = []models.Session{sessions[i]}
		}
	}

	// Don't forget the last group
	if len(currentGroup) == 1 {
		merged = append(merged, currentGroup[0])
	} else if len(currentGroup) > 1 {
		mergedSession := combineSessionGroup(currentGroup)
		merged = append(merged, mergedSession)
	}

	return merged
}

func combineSessionGroup(sessions []models.Session) models.Session {
	// Find overall time bounds
	startTime := sessions[0].StartTime
	endTime := sessions[0].EndTime

	// Combine all asset IDs (use map for deduplication)
	assetIDSet := make(map[string]bool)
	photographers := make(map[string]bool)

	var sumLat, sumLon float64
	count := 0

	for _, session := range sessions {
		if session.StartTime.Before(startTime) {
			startTime = session.StartTime
		}
		if session.EndTime.After(endTime) {
			endTime = session.EndTime
		}

		// Add asset IDs to set (automatically deduplicates)
		for _, assetID := range session.AssetIDs {
			assetIDSet[assetID] = true
		}
		photographers[session.Photographer] = true

		sumLat += session.CenterLat
		sumLon += session.CenterLon
		count++
	}

	// Convert asset ID set back to slice
	allAssetIDs := make([]string, 0, len(assetIDSet))
	for assetID := range assetIDSet {
		allAssetIDs = append(allAssetIDs, assetID)
	}

	// Calculate new center
	centerLat := sumLat / float64(count)
	centerLon := sumLon / float64(count)

	// Calculate new radius
	maxRadius := 0.0
	for _, session := range sessions {
		dist := CalculateDistance(centerLat, centerLon, session.CenterLat, session.CenterLon) + session.Radius
		if dist > maxRadius {
			maxRadius = dist
		}
	}

	// Create photographer list
	var photographerList string
	for p := range photographers {
		if photographerList == "" {
			photographerList = p
		} else {
			photographerList += ", " + p
		}
	}

	return models.Session{
		StartTime:    startTime,
		EndTime:      endTime,
		AssetIDs:     allAssetIDs,
		CenterLat:    centerLat,
		CenterLon:    centerLon,
		Radius:       maxRadius,
		Photographer: photographerList,
	}
}
