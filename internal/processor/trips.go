package processor

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jamo/immich-albums/internal/models"
)

// TripCriteria defines parameters for trip detection
type TripCriteria struct {
	MinDistanceFromHome float64       // km, sessions closer than this are not trips
	MaxSessionGap       time.Duration // max time between sessions to group into same trip
	MinDuration         time.Duration // minimum trip duration
	MinSessions         int           // minimum sessions to form a trip
	MaxHomeStayDuration time.Duration // max time at home before trip splits (for brief returns home)
	ForceSplitDates     []time.Time   // dates where trips should be forcefully split
}

// DefaultTripCriteria returns sensible defaults
func DefaultTripCriteria() TripCriteria {
	return TripCriteria{
		MinDistanceFromHome: 50.0,            // 50km from home
		MaxSessionGap:       48 * time.Hour,  // 2 days between sessions
		MinDuration:         2 * time.Hour,   // at least 2 hours
		MinSessions:         1,               // even single session can be a trip
		MaxHomeStayDuration: 36 * time.Hour,  // if home for more than 1.5 days, trip ends
	}
}

// DetectTrips identifies trips from sessions based on home locations
func DetectTrips(sessions []models.Session, homes []models.HomeLocation, criteria TripCriteria, assets []models.Asset) []models.Trip {
	if len(sessions) == 0 {
		fmt.Println("No sessions to analyze")
		return nil
	}

	// Create asset map for quick lookups
	assetMap := make(map[string]models.Asset)
	for _, asset := range assets {
		assetMap[asset.ID] = asset
	}

	// Sort sessions by start time
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.Before(sessions[j].StartTime)
	})

	// Mark each session as at home or away
	type sessionWithHomeStatus struct {
		session models.Session
		atHome  bool
	}

	var allSessions []sessionWithHomeStatus
	awayCount := 0
	for _, session := range sessions {
		minDistanceFromHome := calculateMinDistanceFromHomes(session, homes)
		atHome := minDistanceFromHome < criteria.MinDistanceFromHome

		allSessions = append(allSessions, sessionWithHomeStatus{
			session: session,
			atHome:  atHome,
		})

		if !atHome {
			awayCount++
		}
	}

	fmt.Printf("Sessions away from home (>%.0fkm): %d\n", criteria.MinDistanceFromHome, awayCount)

	if awayCount == 0 {
		fmt.Println("No sessions found away from home. Add home locations first!")
		return nil
	}

	// Group sessions into trips
	// A trip ends when:
	// 1. We stay home for longer than MaxHomeStayDuration before going away again
	// 2. Time gap exceeds MaxSessionGap
	// 3. We cross a forced split date
	// 4. We reach the end of sessions
	var trips []models.Trip
	var currentTripSessions []models.Session
	var lastHomeReturnTime *time.Time
	inTrip := false

	for i, s := range allSessions {
		// Check if this session crosses a forced split date
		shouldForceSplit := false
		if len(currentTripSessions) > 0 && len(criteria.ForceSplitDates) > 0 {
			lastSessionDate := currentTripSessions[len(currentTripSessions)-1].EndTime
			currentSessionDate := s.session.StartTime

			// Check if we've crossed any split date
			for _, splitDate := range criteria.ForceSplitDates {
				// If the split date is between the last session and current session, split
				if !lastSessionDate.After(splitDate) && currentSessionDate.After(splitDate) {
					shouldForceSplit = true
					fmt.Printf("  Forcing trip split at %s\n", splitDate.Format("2006-01-02"))
					break
				}
			}
		}

		if shouldForceSplit && inTrip && len(currentTripSessions) > 0 {
			// Force split - finalize current trip
			if len(currentTripSessions) >= criteria.MinSessions {
				trip := createTripFromSessions(currentTripSessions, homes, assetMap)
				if trip.EndTime.Sub(trip.StartTime) >= criteria.MinDuration {
					trips = append(trips, trip)
					fmt.Printf("  Trip ended (forced split): %s\n", trip.Name)
				}
			}
			// Start new trip with current session
			currentTripSessions = []models.Session{s.session}
			inTrip = true
			lastHomeReturnTime = nil
			continue
		}

		if s.atHome {
			// At home - just track when we returned, don't end trip yet
			if inTrip && lastHomeReturnTime == nil {
				// First home session after being away - mark the return time
				lastHomeReturnTime = &s.session.StartTime
			}
			// Continue - we might go away again soon (brief return home)
		} else {
			// Away from home
			if !inTrip {
				// Start new trip
				currentTripSessions = []models.Session{s.session}
				inTrip = true
				lastHomeReturnTime = nil
			} else {
				// We're continuing a trip
				// Check if we returned home and how long we stayed
				if lastHomeReturnTime != nil {
					homeStayDuration := s.session.StartTime.Sub(*lastHomeReturnTime)
					if homeStayDuration > criteria.MaxHomeStayDuration {
						// We stayed home too long - this is a new trip
						if len(currentTripSessions) >= criteria.MinSessions {
							trip := createTripFromSessions(currentTripSessions, homes, assetMap)
							if trip.EndTime.Sub(trip.StartTime) >= criteria.MinDuration {
								trips = append(trips, trip)
								fmt.Printf("  Trip ended (stayed home %v): %s\n", homeStayDuration.Round(time.Hour), trip.Name)
							}
						}
						// Start new trip
						currentTripSessions = []models.Session{s.session}
						lastHomeReturnTime = nil
					} else {
						// Brief return home - continue same trip
						currentTripSessions = append(currentTripSessions, s.session)
						lastHomeReturnTime = nil // Reset since we're away again
					}
				} else {
					// Check time gap from last session
					prev := currentTripSessions[len(currentTripSessions)-1]
					timeGap := s.session.StartTime.Sub(prev.EndTime)

					if timeGap <= criteria.MaxSessionGap {
						// Add to current trip
						currentTripSessions = append(currentTripSessions, s.session)
					} else {
						// Time gap too large - end current trip and start new one
						if len(currentTripSessions) >= criteria.MinSessions {
							trip := createTripFromSessions(currentTripSessions, homes, assetMap)
							if trip.EndTime.Sub(trip.StartTime) >= criteria.MinDuration {
								trips = append(trips, trip)
								fmt.Printf("  Trip ended (time gap %v): %s\n", timeGap.Round(time.Hour), trip.Name)
							}
						}
						// Start new trip
						currentTripSessions = []models.Session{s.session}
						lastHomeReturnTime = nil
					}
				}
			}
		}

		// If this is the last session and we're in a trip, finalize it
		if i == len(allSessions)-1 && inTrip && len(currentTripSessions) > 0 {
			if len(currentTripSessions) >= criteria.MinSessions {
				trip := createTripFromSessions(currentTripSessions, homes, assetMap)
				if trip.EndTime.Sub(trip.StartTime) >= criteria.MinDuration {
					trips = append(trips, trip)
					fmt.Printf("  Trip ended (end of sessions): %s\n", trip.Name)
				}
			}
		}
	}

	fmt.Printf("Detected %d trips\n", len(trips))

	return trips
}

