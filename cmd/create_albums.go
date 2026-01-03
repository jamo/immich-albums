package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/jamo/immich-albums/internal/immich"
	"github.com/spf13/cobra"
)

var (
	recreate bool
)

var createAlbumsCmd = &cobra.Command{
	Use:   "create-albums",
	Short: "Create albums in Immich from detected trips",
	Long: `Creates albums in Immich for each detected trip.
Albums are marked with their IDs so they can be regenerated if needed.`,
	RunE: runCreateAlbums,
}

func init() {
	rootCmd.AddCommand(createAlbumsCmd)

	createAlbumsCmd.Flags().BoolVar(&recreate, "recreate", false, "Delete and recreate existing albums")
}

func runCreateAlbums(cmd *cobra.Command, args []string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Load trips
	fmt.Println("Loading trips from database...")
	trips, err := db.GetTrips()
	if err != nil {
		return fmt.Errorf("failed to get trips: %w", err)
	}

	if len(trips) == 0 {
		fmt.Println("No trips found. Run 'detect-trips' first.")
		return nil
	}

	fmt.Printf("Found %d trips\n\n", len(trips))

	// Create Immich client
	client := immich.NewClient(immichURL, immichAPIKey)

	created := 0
	updated := 0
	skipped := 0
	errors := 0

	for i, trip := range trips {
		fmt.Printf("[%d/%d] Processing: %s\n", i+1, len(trips), trip.Name)
		fmt.Printf("        Photos: %d\n", len(trip.AssetIDs))

		// Check if trip is excluded from album creation
		if trip.ExcludeFromAlbum {
			fmt.Println("        ⏭️  Trip excluded from album creation, skipping")
			skipped++
			continue
		}

		// Check if album already exists
		if trip.AlbumID != "" {
			if recreate {
				fmt.Printf("        Deleting existing album (ID: %s)...\n", trip.AlbumID)
				if err := client.DeleteAlbum(trip.AlbumID); err != nil {
					fmt.Printf("        ⚠️  Warning: Failed to delete album: %v\n", err)
					// Continue anyway - album might not exist anymore
				}
				trip.AlbumID = ""
			} else {
				fmt.Printf("        ⏭️  Album already exists (ID: %s), skipping\n", trip.AlbumID)
				fmt.Println("        Use --recreate flag to delete and recreate albums")
				skipped++
				continue
			}
		}

		// Create album description
		duration := trip.EndTime.Sub(trip.StartTime)
		var durationStr string
		if duration > 24*time.Hour {
			durationStr = fmt.Sprintf("%.1f days", duration.Hours()/24)
		} else {
			durationStr = fmt.Sprintf("%.1f hours", duration.Hours())
		}

		description := fmt.Sprintf("%s - %s (%s)\n%d photos by %s\nDistance: %.0fkm from home, %.0fkm traveled",
			trip.StartTime.Format("Jan 2, 2006"),
			trip.EndTime.Format("Jan 2, 2006"),
			durationStr,
			len(trip.AssetIDs),
			trip.Photographers,
			trip.HomeDistance,
			trip.TotalDistance,
		)

		// Create album
		fmt.Println("        Creating album in Immich...")
		albumID, err := client.CreateAlbum(trip.Name, description)
		if err != nil {
			fmt.Printf("        ❌ Error creating album: %v\n", err)
			errors++
			continue
		}

		fmt.Printf("        Album created with ID: %s\n", albumID)

		// Add assets to album
		if len(trip.AssetIDs) > 0 {
			fmt.Printf("        Adding %d photos to album...\n", len(trip.AssetIDs))
			if err := client.AddAssetsToAlbum(albumID, trip.AssetIDs); err != nil {
				fmt.Printf("        ⚠️  Warning: Failed to add assets: %v\n", err)
				// Album was created, so still update the ID
			}
		}

		// Update trip with album ID
		if err := db.UpdateTripAlbumID(trip.ID, albumID); err != nil {
			fmt.Printf("        ⚠️  Warning: Failed to save album ID: %v\n", err)
		}

		fmt.Println("        ✓ Complete!")

		if trip.AlbumID != "" && recreate {
			updated++
		} else {
			created++
		}
	}

	// Print summary
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("ALBUM CREATION SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total trips: %d\n", len(trips))
	fmt.Printf("  Albums created: %d\n", created)
	if updated > 0 {
		fmt.Printf("  Albums recreated: %d\n", updated)
	}
	if skipped > 0 {
		fmt.Printf("  Albums skipped: %d\n", skipped)
	}
	if errors > 0 {
		fmt.Printf("  Errors: %d\n", errors)
	}

	fmt.Println("\n✓ Album creation complete!")

	return nil
}
