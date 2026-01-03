#!/bin/bash
# Full pipeline regeneration with fresh asset import

echo "======================================================"
echo "Immich Albums - Full Regeneration Pipeline"
echo "======================================================"
echo ""
echo "This will:"
echo "  1. Re-import all assets from Immich (2010-01-01 to 2026-01-01)"
echo "  2. Label devices (choose: import seeds or use web UI)"
echo "  3. Re-infer locations (using photographer labels)"
echo "  4. Re-detect sessions (using photographer labels)"
echo "  5. Re-detect trips (using sessions)"
echo "  6. Optionally create albums in Immich"
echo ""
echo "⚠️  WARNING: This will clear all existing data and re-fetch from Immich"
echo ""
read -p "Continue? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]
then
    exit 1
fi

echo ""
echo "[1/6] Re-importing assets from Immich..."
echo "======================================================"
./immich-albums discover --start-date 2000-01-01 --end-date 2026-01-01
if [ $? -ne 0 ]; then
    echo "Error: Asset discovery failed"
    exit 1
fi

echo ""
echo "[2/6] Configuration (Device Labels & Home Locations)"
echo "======================================================"
echo ""
echo "Choose configuration method:"
echo "  1) Import from seed files (seeds/*.json)"
echo "  2) Configure interactively via web UI (RECOMMENDED for first run)"
echo ""
read -p "Choose method (1 or 2): " -n 1 -r CONFIG_METHOD
echo ""
echo ""

if [[ $CONFIG_METHOD == "1" ]]; then
    if [ ! -f "seeds/device_labels.json" ] || [ ! -f "seeds/home_locations.json" ]; then
        echo "Error: Seed files not found!"
        echo "Required files:"
        echo "  - seeds/device_labels.json"
        echo "  - seeds/home_locations.json"
        echo ""
        echo "Run './immich-albums export-seeds' after configuring via web UI."
        exit 1
    fi

    echo "Importing seed files (device labels and home locations)..."
    ./immich-albums import-seeds
    if [ $? -ne 0 ]; then
        echo "Error: Seed import failed"
        exit 1
    fi
elif [[ $CONFIG_METHOD == "2" ]]; then
    echo "Starting web server for interactive configuration..."
    echo ""
    echo "======================================================"
    echo "INTERACTIVE CONFIGURATION"
    echo "======================================================"
    echo ""
    echo "The web UI will guide you through configuration:"
    echo ""
    echo "1. DEVICE LABELING (http://localhost:8080/devices)"
    echo "   - View sample photos from each device"
    echo "   - Assign photographer names to cameras/phones"
    echo ""
    echo "2. HOME LOCATIONS (http://localhost:8080/heatmap)"
    echo "   - View activity heatmap to identify home locations"
    echo "   - Then go to http://localhost:8080/homes"
    echo "   - Click on map to add home locations with radius"
    echo ""
    echo "======================================================"
    echo ""
    echo "Press Enter to start the web server..."
    read

    ./immich-albums serve --port 8080 &
    SERVER_PID=$!

    echo ""
    echo "Web server started (PID: $SERVER_PID)"
    echo ""
    echo "Configuration URLs:"
    echo "  1. Devices:  http://localhost:8080/devices"
    echo "  2. Heatmap:  http://localhost:8080/heatmap  (find your home)"
    echo "  3. Homes:    http://localhost:8080/homes    (add home locations)"
    echo ""
    echo "Press Enter when you've finished configuration..."
    read

    kill $SERVER_PID 2>/dev/null
    wait $SERVER_PID 2>/dev/null

    echo ""
    echo "Exporting configuration to seed files for future use..."
    ./immich-albums export-seeds

    echo ""
    echo "Configuration complete!"
else
    echo "Invalid choice. Exiting."
    exit 1
fi

echo ""
echo "[3/6] Inferring locations..."
echo "======================================================"
./immich-albums infer-locations --min-confidence 0.3
if [ $? -ne 0 ]; then
    echo "Error: Location inference failed"
    exit 1
fi

echo ""
echo "[4/6] Detecting sessions..."
echo "======================================================"
./immich-albums detect-sessions --max-time-gap 6.0 --max-distance 5.0 --min-photos 2
if [ $? -ne 0 ]; then
    echo "Error: Session detection failed"
    exit 1
fi

echo ""
echo "[5/6] Detecting trips..."
echo "======================================================"
./immich-albums detect-trips --min-distance 2.0 --max-session-gap 72.0 --min-duration 2.0 --split-date 2025-02-11 --split-date 2025-02-25
if [ $? -ne 0 ]; then
    echo "Error: Trip detection failed"
    exit 1
fi

echo ""
echo "[6/6] Creating albums in Immich..."
echo "======================================================"
echo "Do you want to create/update albums in Immich? (y/n)"
read -p "Create albums? " -n 1 -r CREATE_ALBUMS
echo ""
if [[ $CREATE_ALBUMS =~ ^[Yy]$ ]]; then
    ./immich-albums create-albums --recreate
    if [ $? -ne 0 ]; then
        echo "Warning: Album creation failed (continuing anyway)"
    fi
else
    echo "Skipping album creation."
fi

echo ""
echo "======================================================"
echo "✓ Regeneration complete!"
echo "======================================================"
echo ""
echo "Next steps:"
echo "  - View results: ./immich-albums serve --port 8080"
echo "  - Then visit: http://localhost:8080/trips"
echo ""