func calculateMinDistanceFromHomes(session models.Session, homes []models.HomeLocation) float64 {
	if len(homes) == 0 {
		return 999999.0 // Very far if no homes defined
	}

	minDistance := 999999.0
	for _, home := range homes {
		distance := CalculateDistance(
			session.CenterLat, session.CenterLon,
			home.Latitude, home.Longitude,
		)
		if distance < minDistance {
			minDistance = distance
		}
	}

	return minDistance
}

func createTripFromSessions(sessions []models.Session, homes []models.HomeLocation, assetMap map[string]models.Asset) models.Trip {
	// Calculate trip bounds
	startTime := sessions[0].StartTime
	endTime := sessions[len(sessions)-1].EndTime

	// Collect all session IDs and asset IDs
	var sessionIDs []int64
	var allAssetIDs []string
	photographerSet := make(map[string]bool)

	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.ID)
		allAssetIDs = append(allAssetIDs, session.AssetIDs...)
		photographerSet[session.Photographer] = true
	}

	// Calculate center point of trip (average of session centers)
	var sumLat, sumLon float64
	for _, session := range sessions {
		sumLat += session.CenterLat
		sumLon += session.CenterLon
	}
	centerLat := sumLat / float64(len(sessions))
	centerLon := sumLon / float64(len(sessions))

	// Calculate distance from home
	minHomeDistance := calculateMinDistanceFromHomes(sessions[0], homes)

	// Calculate total travel distance (sum of distances between session centers)
	totalDistance := 0.0
	for i := 1; i < len(sessions); i++ {
		dist := CalculateDistance(
			sessions[i-1].CenterLat, sessions[i-1].CenterLon,
			sessions[i].CenterLat, sessions[i].CenterLon,
		)
		totalDistance += dist
	}

	// Generate trip name
	name := generateTripName(sessions, startTime, endTime, centerLat, centerLon, assetMap)

	// Collect photographers
	var photographers []string
	for p := range photographerSet {
		photographers = append(photographers, p)
	}
	sort.Strings(photographers)

	return models.Trip{
		Name:          name,
		StartTime:     startTime,
		EndTime:       endTime,
		Sessions:      sessions,
		HomeDistance:  minHomeDistance,
		TotalDistance: totalDistance,
		CenterLat:     centerLat,
		CenterLon:     centerLon,
		AssetIDs:      allAssetIDs,
		Photographers: strings.Join(photographers, ", "),
		SessionCount:  len(sessions),
	}
}

