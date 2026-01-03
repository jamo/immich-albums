package cmd

import (
	"fmt"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/jamo/immich-albums/internal/processor"
	"github.com/spf13/cobra"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze photo coverage and categorization",
	Long:  `Shows statistics about how photos are categorized: at home, in trips, in sessions, or uncategorized.`,
	RunE:  runAnalyze,
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Load all data
	fmt.Println("Loading data from database...")
	assets, err := db.GetAssets()
	if err != nil {
		return fmt.Errorf("failed to get assets: %w", err)
	}

	sessions, err := db.GetSessions()
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	trips, err := db.GetTrips()
	if err != nil {
		return fmt.Errorf("failed to get trips: %w", err)
	}

	homes, err := db.GetHomeLocations()
	if err != nil {
		return fmt.Errorf("failed to get home locations: %w", err)
	}

	// Create sets for efficient lookups
	assetsInSessions := make(map[string]bool)
	for _, session := range sessions {
		for _, assetID := range session.AssetIDs {
			assetsInSessions[assetID] = true
		}
	}

	assetsInTrips := make(map[string]bool)
	for _, trip := range trips {
		for _, assetID := range trip.AssetIDs {
			assetsInTrips[assetID] = true
		}
	}

	// Categorize assets
	var photosWithLocation int
	var photosWithoutLocation int
	var photosAtHome int
	var photosInTrips int
	var photosInSessionsNotTrips int
	var photosNotInSessions int
	var photosAwayFromHomeNotInTrips int

	for _, asset := range assets {
		// Check if has location (original GPS only for this analysis)
		var lat, lon float64

		if asset.Latitude != nil && asset.Longitude != nil {
			lat = *asset.Latitude
			lon = *asset.Longitude
			photosWithLocation++
		} else {
			photosWithoutLocation++
			continue // Skip location-based analysis for assets without GPS
		}

		// Check if at home
		atHome := false
		if len(homes) > 0 {
			for _, home := range homes {
				distance := processor.CalculateDistance(lat, lon, home.Latitude, home.Longitude)
				if distance <= home.Radius {
					atHome = true
					break
				}
			}
		}

		if atHome {
			photosAtHome++
		}

		// Check if in trips
		if assetsInTrips[asset.ID] {
			photosInTrips++
		} else if assetsInSessions[asset.ID] {
			// In a session but not in a trip
			photosInSessionsNotTrips++
			if !atHome {
				photosAwayFromHomeNotInTrips++
			}
		} else {
			// Not in any session
			photosNotInSessions++
		}
	}

	// Print analysis
	fmt.Println("\n======================================================================")
	fmt.Println("PHOTO COVERAGE ANALYSIS")
	fmt.Println("======================================================================")
	fmt.Println()

	fmt.Printf("Total Photos:                           %d\n", len(assets))
	fmt.Println()

	fmt.Println("Location Data:")
	fmt.Printf("  Photos with GPS data:                 %d (%.1f%%)\n",
		photosWithLocation, float64(photosWithLocation)*100/float64(len(assets)))
	fmt.Printf("  Photos without GPS data:              %d (%.1f%%)\n",
		photosWithoutLocation, float64(photosWithoutLocation)*100/float64(len(assets)))
	fmt.Println()

	fmt.Println("Categorization (of photos with location):")
	if len(homes) > 0 {
		fmt.Printf("  Photos at home:                       %d (%.1f%%)\n",
			photosAtHome, float64(photosAtHome)*100/float64(photosWithLocation))
	} else {
		fmt.Println("  Photos at home:                       N/A (no home locations defined)")
	}
	fmt.Printf("  Photos in trips:                      %d (%.1f%%)\n",
		photosInTrips, float64(photosInTrips)*100/float64(photosWithLocation))
	fmt.Printf("  Photos in sessions (not trips):       %d (%.1f%%)\n",
		photosInSessionsNotTrips, float64(photosInSessionsNotTrips)*100/float64(photosWithLocation))
	if len(homes) > 0 {
		fmt.Printf("    - Away from home:                   %d (%.1f%%)\n",
			photosAwayFromHomeNotInTrips, float64(photosAwayFromHomeNotInTrips)*100/float64(photosWithLocation))
	}
	fmt.Printf("  Photos not in any session:            %d (%.1f%%)\n",
		photosNotInSessions, float64(photosNotInSessions)*100/float64(photosWithLocation))
	fmt.Println()

	fmt.Println("Summary:")
	fmt.Printf("  Sessions: %d\n", len(sessions))
	fmt.Printf("  Trips: %d\n", len(trips))
	if len(homes) > 0 {
		fmt.Printf("  Home locations: %d\n", len(homes))
	}
	fmt.Println()

	if photosAwayFromHomeNotInTrips > 0 && len(homes) > 0 {
		fmt.Println("Recommendations:")
		fmt.Printf("  %d photos are away from home but not in trips.\n", photosAwayFromHomeNotInTrips)
		fmt.Println("  These might be:")
		fmt.Println("    - Day trips that didn't meet distance/duration criteria")
		fmt.Println("    - Work, errands, or regular activities")
		fmt.Println("    - Sessions that are too short to be trips")
		fmt.Println()
		fmt.Println("  Consider:")
		fmt.Println("    - Adjusting trip detection parameters (--min-distance, --min-duration)")
		fmt.Println("    - Creating separate albums for frequent locations")
		fmt.Println("    - Adding more home locations for work/regular places")
	}

	if photosNotInSessions > 0 {
		fmt.Printf("  %d photos are not grouped into any session.\n", photosNotInSessions)
		fmt.Println("  These might be:")
		fmt.Println("    - Isolated photos taken far from other photos")
		fmt.Println("    - Photos that didn't meet session minimum criteria")
		fmt.Println()
		fmt.Println("  Consider:")
		fmt.Println("    - Lowering session detection parameters (--min-photos)")
		fmt.Println("    - Increasing time/distance thresholds for sessions")
	}

	return nil
}
