package cmd

import (
	"fmt"
	"time"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/jamo/immich-albums/internal/immich"
	"github.com/jamo/immich-albums/internal/models"
	"github.com/jamo/immich-albums/internal/processor"
	"github.com/spf13/cobra"
)

var (
	startDate string
	endDate   string
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover devices and fetch photos from Immich",
	Long: `Fetches photos from Immich for the specified date range,
discovers all unique camera and phone models, and stores metadata locally.`,
	RunE: runDiscover,
}

func init() {
	rootCmd.AddCommand(discoverCmd)

	discoverCmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	discoverCmd.Flags().StringVar(&endDate, "end-date", "", "End date (YYYY-MM-DD)")

	discoverCmd.MarkFlagRequired("start-date")
	discoverCmd.MarkFlagRequired("end-date")
}

func runDiscover(cmd *cobra.Command, args []string) error {
	// Parse dates
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}

	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return fmt.Errorf("invalid end date: %w", err)
	}

	// Initialize database
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Initialize Immich client
	client := immich.NewClient(immichURL, immichAPIKey)

	// Fetch assets
	fmt.Printf("Fetching assets from %s to %s...\n", startDate, endDate)
	assets, err := client.FetchAssets(start, end)
	if err != nil {
		return fmt.Errorf("failed to fetch assets: %w", err)
	}

	fmt.Printf("Fetched %d assets\n", len(assets))

	// Validate timestamps and filter out invalid assets
	fmt.Println("Validating asset timestamps...")
	validAssets := make([]models.Asset, 0, len(assets))
	invalidCount := 0
	for _, asset := range assets {
		// Check for zero/invalid timestamp
		if asset.LocalDateTime.IsZero() {
			invalidCount++
			continue
		}
		// Check for unreasonable timestamps (before year 1900 or after 2100)
		year := asset.LocalDateTime.Year()
		if year < 1900 || year > 2100 {
			invalidCount++
			continue
		}
		validAssets = append(validAssets, asset)
	}

	if invalidCount > 0 {
		fmt.Printf("Warning: Skipped %d assets with invalid timestamps\n", invalidCount)
	}
	fmt.Printf("Valid assets: %d\n", len(validAssets))

	// Store assets in database
	fmt.Println("Storing assets in database...")
	if err := db.StoreAssets(validAssets); err != nil {
		return fmt.Errorf("failed to store assets: %w", err)
	}

	// Discover devices
	fmt.Println("\nDiscovering devices...")
	devices := processor.DiscoverDevices(validAssets)

	fmt.Printf("\nFound %d unique devices:\n", len(devices))
	for _, device := range devices {
		fmt.Printf("  - %s (Model: %s, Make: %s) - %d photos\n",
			device.ID, device.Model, device.Make, device.PhotoCount)
	}

	// Store devices
	if err := db.StoreDevices(devices); err != nil {
		return fmt.Errorf("failed to store devices: %w", err)
	}

	fmt.Println("\nRun 'immich-albums label-devices' to assign photographers to devices")

	return nil
}