func generateTripName(sessions []models.Session, start, end time.Time, lat, lon float64, assetMap map[string]models.Asset) string {
	// Try to extract location from session data
	location := extractLocationFromSessions(sessions, assetMap)

	// Format dates
	if start.Year() == end.Year() && start.Month() == end.Month() && start.Day() == end.Day() {
		// Single day trip
		dateStr := start.Format("Jan 2, 2006")
		if location != "" {
			return fmt.Sprintf("%s - %s", location, dateStr)
		}
		return fmt.Sprintf("Trip - %s", dateStr)
	} else {
		// Multi-day trip
		if start.Year() == end.Year() && start.Month() == end.Month() {
			// Same month
			dateStr := fmt.Sprintf("%s %d-%d, %d", start.Format("Jan"), start.Day(), end.Day(), start.Year())
			if location != "" {
				return fmt.Sprintf("%s - %s", location, dateStr)
			}
			return fmt.Sprintf("Trip - %s", dateStr)
		} else {
			// Different months
			dateStr := fmt.Sprintf("%s - %s", start.Format("Jan 2"), end.Format("Jan 2, 2006"))
			if location != "" {
				return fmt.Sprintf("%s - %s", location, dateStr)
			}
			return fmt.Sprintf("Trip - %s", dateStr)
		}
	}
}

func extractLocationFromSessions(sessions []models.Session, assetMap map[string]models.Asset) string {
	// Count city and country occurrences across all assets in sessions
	cityCount := make(map[string]int)
	countryCount := make(map[string]int)

	for _, session := range sessions {
		for _, assetID := range session.AssetIDs {
			if asset, ok := assetMap[assetID]; ok {
				// Count cities
				if asset.City != "" {
					cityCount[asset.City]++
				}
				// Count countries
				if asset.Country != "" {
					countryCount[asset.Country]++
				}
			}
		}
	}

	// Find most common city and country
	var bestCity string
	var bestCityCount int
	for city, count := range cityCount {
		if count > bestCityCount {
			bestCity = city
			bestCityCount = count
		}
	}

	var bestCountry string
	var bestCountryCount int
	for country, count := range countryCount {
		if count > bestCountryCount {
			bestCountry = country
			bestCountryCount = count
		}
	}

	// Format location string
	if bestCity != "" && bestCountry != "" {
		return fmt.Sprintf("%s, %s", bestCity, bestCountry)
	} else if bestCity != "" {
		return bestCity
	} else if bestCountry != "" {
		return bestCountry
	}

	return ""
}
