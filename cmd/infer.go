package cmd

import (
	"fmt"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/jamo/immich-albums/internal/processor"
	"github.com/spf13/cobra"
)

var (
	minConfidence float64
)

var inferCmd = &cobra.Command{
	Use:   "infer-locations",
	Short: "Infer locations for photos without GPS data",
	Long: `Analyzes photos and infers locations for DSLR images without GPS
by using nearby phone photos from the same photographer. Handles gaps of days
between photos with confidence scoring.`,
	RunE: runInfer,
}

func init() {
	rootCmd.AddCommand(inferCmd)

	inferCmd.Flags().Float64Var(&minConfidence, "min-confidence", 0.3, "Minimum confidence score (0.0-1.0)")
}

func runInfer(cmd *cobra.Command, args []string) error {
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
	fmt.Println("Loading device labels...")
	devices, err := db.GetDevices()
	if err != nil {
		return fmt.Errorf("failed to get devices: %w", err)
	}

	// Check if devices are labeled
	labeledCount := 0
	for _, device := range devices {
		if device.Photographer != "" {
			labeledCount++
		}
	}

	if labeledCount == 0 {
		return fmt.Errorf("no devices have been labeled with photographers. Run 'label-devices' first")
	}

	fmt.Printf("Found %d labeled devices out of %d total\n", labeledCount, len(devices))

	// Infer locations
	fmt.Println("\nInferring locations...")
	inferences := processor.InferLocations(assets, devices)

	// Filter by minimum confidence
	filtered := 0
	for _, inf := range inferences {
		if inf.Confidence >= minConfidence {
			filtered++
		}
	}

	fmt.Printf("\nInferences with confidence >= %.2f: %d\n", minConfidence, filtered)

	// Store inferences in database
	fmt.Println("Storing inferences in database...")
	if err := storeInferences(db, inferences, minConfidence); err != nil {
		return fmt.Errorf("failed to store inferences: %w", err)
	}

	// Print summary by confidence level
	fmt.Println("\nConfidence distribution:")
	confidenceBuckets := map[string]int{
		"Very High (0.9-1.0)": 0,
		"High (0.7-0.9)":      0,
		"Good (0.5-0.7)":      0,
		"Moderate (0.3-0.5)":  0,
		"Low (0.1-0.3)":       0,
	}

	for _, inf := range inferences {
		switch {
		case inf.Confidence >= 0.9:
			confidenceBuckets["Very High (0.9-1.0)"]++
		case inf.Confidence >= 0.7:
			confidenceBuckets["High (0.7-0.9)"]++
		case inf.Confidence >= 0.5:
			confidenceBuckets["Good (0.5-0.7)"]++
		case inf.Confidence >= 0.3:
			confidenceBuckets["Moderate (0.3-0.5)"]++
		default:
			confidenceBuckets["Low (0.1-0.3)"]++
		}
	}

	for level, count := range confidenceBuckets {
		if count > 0 {
			fmt.Printf("  %s: %d\n", level, count)
		}
	}

	fmt.Println("\n✓ Location inference complete!")
	fmt.Println("Next: Run 'detect-sessions' to group photos into sessions")

	return nil
}

func storeInferences(db *database.DB, inferences []processor.LocationInference, minConfidence float64) error {
	tx, err := db.BeginTx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		UPDATE assets
		SET inferred_latitude = ?, inferred_longitude = ?, location_confidence = ?, location_source = ?
		WHERE id = ?
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	count := 0
	totalToStore := 0
	for _, inf := range inferences {
		if inf.Confidence >= minConfidence {
			totalToStore++
		}
	}

	for _, inf := range inferences {
		if inf.Confidence < minConfidence {
			continue
		}

		// Progress indicator every 500 inferences
		if count > 0 && count%500 == 0 {
			fmt.Printf("  Storing: %d/%d (%.1f%%)\r", count, totalToStore, float64(count)*100/float64(totalToStore))
		}

		_, err := stmt.Exec(inf.Latitude, inf.Longitude, inf.Confidence, inf.Source, inf.AssetID)
		if err != nil {
			return err
		}
		count++
	}
	if totalToStore > 0 {
		fmt.Printf("  Storing: %d/%d (100.0%%)  \n", count, totalToStore)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	fmt.Printf("Stored %d inferences in database\n", count)
	return nil
}
