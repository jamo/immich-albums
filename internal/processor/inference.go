package processor

import (
	"fmt"
	"math"
	"sort"

	"github.com/jamo/immich-albums/internal/models"
)

// Constants for location inference
const (
	EarthRadiusKM              = 6371.0 // Earth radius in kilometers
	InterpolationPenalty       = 0.9    // Confidence penalty for interpolated locations
	MinimumConfidenceThreshold = 0.1    // Minimum confidence to accept an inference
	ProgressReportInterval     = 1000   // Report progress every N assets
)

// LocationInference contains inferred location data
type LocationInference struct {
	AssetID    string
	Latitude   float64
	Longitude  float64
	Confidence float64
	Source     string // "nearby", "interpolated", "same-session"
	Method     string // Description of how it was inferred
}

// InferLocations processes assets and infers locations for those without GPS
func InferLocations(assets []models.Asset, devices []models.Device) []LocationInference {
	// Create device map for quick lookup
	deviceMap := make(map[string]models.Device)
	for _, device := range devices {
		deviceMap[device.ID] = device
	}

	// Separate assets with and without GPS
	var withGPS []models.Asset
	var withoutGPS []models.Asset

	for _, asset := range assets {
		if asset.Latitude != nil && asset.Longitude != nil {
			withGPS = append(withGPS, asset)
		} else {
			withoutGPS = append(withoutGPS, asset)
		}
	}

	fmt.Printf("Assets with GPS: %d\n", len(withGPS))
	fmt.Printf("Assets without GPS: %d\n", len(withoutGPS))

	// Sort both by timestamp for efficient searching
	sort.Slice(withGPS, func(i, j int) bool {
		return withGPS[i].LocalDateTime.Before(withGPS[j].LocalDateTime)
	})
	sort.Slice(withoutGPS, func(i, j int) bool {
		return withoutGPS[i].LocalDateTime.Before(withoutGPS[j].LocalDateTime)
	})

	// Pre-group GPS assets by photographer for efficiency
	fmt.Println("Grouping GPS assets by photographer...")
	photographerGPS := make(map[string][]models.Asset)
	for _, gpsAsset := range withGPS {
		if gpsAsset.Make == "" && gpsAsset.Model == "" {
			continue // Skip assets without device info
		}
		// Find matching device for this asset
		deviceID := findMatchingDeviceMap(gpsAsset, deviceMap)
		if deviceID != "" {
			if gpsDevice, exists := deviceMap[deviceID]; exists && gpsDevice.Photographer != "" {
				photographerGPS[gpsDevice.Photographer] = append(photographerGPS[gpsDevice.Photographer], gpsAsset)
			}
		}
	}
	fmt.Printf("Found GPS data for %d photographers\n", len(photographerGPS))

	var inferences []LocationInference

	fmt.Println("Processing assets...")
	for i, asset := range withoutGPS {
		// Progress indicator
		if i > 0 && i%ProgressReportInterval == 0 {
			fmt.Printf("  Progress: %d/%d (%.1f%%)\r", i, len(withoutGPS), float64(i)*100/float64(len(withoutGPS)))
		}

		if asset.Make == "" && asset.Model == "" {
			continue // Skip assets without device info
		}
		// Find matching device for this asset
		deviceID := findMatchingDeviceMap(asset, deviceMap)
		if deviceID == "" {
			continue
		}
		device, exists := deviceMap[deviceID]
		if !exists || device.Photographer == "" {
			continue // Skip if device not labeled
		}

		// Get GPS assets for this photographer
		gpsForPhotographer, hasGPS := photographerGPS[device.Photographer]
		if !hasGPS || len(gpsForPhotographer) == 0 {
			continue // No GPS data for this photographer
		}

		// Try to infer location
		inference := inferSingleLocation(asset, device, gpsForPhotographer)
		if inference != nil {
			inferences = append(inferences, *inference)
		}
	}
	fmt.Printf("  Progress: %d/%d (100.0%%)  \n", len(withoutGPS), len(withoutGPS))

	fmt.Printf("Successfully inferred: %d locations\n", len(inferences))

	return inferences
}

func inferSingleLocation(asset models.Asset, device models.Device, photographerGPS []models.Asset) *LocationInference {
	// GPS assets are already filtered for this photographer
	// Strategy 1: Find nearest GPS photo in time
	nearest := findNearestInTime(asset, photographerGPS)
	if nearest != nil {
		timeDiff := math.Abs(asset.LocalDateTime.Sub(nearest.LocalDateTime).Hours())

		// Calculate confidence based on time gap
		confidence := calculateTimeBasedConfidence(timeDiff)

		if confidence > MinimumConfidenceThreshold { // Only accept if confidence is reasonable
			return &LocationInference{
				AssetID:    asset.ID,
				Latitude:   *nearest.Latitude,
				Longitude:  *nearest.Longitude,
				Confidence: confidence,
				Source:     "nearby",
				Method:     fmt.Sprintf("nearest photo %.1f hours away", timeDiff),
			}
		}
	}

	// Strategy 2: Interpolation between two GPS photos
	interpolated := interpolateLocation(asset, photographerGPS)
	if interpolated != nil {
		return interpolated
	}

	return nil
}

