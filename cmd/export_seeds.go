package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/spf13/cobra"
)

var exportSeedsCmd = &cobra.Command{
	Use:   "export-seeds",
	Short: "Export home locations and device labels to seed files",
	Long:  `Exports your home locations and device photographer labels to JSON files for backup and restoration.`,
	RunE:  runExportSeeds,
}

func init() {
	rootCmd.AddCommand(exportSeedsCmd)
}

func runExportSeeds(cmd *cobra.Command, args []string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Export home locations
	homes, err := db.GetHomeLocations()
	if err != nil {
		return fmt.Errorf("failed to get home locations: %w", err)
	}

	homesFile, err := os.Create("seeds/home_locations.json")
	if err != nil {
		// Try to create seeds directory
		if err := os.MkdirAll("seeds", 0755); err != nil {
			return fmt.Errorf("failed to create seeds directory: %w", err)
		}
		homesFile, err = os.Create("seeds/home_locations.json")
		if err != nil {
			return fmt.Errorf("failed to create home_locations.json: %w", err)
		}
	}
	defer homesFile.Close()

	encoder := json.NewEncoder(homesFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(homes); err != nil {
		return fmt.Errorf("failed to encode home locations: %w", err)
	}

	fmt.Printf("✓ Exported %d home locations to seeds/home_locations.json\n", len(homes))

	// Export device labels
	devices, err := db.GetDevices()
	if err != nil {
		return fmt.Errorf("failed to get devices: %w", err)
	}

	// Only export devices that have labels
	type DeviceLabel struct {
		ID           string `json:"id"`
		Make         string `json:"make"`
		Model        string `json:"model"`
		Photographer string `json:"photographer"`
	}

	var labeledDevices []DeviceLabel
	for _, device := range devices {
		if device.Photographer != "" {
			labeledDevices = append(labeledDevices, DeviceLabel{
				ID:           device.ID,
				Make:         device.Make,
				Model:        device.Model,
				Photographer: device.Photographer,
			})
		}
	}

	devicesFile, err := os.Create("seeds/device_labels.json")
	if err != nil {
		return fmt.Errorf("failed to create device_labels.json: %w", err)
	}
	defer devicesFile.Close()

	encoder = json.NewEncoder(devicesFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(labeledDevices); err != nil {
		return fmt.Errorf("failed to encode device labels: %w", err)
	}

	fmt.Printf("✓ Exported %d device labels to seeds/device_labels.json\n", len(labeledDevices))
	fmt.Println("\nSeed files created successfully in seeds/ directory")

	return nil
}
