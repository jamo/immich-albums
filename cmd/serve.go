package cmd

import (
	"fmt"
	"net/http"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/jamo/immich-albums/internal/web"
	"github.com/spf13/cobra"
)

var (
	port int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web UI for visualization and home labeling",
	Long: `Starts a web server with an interactive UI to:
  - Visualize sessions on a map
  - View activity heatmap to identify home locations
  - Label home locations
  - Review and adjust parameters`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().IntVar(&port, "port", 8080, "Port to run web server on")
}

func runServe(cmd *cobra.Command, args []string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create web server
	server := web.NewServer(db, immichURL, immichAPIKey)

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Starting web server on http://localhost%s\n", addr)
	fmt.Println("\nAvailable pages:")
	fmt.Println("  - http://localhost:8080/         - Dashboard")
	fmt.Println("  - http://localhost:8080/sessions - Sessions map")
	fmt.Println("  - http://localhost:8080/heatmap  - Activity heatmap")
	fmt.Println("  - http://localhost:8080/homes    - Home locations")
	fmt.Println("  - http://localhost:8080/trips    - Detected trips")
	fmt.Println("  - http://localhost:8080/coverage - Photo coverage analysis")
	fmt.Println("  - http://localhost:8080/devices  - Label devices")
	fmt.Println("\nPress Ctrl+C to stop")

	if err := http.ListenAndServe(addr, server); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
