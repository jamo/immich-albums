package cmd

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	immichURL    string
	immichAPIKey string
	dbPath       string
)

var rootCmd = &cobra.Command{
	Use:   "immich-albums",
	Short: "Intelligently create Immich albums from photo trips",
	Long: `Immich Albums analyzes your photo collection to automatically detect trips
and create albums. It uses location data, device information, and temporal
clustering to group photos into meaningful collections.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Load .env file if it exists
	godotenv.Load()

	rootCmd.PersistentFlags().StringVar(&immichURL, "immich-url", os.Getenv("IMMICH_URL"), "Immich instance URL (can be set via IMMICH_URL env var)")
	rootCmd.PersistentFlags().StringVar(&immichAPIKey, "api-key", os.Getenv("IMMICH_API_KEY"), "Immich API key (can be set via IMMICH_API_KEY env var)")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "./immich-albums.db", "Path to local SQLite database")

	// Add a pre-run check to ensure credentials are provided
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if immichURL == "" {
			return fmt.Errorf("immich-url is required (use --immich-url flag or IMMICH_URL env var)")
		}
		if immichAPIKey == "" {
			return fmt.Errorf("api-key is required (use --api-key flag or IMMICH_API_KEY env var)")
		}
		return nil
	}
}
