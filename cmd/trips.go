package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/jamo/immich-albums/internal/processor"
	"github.com/spf13/cobra"
)

var (
	minDistanceFromHome float64
	maxSessionGap       float64
	minTripDuration     float64
	minSessionsInTrip   int
	maxHomeStayHours    float64
	splitDates          []string
)

var tripsCmd = &cobra.Command{
	Use:   "detect-trips",
	Short: "Detect trips from sessions based on distance from home",
	Long: `Analyzes sessions and identifies trips based on:
  - Distance from home locations
  - Time gaps between sessions
  - Trip duration and session count
  - Brief returns home (e.g., overnight on boating trips)`,
	RunE: runTrips,
}

func init() {
	rootCmd.AddCommand(tripsCmd)

	tripsCmd.Flags().Float64Var(&minDistanceFromHome, "min-distance", 50.0, "Minimum distance from home in km to qualify as trip")
	tripsCmd.Flags().Float64Var(&maxSessionGap, "max-session-gap", 48.0, "Maximum hours between sessions to group into same trip")
	tripsCmd.Flags().Float64Var(&minTripDuration, "min-duration", 2.0, "Minimum trip duration in hours")
	tripsCmd.Flags().IntVar(&minSessionsInTrip, "min-sessions", 1, "Minimum sessions required for a trip")
	tripsCmd.Flags().Float64Var(&maxHomeStayHours, "max-home-stay", 36.0, "Maximum hours at home before trip splits (brief returns home like overnight stops)")
	tripsCmd.Flags().StringSliceVar(&splitDates, "split-date", []string{}, "Force trip split at specific dates (format: 2024-07-15). Can be specified multiple times.")
}

func runTrips(cmd *cobra.Command, args []string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Load sessions
	fmt.Println("Loading sessions from database...")
	sessions, err := db.GetSessions()
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	if len(sessions) == 0 {
		return fmt.Errorf("no sessions found. Run 'detect-sessions' first")
	}

	fmt.Printf("Loaded %d sessions\n", len(sessions))

	// Load home locations
	fmt.Println("Loading home locations...")
	homes, err := db.GetHomeLocations()
	if err != nil {
		return fmt.Errorf("failed to get home locations: %w", err)
	}

	if len(homes) == 0 {
		fmt.Println("Warning: No home locations defined!")
		fmt.Println("Without home locations, all sessions will be considered potential trips.")
		fmt.Println("Use the web UI (http://localhost:8080/homes) to label home locations for better trip detection.")
		fmt.Println()
	} else {
		fmt.Printf("Loaded %d home locations\n", len(homes))
		for _, home := range homes {
			fmt.Printf("  - %s (%.4f, %.4f, %.1fkm radius)\n", home.Name, home.Latitude, home.Longitude, home.Radius)
		}
	}
	// Load assets for location extraction
	fmt.Println("Loading assets from database...")
	assets, err := db.GetAssets()
	if err != nil {
		return fmt.Errorf("failed to get assets: %w", err)
	}
	fmt.Printf("Loaded %d assets\n", len(assets))

	// Parse split dates
	var parsedSplitDates []time.Time
	if len(splitDates) > 0 {
		fmt.Printf("\nParsing %d forced split dates...\n", len(splitDates))
		for _, dateStr := range splitDates {
			// Parse date in format YYYY-MM-DD
			t, err := time.Parse("2006-01-02", strings.TrimSpace(dateStr))
			if err != nil {
				return fmt.Errorf("invalid split date '%s': %w (expected format: YYYY-MM-DD)", dateStr, err)
			}
			parsedSplitDates = append(parsedSplitDates, t)
			fmt.Printf("  - Split at: %s\n", t.Format("2006-01-02"))
		}
	}

	// Set up criteria
	criteria := processor.TripCriteria{
		MinDistanceFromHome: minDistanceFromHome,
		MaxSessionGap:       time.Duration(maxSessionGap) * time.Hour,
		MinDuration:         time.Duration(minTripDuration) * time.Hour,
		MinSessions:         minSessionsInTrip,
		MaxHomeStayDuration: time.Duration(maxHomeStayHours) * time.Hour,
		ForceSplitDates:     parsedSplitDates,
	}

	fmt.Println("\nDetecting trips...")
	fmt.Printf("Parameters:\n")
	fmt.Printf("  Min distance from home: %.0fkm\n", criteria.MinDistanceFromHome)
	fmt.Printf("  Max session gap: %.0f hours\n", maxSessionGap)
	fmt.Printf("  Max home stay: %.0f hours (brief returns home don't split trips)\n", maxHomeStayHours)
	fmt.Printf("  Min trip duration: %.0f hours\n", minTripDuration)
	fmt.Printf("  Min sessions: %d\n", criteria.MinSessions)
	if len(parsedSplitDates) > 0 {
		fmt.Printf("  Forced split dates: %d\n", len(parsedSplitDates))
	}
	fmt.Println()

	// Detect trips
	trips := processor.DetectTrips(sessions, homes, criteria, assets)

	if len(trips) == 0 {
		fmt.Println("\nNo trips detected with current criteria.")
		fmt.Println("Try adjusting parameters or ensure you have sessions away from home.")
		return nil
	}

	// Store trips
	fmt.Println("\nStoring trips in database...")
	if err := db.StoreTrips(trips); err != nil {
		return fmt.Errorf("failed to store trips: %w", err)
	}

	// Print summary
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("TRIP DETECTION SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total trips detected: %d\n\n", len(trips))

	for i, trip := range trips {
		fmt.Printf("Trip %d: %s\n", i+1, trip.Name)
		fmt.Printf("  Dates: %s - %s\n",
			trip.StartTime.Format("Jan 2, 2006 15:04"),
			trip.EndTime.Format("Jan 2, 2006 15:04"))

		duration := trip.EndTime.Sub(trip.StartTime)
		if duration > 24*time.Hour {
			fmt.Printf("  Duration: %.1f days\n", duration.Hours()/24)
		} else {
			fmt.Printf("  Duration: %.1f hours\n", duration.Hours())
		}

		fmt.Printf("  Distance from home: %.1fkm\n", trip.HomeDistance)
		fmt.Printf("  Travel distance: %.1fkm\n", trip.TotalDistance)
		fmt.Printf("  Sessions: %d\n", trip.SessionCount)
		fmt.Printf("  Photos: %d\n", len(trip.AssetIDs))
		fmt.Printf("  Photographers: %s\n", trip.Photographers)
		fmt.Println()
	}

	fmt.Println("✓ Trip detection complete!")
	fmt.Println("Next: Run 'create-albums' to generate albums in Immich")

	return nil
}
