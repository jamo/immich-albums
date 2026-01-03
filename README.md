# Immich Albums - Intelligent Trip Detection

Automatically create Immich albums from photo trips using location data, device information, and temporal clustering.

## Features

- **Smart Device Discovery**: Identifies all cameras and phones, including multiple devices of the same model using filename counter analysis
- **Location Inference**: Infers location for DSLR photos based on nearby phone photos from the same photographer
- **Confidence Scoring**: Tracks confidence levels for inferred locations (handles gaps of days)
- **Session Detection**: Groups photos into sessions based on spatial-temporal patterns
- **Trip Detection**: Identifies trips based on distance from home and session patterns
- **Home Detection**: Learn home locations to distinguish trips from daily activities
- **Interactive Web UI**: Device labeling with photo previews, activity heatmap, trip visualization with routes
- **Album Creation**: Automatically create albums in Immich with smart naming and descriptions

## Installation

```bash
# Install dependencies
go mod download

# Build the binary
go build -o immich-albums

# Or run directly
go run main.go --help
```

## Quick Reference

```bash
# Full automated pipeline (recommended for first run)
./regenerate.sh

# Manual commands
./immich-albums discover --start-date 2000-01-01 --end-date 2026-01-01
./immich-albums serve --port 8080  # Open http://localhost:8080
./immich-albums infer-locations --min-confidence 0.3
./immich-albums detect-sessions --max-time-gap 6.0 --max-distance 5.0
./immich-albums detect-trips --min-distance 50.0 --max-session-gap 48.0
./immich-albums create-albums
./immich-albums create-albums --recreate  # Delete and recreate albums

# Configuration management
./immich-albums export-seeds  # Save device labels and home locations
./immich-albums import-seeds  # Restore from seed files
```

## Quick Start

### Setup

1. Create a `.env` file with your Immich credentials:

```bash
IMMICH_URL=https://your-immich-instance.com
IMMICH_API_KEY=your-api-key-here
```

### Recommended: Full Pipeline with Interactive Configuration

The easiest way to get started is using the automated regeneration script:

```bash
./regenerate.sh
```

This interactive script will guide you through the entire process:

1. **Import Assets** - Fetches all photos from Immich (2000-01-01 to 2026-01-01)
2. **Configure Devices & Homes** - Interactive web UI for labeling
3. **Infer Locations** - Assigns locations to DSLR photos
4. **Detect Sessions** - Groups photos by time and location
5. **Detect Trips** - Identifies trips based on home distance
6. **Create Albums** - Generates albums in Immich

