package processor

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jamo/immich-albums/internal/models"
)

// DiscoverDevices analyzes assets and returns unique devices
func DiscoverDevices(assets []models.Asset) []models.Device {
	skippedCount := 0

	// Group assets by make/model first
	makeModelGroups := make(map[string][]models.Asset)
	for _, asset := range assets {
		if asset.Make == "" && asset.Model == "" {
			skippedCount++
			continue
		}
		key := makeDeviceID(asset.Make, asset.Model)
		makeModelGroups[key] = append(makeModelGroups[key], asset)
	}

	fmt.Printf("Device discovery stats:\n")
	fmt.Printf("  Assets with Make/Model: %d\n", len(assets)-skippedCount)
	fmt.Printf("  Assets skipped (no device info): %d\n", skippedCount)
	fmt.Printf("  Unique Make/Model combinations: %d\n", len(makeModelGroups))

	// For each make/model group, try to identify sub-devices based on temporal patterns
	var devices []models.Device
	for makeModel, groupAssets := range makeModelGroups {
		subDevices := identifySubDevices(makeModel, groupAssets)
		devices = append(devices, subDevices...)
	}

	fmt.Printf("  Total devices after temporal analysis: %d\n", len(devices))

	return devices
}

// makeDeviceID creates a fallback device ID from make/model (for backwards compatibility)
func makeDeviceID(make, model string) string {
	// Normalize strings
	make = strings.TrimSpace(strings.ToLower(make))
	model = strings.TrimSpace(strings.ToLower(model))

	if make == "" && model == "" {
		return "unknown"
	}

	return fmt.Sprintf("%s-%s", make, model)
}

