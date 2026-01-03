package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/jamo/immich-albums/internal/database"
	"github.com/jamo/immich-albums/internal/models"
	"github.com/spf13/cobra"
)

var (
	labelAll bool
)

var labelCmd = &cobra.Command{
	Use:   "label-devices",
	Short: "Label devices with photographer names",
	Long: `Interactive tool to assign photographer names to discovered devices.
This helps the system understand which cameras and phones belong to which people.`,
	RunE: runLabel,
}

func init() {
	rootCmd.AddCommand(labelCmd)
	labelCmd.Flags().BoolVar(&labelAll, "all", false, "Show all devices including already labeled ones")
}

func runLabel(cmd *cobra.Command, args []string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	allDevices, err := db.GetDevices()
	if err != nil {
		return fmt.Errorf("failed to get devices: %w", err)
	}

	if len(allDevices) == 0 {
		fmt.Println("No devices found. Run 'discover' first.")
		return nil
	}

	// Count labeled vs unlabeled
	var labeled, unlabeled int
	var devicesToLabel []models.Device
	for _, device := range allDevices {
		if device.Photographer != "" {
			labeled++
			if labelAll {
				devicesToLabel = append(devicesToLabel, device)
			}
		} else {
			unlabeled++
			devicesToLabel = append(devicesToLabel, device)
		}
	}

	// Show summary
	fmt.Println("Device Labeling")
	fmt.Println("===============")
	fmt.Printf("Total devices: %d\n", len(allDevices))
	fmt.Printf("  Already labeled: %d\n", labeled)
	fmt.Printf("  Unlabeled: %d\n", unlabeled)
	fmt.Println()

	if unlabeled == 0 && !labelAll {
		fmt.Println("All devices are already labeled!")
		fmt.Println("Use --all flag to relabel devices.")
		return nil
	}

	if len(devicesToLabel) == 0 {
		fmt.Println("No devices to label.")
		return nil
	}

	reader := bufio.NewReader(os.Stdin)

	if labelAll {
		fmt.Println("Showing ALL devices (including already labeled).")
	} else {
		fmt.Println("Showing only UNLABELED devices.")
		fmt.Println("Use --all flag to show all devices.")
	}
	fmt.Println("For each device, enter the photographer name (or press Enter to skip)")
	fmt.Println()

	for i, device := range devicesToLabel {
		fmt.Printf("\n[%d/%d] Device: %s %s\n", i+1, len(devicesToLabel), device.Make, device.Model)
		fmt.Printf("       Photos: %d\n", device.PhotoCount)

		if device.Photographer != "" {
			fmt.Printf("       Current photographer: %s\n", device.Photographer)
		}

		// Get photographer name
		fmt.Print("  Photographer name: ")
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)

		if name != "" {
			if err := db.UpdateDevicePhotographer(device.ID, name); err != nil {
				return fmt.Errorf("failed to update photographer: %w", err)
			}
			fmt.Printf("  ✓ Set photographer to: %s\n", name)
		}
	}

	fmt.Println("\n✓ Device labeling complete!")
	return nil
}