See [Full Pipeline Regeneration](#full-pipeline-regeneration) section for details.

### Manual Workflow (Step by Step)

#### 1. Discover Devices

Fetch photos from your Immich instance and discover all devices:

```bash
./immich-albums discover --start-date 2024-01-01 --end-date 2026-01-01
```

This will:

- Fetch all assets in the date range
- Extract device information (make, model, GPS capability)
- Store metadata in a local SQLite database (`immich-albums.db`)

#### 2. Label Devices (Interactive Web UI)

Start the web server and label devices with photo previews:

```bash
./immich-albums serve --port 8080
```

Then visit http://localhost:8080/devices to:

- View sample photos from each device (loaded from Immich)
- Assign photographer names to cameras and phones
- See device make, model, and photo count
- Distinguish between multiple devices of the same model

The system uses filename counter analysis to identify multiple physical devices with the same make/model (e.g., multiple iPhones).

#### 3. Infer Locations

Infer locations for DSLR photos based on nearby phone photos:

```bash
./immich-albums infer-locations --min-confidence 0.3
```

Features:

- Handles gaps of **days** between phone and DSLR photos
- Confidence scoring (1.0 = same hour, 0.5 = 3 days, 0.15 = 14 days)
- Interpolation between known locations
- Adjustable confidence threshold

#### 4. Detect Sessions

Group photos into sessions based on time and location:

```bash
./immich-albums detect-sessions \
  --max-time-gap 6.0 \
  --max-distance 5.0 \
  --min-photos 2
```

Options:

- `--max-time-gap`: Maximum hours between photos in same session (default: 6)
- `--max-distance`: Maximum km between photos (default: 5)
- `--min-photos`: Minimum photos to form a session (default: 2)
- `--merge`: Merge sessions from different photographers

#### 5. Define Home Locations

Use the web UI to identify and label your home locations:

```bash
./immich-albums serve --port 8080
```

Visit http://localhost:8080/heatmap to view your activity heatmap, then go to http://localhost:8080/homes to:

- Toggle the activity heatmap overlay to see where you spend most time
- Click on the map to add home locations (home, office, parents' house, etc.)
- Set a radius for each location (default 2km)
- Home locations are used to distinguish trips from daily activities

#### 6. Detect Trips

Identify trips based on distance from home and session patterns:

```bash
./immich-albums detect-trips \
  --min-distance 50.0 \
  --max-session-gap 48.0 \
  --max-home-stay 36.0 \
  --min-duration 2.0 \
  --min-sessions 1 \
  --split-date 2025-02-11
```

Options:

- `--min-distance`: Minimum distance from home in km (default: 50)
- `--max-session-gap`: Maximum hours between sessions in same trip (default: 48)
- `--max-home-stay`: Maximum hours at home before trip splits (default: 36, allows brief returns like overnight stops)
- `--min-duration`: Minimum trip duration in hours (default: 2)
- `--min-sessions`: Minimum sessions required for a trip (default: 1)
- `--split-date`: Force trip split at specific dates (format: YYYY-MM-DD, can specify multiple times)

The trip detection algorithm:

- Filters sessions by distance from home locations
- Groups nearby sessions within the time gap
- Allows brief returns home (e.g., overnight on boating trips) without splitting trips
- Respects forced split dates for manual boundaries
- Calculates travel distance between sessions
- Generates smart trip names using location data (city, country) and dates

#### 7. Review and Edit Trips

After detecting trips, review them in the web UI:

Visit http://localhost:8080/trips to:

- **Photo Previews**: See 4 sample photos from each trip to quickly identify it
- **Interactive Map**: See all trips with color-coded markers
- **Route Visualization**: Click trips to show complete routes with session markers, connecting lines, and direction arrows
- **Edit Trip Names**: Click the pencil icon to rename trips
- **Exclude Trips**: Check "Exclude from album creation" for trips that shouldn't become albums (e.g., test shots, commutes)
- **Trip Details**: View duration, distance from home, travel distance, photographers, photo counts

#### 8. Create Albums in Immich

Generate albums in Immich from your trips:

```bash
./immich-albums create-albums
```

This will:
- Create an album for each trip (except excluded ones)
- Use the trip name as album name
- Add a description with dates, duration, photographers, and distances
- Add all photos from the trip to the album
- Store the Immich album ID for future updates

**Recreate existing albums:**

```bash
./immich-albums create-albums --recreate
```

This will delete and recreate all albums with updated data (useful after renaming trips or adjusting parameters).

## Web UI Features

The web interface (`./immich-albums serve --port 8080`) provides interactive tools for the entire workflow:

### Pages

| Page | URL | Purpose |
|------|-----|---------|
| **Dashboard** | `/` | Statistics overview and quick links |
| **Devices** | `/devices` | Label photographers with photo previews from Immich |
| **Sessions Map** | `/sessions` | View all detected sessions on interactive map |
| **Activity Heatmap** | `/heatmap` | Identify where you take most photos (for finding home locations) |
| **Home Locations** | `/homes` | Add/manage home locations with heatmap overlay |
| **Trips** | `/trips` | View, edit, and manage trips with photo previews and route visualization |
| **Coverage Analysis** | `/coverage` | Analyze geographic coverage of your photos |

### Key Features

- **Leaflet Maps**: Interactive maps with zoom, pan, and marker clustering
- **Immich Integration**: Photo thumbnails loaded directly from Immich via proxy
- **Real-time Updates**: Changes saved to database immediately via REST API
- **Responsive Design**: Works on desktop and tablet browsers
- **Photo Previews**: See sample photos from devices and trips without leaving the interface

### Configuration Management

#### Export Configuration

Save your device labels and home locations to seed files:

```bash
./immich-albums export-seeds
```

This creates:

- `seeds/device_labels.json`: All labeled devices with photographer assignments
- `seeds/home_locations.json`: All defined home locations

#### Import Configuration

Restore device labels and home locations from seed files:

```bash
./immich-albums import-seeds
```

Use this after re-importing assets to restore your configuration.

### Full Pipeline Regeneration

To run the complete pipeline from start to finish:

```bash
./regenerate.sh
```

This interactive script guides you through the entire process:

#### Step 1: Import Assets
Fetches all assets from Immich (2000-01-01 to 2026-01-01) and stores them in the local database.

#### Step 2: Configuration (Interactive)
You'll be asked to choose a configuration method:

**Option 1: Import from seed files** (Automated)
- Loads `seeds/device_labels.json` and `seeds/home_locations.json`
- Use this if you've run the pipeline before and exported seeds

**Option 2: Configure via Web UI** (Recommended for first run)
- Starts the web server automatically
- Opens these pages for configuration:
  - `/devices` - Label photographers with photo previews
  - `/heatmap` - View activity patterns to identify home locations
  - `/homes` - Click map to add home locations with radius
- Press Enter when done, and the script exports your configuration to seed files

#### Step 3: Infer Locations
Assigns locations to DSLR photos based on nearby phone photos from the same photographer (min confidence: 0.3).

#### Step 4: Detect Sessions
Groups photos into sessions using spatial-temporal clustering (max 6h gap, 5km distance, min 2 photos).

#### Step 5: Detect Trips
Identifies trips based on home distance (50km+), session gaps (48h max), allowing brief home returns (36h max).

#### Step 6: Create Albums (Optional)
Asks if you want to create/update albums in Immich. Creates albums for all non-excluded trips.

**When to use this:**

- First-time setup (choose interactive configuration)
- After changing device photographer labels
- After modifying home locations
- To process updated data from Immich
- To try different trip detection parameters
- Re-running after excluding certain trips from albums

## Architecture

```
immich-albums/
├── cmd/                    # CLI commands
│   ├── root.go            # Root command and global flags
│   ├── discover.go        # Device discovery
│   ├── infer.go           # Location inference
│   ├── sessions.go        # Session detection
│   ├── trips.go           # Trip detection
│   ├── serve.go           # Web UI server
│   ├── create_albums.go   # Album creation in Immich
│   ├── export_seeds.go    # Export configuration
│   └── import_seeds.go    # Import configuration
├── internal/
│   ├── models/            # Data structures
│   │   └── models.go      # Asset, Device, Session, Trip, HomeLocation
│   ├── immich/            # Immich API client
│   │   └── client.go      # API methods for albums, assets
│   ├── database/          # SQLite operations
│   │   ├── database.go    # Schema and migrations
│   │   ├── trips.go       # Trip-specific queries
│   │   └── homes.go       # Home location operations
│   ├── processor/         # Core algorithms
│   │   ├── devices.go     # Device discovery with filename counter clustering
│   │   ├── inference.go   # Location inference with confidence scoring
│   │   ├── clustering.go  # Spatial-temporal clustering for sessions
│   │   └── trips.go       # Trip detection with home distance analysis
│   └── web/               # Web UI handlers and templates
│       ├── server.go      # HTTP server, routes, and API endpoints
│       └── templates/     # HTML templates with Leaflet maps
│           ├── dashboard.html
│           ├── devices.html    # Device labeling with photo previews
│           ├── sessions.html
│           ├── heatmap.html
│           ├── homes.html      # Home location management
│           ├── trips.html      # Trip visualization and editing
│           └── coverage.html
├── seeds/                 # Configuration backup files
│   ├── device_labels.json # Device photographer assignments
│   └── home_locations.json# Home location definitions
├── regenerate.sh          # Full pipeline regeneration script
├── immich-albums.db       # SQLite database (generated)
└── main.go
```

## Configuration

The tool uses command-line flags for configuration. Required flags:

- `--immich-url`: Your Immich instance URL
- `--api-key`: Your Immich API key

Optional flags:

- `--db`: Path to SQLite database (default: `./immich-albums.db`)

## How It Works

### 1. Smart Device Discovery

Analyzes EXIF data to identify unique devices:

- Extracts make/model from photo metadata
- Uses Immich's DeviceID when available
- **Filename Counter Clustering**: For devices with the same make/model (e.g., multiple iPhones):
  - Extracts numeric counters from filenames (IMG_1234.jpg → 1234)
  - Identifies distinct counter ranges representing different physical devices
  - Filters out small clusters (messaging apps, screenshots)
  - Creates sub-device IDs (e.g., "iPhone 16 Pro Max #1", "iPhone 16 Pro Max #2")

### 2. Interactive Photographer Labeling

Web UI for device labeling:

- View sample photos from each device via Immich proxy
- Assign photographer names to cameras and phones
- System tracks which devices belong to whom
- Configuration saved to seed files for reuse

### 3. Location Inference with Confidence Scoring

For DSLR photos without GPS:

- Finds nearby phone photos from the **same photographer**
- Considers temporal proximity (can be **days** apart)
- Assigns confidence score based on time gap:
  - 1.0 = same hour
  - 0.5 = 3 days apart
  - 0.15 = 14 days apart
- Uses interpolation for photos between two known locations
- Minimum confidence threshold configurable (default: 0.3)

### 4. Session Detection (Spatial-Temporal Clustering)

Groups photos into sessions:

- **Temporal clustering**: Maximum time gap between photos (default: 6 hours)
- **Spatial clustering**: Maximum distance between photos (default: 5km)
- **Photographer association**: Tracks who took photos in each session
- Calculates session center point and radius
- Minimum photos per session (default: 2)

### 5. Trip Detection with Home Awareness

Analyzes sessions to identify trips:

- **Distance from home**: Sessions >50km from home locations qualify as trips
- **Session grouping**: Groups sessions within 48 hours of each other
- **Brief home returns**: Allows staying home for up to 36 hours without splitting trips (e.g., overnight stops on boating trips)
- **Forced split dates**: Manually split trips at specific dates
- **Smart naming**: Extracts location (city, country) from asset metadata and combines with date ranges
- Calculates total travel distance between session centers

### 6. Trip Review and Editing

Interactive web UI for trip management:

- **Photo previews**: 4 sample photos per trip for quick identification
- **Route visualization**: Complete routes with session markers, connecting lines, and direction arrows
- **Edit names**: Rename trips inline
- **Exclude from albums**: Mark trips that shouldn't become albums (test shots, commutes, etc.)

### 7. Album Creation in Immich

Generates Immich albums:

- Creates album for each non-excluded trip
- **Smart naming**: Uses trip name with location and dates
- **Rich descriptions**: Includes dates, duration, photographers, distances
- **Asset management**: Adds all trip photos to the album
- **Album ID tracking**: Stores Immich album IDs for updates
- **Recreate support**: Can delete and recreate albums with updated data

## Development Status

- [x] Project structure
- [x] Immich API client with album operations
- [x] Database schema with migrations
- [x] Smart device discovery with filename counter clustering
- [x] Interactive device labeling with photo previews
- [x] Location inference with confidence scoring
- [x] Session detection with spatial-temporal clustering
- [x] Web UI with interactive Leaflet maps
- [x] Dashboard with statistics
- [x] Sessions map visualization
- [x] Activity heatmap for identifying home locations
- [x] Home location management interface with heatmap overlay
- [x] Trip detection with home awareness and forced split dates
- [x] Trip visualization with routes, session markers, and direction arrows
- [x] Trip editing: rename trips and exclude from albums
- [x] Photo previews in trip list
- [x] Seed export/import for configuration management
- [x] Full pipeline regeneration script with interactive configuration
- [x] Album creation in Immich with recreate support

## Possible Future Enhancements

- [ ] Multi-photographer trip merging
- [ ] Weather data integration
- [ ] Photo quality scoring for album cover selection
- [ ] Custom album templates
- [ ] Statistics dashboard improvements
- [ ] Export trips to GPX/KML format

## License

MIT