func findNearestInTime(target models.Asset, candidates []models.Asset) *models.Asset {
	if len(candidates) == 0 {
		return nil
	}

	// Binary search to find insertion point (candidates are sorted by time)
	idx := sort.Search(len(candidates), func(i int) bool {
		return candidates[i].LocalDateTime.After(target.LocalDateTime) ||
			candidates[i].LocalDateTime.Equal(target.LocalDateTime)
	})

	// Check the candidate at idx and idx-1 to find the nearest
	var nearest *models.Asset
	minDiff := math.MaxFloat64

	// Check candidate before insertion point
	if idx > 0 {
		diff := math.Abs(target.LocalDateTime.Sub(candidates[idx-1].LocalDateTime).Seconds())
		if diff < minDiff {
			minDiff = diff
			nearest = &candidates[idx-1]
		}
	}

	// Check candidate at or after insertion point
	if idx < len(candidates) {
		diff := math.Abs(target.LocalDateTime.Sub(candidates[idx].LocalDateTime).Seconds())
		if diff < minDiff {
			nearest = &candidates[idx]
		}
	}

	return nearest
}

func interpolateLocation(target models.Asset, gpsAssets []models.Asset) *LocationInference {
	// Binary search to find insertion point (gpsAssets are sorted by time)
	idx := sort.Search(len(gpsAssets), func(i int) bool {
		return gpsAssets[i].LocalDateTime.After(target.LocalDateTime) ||
			gpsAssets[i].LocalDateTime.Equal(target.LocalDateTime)
	})

	// Need a photo before and after the target for interpolation
	if idx == 0 || idx >= len(gpsAssets) {
		return nil // Can't interpolate - target is before first or after last GPS photo
	}

	before := &gpsAssets[idx-1]
	after := &gpsAssets[idx]

	// Calculate time-based interpolation weight
	totalDuration := after.LocalDateTime.Sub(before.LocalDateTime).Seconds()
	if totalDuration == 0 {
		return nil
	}

	targetOffset := target.LocalDateTime.Sub(before.LocalDateTime).Seconds()
	weight := targetOffset / totalDuration

	// Interpolate coordinates
	lat := *before.Latitude + (*after.Latitude-*before.Latitude)*weight
	lon := *before.Longitude + (*after.Longitude-*before.Longitude)*weight

	// Calculate confidence
	timeDiffBefore := target.LocalDateTime.Sub(before.LocalDateTime).Hours()
	timeDiffAfter := after.LocalDateTime.Sub(target.LocalDateTime).Hours()
	maxTimeDiff := math.Max(timeDiffBefore, timeDiffAfter)

	confidence := calculateTimeBasedConfidence(maxTimeDiff) * InterpolationPenalty // Slightly lower for interpolation

	if confidence < MinimumConfidenceThreshold {
		return nil
	}

	return &LocationInference{
		AssetID:    target.ID,
		Latitude:   lat,
		Longitude:  lon,
		Confidence: confidence,
		Source:     "interpolated",
		Method:     fmt.Sprintf("interpolated between photos %.1fh before and %.1fh after", timeDiffBefore, timeDiffAfter),
	}
}

// calculateTimeBasedConfidence returns a confidence score (0-1) based on time gap
// Confidence decay function:
// - < 1 hour: 1.0 (very high confidence)
// - 1-6 hours: 0.9 (high confidence)
// - 6-24 hours: 0.7 (good confidence)
// - 1-3 days: 0.5 (moderate confidence)
// - 3-7 days: 0.3 (low confidence)
// - > 7 days: 0.1 (very low confidence)
func calculateTimeBasedConfidence(hoursDiff float64) float64 {
	switch {
	case hoursDiff < 1:
		return 1.0
	case hoursDiff < 6:
		return 0.9
	case hoursDiff < 24:
		return 0.7
	case hoursDiff < 72: // 3 days
		return 0.5
	case hoursDiff < 168: // 7 days
		return 0.3
	case hoursDiff < 336: // 14 days
		return 0.15
	default:
		return 0.1
	}
}

// CalculateDistance returns distance in kilometers between two GPS coordinates
// Using the Haversine formula
func CalculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Convert to radians
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	// Haversine formula
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return EarthRadiusKM * c
}

// GetEffectiveLocation returns the best available location for an asset
func GetEffectiveLocation(asset models.Asset, inferences map[string]LocationInference) (lat, lon float64, hasLocation bool, confidence float64) {
	// Prefer original GPS data
	if asset.Latitude != nil && asset.Longitude != nil {
		return *asset.Latitude, *asset.Longitude, true, 1.0
	}

	// Fall back to inferred location
	if inference, exists := inferences[asset.ID]; exists {
		return inference.Latitude, inference.Longitude, true, inference.Confidence
	}

	return 0, 0, false, 0
}