// identifySubDevices tries to identify multiple physical devices with the same make/model
// Uses filename counter distribution to find distinct counter ranges representing different devices
func identifySubDevices(makeModel string, assets []models.Asset) []models.Device {
	// If only a few assets, don't bother splitting
	if len(assets) < 20 {
		return []models.Device{{
			ID:         makeModel,
			Make:       assets[0].Make,
			Model:      assets[0].Model,
			PhotoCount: len(assets),
		}}
	}

	// Extract filename counters
	type assetWithCounter struct {
		asset      models.Asset
		counter    int
		hasCounter bool
	}

	var withCounters []assetWithCounter
	for _, asset := range assets {
		counter, hasCounter := extractFilenameCounter(asset.OriginalFileName)
		if hasCounter {
			withCounters = append(withCounters, assetWithCounter{
				asset:      asset,
				counter:    counter,
				hasCounter: true,
			})
		}
	}

	// Need enough samples to analyze distribution
	if len(withCounters) < 10 {
		return []models.Device{{
			ID:         makeModel,
			Make:       assets[0].Make,
			Model:      assets[0].Model,
			PhotoCount: len(assets),
		}}
	}

	// Sort by counter value to find clusters
	sort.Slice(withCounters, func(i, j int) bool {
		return withCounters[i].counter < withCounters[j].counter
	})

	// Find gaps in counter distribution to identify distinct devices
	// A large gap suggests different counter ranges from different devices
	type counterCluster struct {
		minCounter int
		maxCounter int
		assets     []models.Asset
	}

	clusters := []counterCluster{{
		minCounter: withCounters[0].counter,
		maxCounter: withCounters[0].counter,
		assets:     []models.Asset{withCounters[0].asset},
	}}

	currentCluster := 0

	for i := 1; i < len(withCounters); i++ {
		curr := withCounters[i]
		prevCounter := withCounters[i-1].counter
		gap := curr.counter - prevCounter

		// Large gap suggests different device - use adaptive threshold
		// Gap needs to be both large in absolute terms (>1000) and relative (>10x typical increment)
		clusterRange := clusters[currentCluster].maxCounter - clusters[currentCluster].minCounter
		typicalIncrement := clusterRange / max(len(clusters[currentCluster].assets), 1)

		shouldSplit := gap > 1000 && (typicalIncrement == 0 || gap > typicalIncrement*20)

		if shouldSplit {
			// Start new cluster
			clusters = append(clusters, counterCluster{
				minCounter: curr.counter,
				maxCounter: curr.counter,
				assets:     []models.Asset{curr.asset},
			})
			currentCluster++
		} else {
			// Add to current cluster
			clusters[currentCluster].maxCounter = curr.counter
			clusters[currentCluster].assets = append(clusters[currentCluster].assets, curr.asset)
		}
	}

	// Filter out very small clusters (likely noise/chat apps)
	var significantClusters []counterCluster
	for _, cluster := range clusters {
		if len(cluster.assets) >= 5 { // Need at least 5 photos to be a real device
			significantClusters = append(significantClusters, cluster)
		}
	}

	// If we filtered everything out, just use one device
	if len(significantClusters) == 0 {
		return []models.Device{{
			ID:         makeModel,
			Make:       assets[0].Make,
			Model:      assets[0].Model,
			PhotoCount: len(assets),
		}}
	}

	// Create device entries and store their counter ranges
	var devices []models.Device
	for i, cluster := range significantClusters {
		deviceID := makeModel
		if len(significantClusters) > 1 {
			deviceID = fmt.Sprintf("%s-device%d", makeModel, i+1)
		}

		// Store counter range for this device (for matching assets later)
		deviceCounterRanges[deviceID] = struct{ min, max int }{
			min: cluster.minCounter,
			max: cluster.maxCounter,
		}

		devices = append(devices, models.Device{
			ID:         deviceID,
			Make:       cluster.assets[0].Make,
			Model:      cluster.assets[0].Model,
			PhotoCount: len(cluster.assets),
		})
	}

	if len(devices) > 1 {
		fmt.Printf("    %s split into %d devices based on counter ranges:\n", makeModel, len(devices))
		for i, cluster := range significantClusters {
			fmt.Printf("      device%d: counters %d-%d (%d photos)\n",
				i+1, cluster.minCounter, cluster.maxCounter, len(cluster.assets))
		}
	}

	return devices
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// extractFilenameCounter extracts numeric counter from common filename patterns
// Examples: IMG_1234.jpg -> 1234, DSC_5678.NEF -> 5678, PXL_20240101_123456.jpg -> 20240101123456
func extractFilenameCounter(filename string) (int, bool) {
	// Common patterns:
	// IMG_XXXX, DSC_XXXX, _MG_XXXX, etc.
	patterns := []string{
		`IMG_(\d+)`,
		`DSC_(\d+)`,
		`_MG_(\d+)`,
		`DSCF(\d+)`,
		`P\d+_(\d+)`,
		`PXL_(\d{8})_(\d{6})`, // Pixel phones
		`(\d{8}_\d{6})`, // Generic timestamp pattern
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(filename)
		if len(matches) > 1 {
			// Use first captured group
			if num, err := strconv.Atoi(matches[1]); err == nil {
				return num, true
			}
		}
	}

	return 0, false
}

// deviceCounterRanges stores the counter ranges for each device (populated during discovery)
var deviceCounterRanges = make(map[string]struct{ min, max int })

// FindMatchingDevice finds the correct device ID for an asset
// When there are multiple sub-devices (e.g., apple-iphone 13-device1, apple-iphone 13-device2),
// we match based on the filename counter range
func FindMatchingDevice(asset models.Asset, devices []models.Device) string {
	// Convert slice to map for easier lookup
	deviceMap := make(map[string]models.Device)
	for _, d := range devices {
		deviceMap[d.ID] = d
	}

	return findMatchingDeviceMap(asset, deviceMap)
}

// findMatchingDeviceMap is the internal version that works with a map
func findMatchingDeviceMap(asset models.Asset, devices map[string]models.Device) string {
	baseDeviceID := makeDeviceID(asset.Make, asset.Model)

	// Check if base device exists (no sub-devices)
	if _, exists := devices[baseDeviceID]; exists {
		return baseDeviceID
	}

	// Extract counter from asset filename
	counter, hasCounter := extractFilenameCounter(asset.OriginalFileName)
	if !hasCounter {
		// No counter, assign to first matching device
		for deviceID := range devices {
			if strings.HasPrefix(deviceID, baseDeviceID+"-device") {
				return deviceID
			}
		}
		return ""
	}

	// Find device whose counter range contains this asset's counter
	for deviceID := range devices {
		if strings.HasPrefix(deviceID, baseDeviceID+"-device") {
			if counterRange, exists := deviceCounterRanges[deviceID]; exists {
				// Check if counter falls within this device's range (with some tolerance)
				tolerance := (counterRange.max - counterRange.min) / 4 // 25% tolerance
				if counter >= counterRange.min-tolerance && counter <= counterRange.max+tolerance {
					return deviceID
				}
			}
		}
	}

	// Fallback: return first matching device
	for deviceID := range devices {
		if strings.HasPrefix(deviceID, baseDeviceID+"-device") {
			return deviceID
		}
	}

	return ""
}
