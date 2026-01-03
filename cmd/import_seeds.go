package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/jamo/immich-albums/internal/models"
	"github.com/spf13/cobra"
)

var importSeedsCmd = &cobra.Command{
	Use:   "import-seeds",
	Short: "Import home locations and device labels from seed files",
	Long:  `Imports home locations and device photographer labels from JSON seed files.`,
	RunE:  runImportSeeds,
}

func init() {
	rootCmd.AddCommand(importSeedsCmd)
}

func runImportSeeds(cmd *cobra.Command, args []string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Import home locations
	homesFile, err := os.Open("seeds/home_locations.json")
	if err != nil {
		return fmt.Errorf("failed to open seeds/home_locations.json: %w", err)
	}
	defer homesFile.Close()

	var homes []models.HomeLocation
	decoder := json.NewDecoder(homesFile)
	if err := decoder.Decode(&homes); err != nil {
		return fmt.Errorf("failed to decode home locations: %w", err)
	}

	// Clear existing homes and import
	if _, err := db.Exec("DELETE FROM home_locations"); err != nil {
		return fmt.Errorf("failed to clear home locations: %w", err)
	}

	for _, home := range homes {
		if err := db.StoreHomeLocation(home); err != nil {
			return fmt.Errorf("failed to store home location: %w", err)
		}
	}

	fmt.Printf("✓ Imported %d home locations\n", len(homes))

	// Import device labels
	devicesFile, err := os.Open("seeds/device_labels.json")
	if err != nil {
		return fmt.Errorf("failed to open seeds/device_labels.json: %w", err)
	}
	defer devicesFile.Close()

	type DeviceLabel struct {
		ID           string `json:"id"`
		Make         string `json:"make"`
		Model        string `json:"model"`
		Photographer string `json:"photographer"`
	}

	var deviceLabels []DeviceLabel
	decoder = json.NewDecoder(devicesFile)
	if err := decoder.Decode(&deviceLabels); err != nil {
		return fmt.Errorf("failed to decode device labels: %w", err)
	}

	// Update device labels
	for _, label := range deviceLabels {
		if err := db.UpdateDevicePhotographer(label.ID, label.Photographer); err != nil {
			return fmt.Errorf("failed to update photographer for %s: %w", label.ID, err)
		}
	}

	fmt.Printf("✓ Imported %d device labels\n", len(deviceLabels))
	fmt.Println("\nSeed files imported successfully!")

	return nil
}
