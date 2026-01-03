package cmd

import (
	"fmt"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/jamo/immich-albums/internal/models"
	"github.com/jamo/immich-albums/internal/processor"
	"github.com/spf13/cobra"
)

var (
	maxTimeGap       float64
	maxDistance      float64
	minPhotos        int
	mergeSessions    bool
	mergeTimeGap     float64
	mergeDistance    float64
)

var sessionsCmd = &cobra.Command{
	Use:   "detect-sessions",
	Short: "Detect photo sessions using spatial-temporal clustering",
	Long: `Groups photos into sessions based on time proximity and geographic location.
Sessions are detected per photographer and can optionally be merged across photographers.`,
	RunE: runSessions,
}

func init() {
	rootCmd.AddCommand(sessionsCmd)

	sessionsCmd.Flags().Float64Var(&maxTimeGap, "max-time-gap", 6.0, "Maximum time gap between photos in hours")
	sessionsCmd.Flags().Float64Var(&maxDistance, "max-distance", 5.0, "Maximum distance between photos in km")
	sessionsCmd.Flags().IntVar(&minPhotos, "min-photos", 2, "Minimum photos required for a session")
	sessionsCmd.Flags().BoolVar(&mergeSessions, "merge", false, "Merge sessions from different photographers")
	sessionsCmd.Flags().Float64Var(&mergeTimeGap, "merge-time-gap", 2.0, "Time gap for merging sessions in hours")
	sessionsCmd.Flags().Float64Var(&mergeDistance, "merge-distance", 1.0, "Distance for merging sessions in km")
}

func runSessions(cmd *cobra.Command, args []string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Load assets
	fmt.Println("Loading assets from database...")
	assets, err := db.GetAssets()
	if err != nil {
		return fmt.Errorf("failed to get assets: %w", err)
	}
	fmt.Printf("Loaded %d assets\n", len(assets))

	// Load devices
	devices, err := db.GetDevices()
	if err != nil {
		return fmt.Errorf("failed to get devices: %w", err)
	}

	// Create device map
	deviceMap := make(map[string]models.Device)
	for _, d := range devices {
		deviceMap[d.ID] = d
	}

	// Build inference map from database
	// For now, the inferred locations are stored directly in the assets table
	// We'll build the map by checking which assets have inferred locations
	inferenceMap := make(map[string]processor.LocationInference)
	// TODO: Load inferred locations from database into inferenceMap

	// Set clustering parameters
	params := processor.ClusteringParams{
		MaxTimeGapHours:    maxTimeGap,
		MaxDistanceKM:      maxDistance,
		MinPhotosInSession: minPhotos,
		MinConfidence:      0.3,
	}

	fmt.Println("\nDetecting sessions...")
	fmt.Printf("Parameters:\n")
	fmt.Printf("  Max time gap: %.1f hours\n", params.MaxTimeGapHours)
	fmt.Printf("  Max distance: %.1f km\n", params.MaxDistanceKM)
	fmt.Printf("  Min photos: %d\n", params.MinPhotosInSession)
	fmt.Printf("  Min confidence: %.2f\n", params.MinConfidence)

	sessions := processor.DetectSessions(assets, inferenceMap, deviceMap, params)

	if mergeSessions && len(sessions) > 1 {
		fmt.Printf("\nMerging sessions across photographers...\n")
		fmt.Printf("  Merge time gap: %.1f hours\n", mergeTimeGap)
		fmt.Printf("  Merge distance: %.1f km\n", mergeDistance)

		sessions = processor.MergeSessions(sessions, mergeTimeGap, mergeDistance)
		fmt.Printf("After merging: %d sessions\n", len(sessions))
	}

	// Store sessions
	fmt.Println("\nStoring sessions in database...")
	if err := db.StoreSessions(sessions); err != nil {
		return fmt.Errorf("failed to store sessions: %w", err)
	}

	// Print summary
	fmt.Println("\nSession Summary:")
	fmt.Printf("  Total sessions: %d\n", len(sessions))

	totalPhotos := 0
	for _, session := range sessions {
		totalPhotos += len(session.AssetIDs)
	}
	fmt.Printf("  Total photos in sessions: %d\n", totalPhotos)

	if len(sessions) > 0 {
		avgPhotos := float64(totalPhotos) / float64(len(sessions))
		fmt.Printf("  Average photos per session: %.1f\n", avgPhotos)
	}

	fmt.Println("\n✓ Session detection complete!")
	fmt.Println("Next: Run 'serve' to visualize sessions and label home locations")

	return nil
}
