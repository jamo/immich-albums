package immich

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jamo/immich-albums/internal/models"
)

type Client struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchAssets retrieves all assets within a date range
func (c *Client) FetchAssets(start, end time.Time) ([]models.Asset, error) {
	endpoint := fmt.Sprintf("%s/api/search/metadata", c.baseURL)

	var allAssets []models.Asset
	page := 1
	size := 1000 // Max page size

	for {
		// Build request body
		requestBody := map[string]interface{}{
			"takenAfter":  start.Format(time.RFC3339),
			"takenBefore": end.Format(time.RFC3339),
			"page":        page,
			"size":        size,
			"withExif":    true,
		}

		jsonBody, err := json.Marshal(requestBody)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonBody))
		if err != nil {
			return nil, err
		}

		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		var response struct {
			Assets struct {
				Count      int             `json:"count"`
				Items      []assetResponse `json:"items"`
				Total      int             `json:"total"`
				NextPage   *string         `json:"nextPage"`
			} `json:"assets"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()

		// Parse assets from this page
		for _, item := range response.Assets.Items {
			allAssets = append(allAssets, parseAsset(item))
		}

		fmt.Printf("Fetched page %d: %d assets (total so far: %d)\n", page, len(response.Assets.Items), len(allAssets))

		// Check if there are more pages
		if len(response.Assets.Items) < size {
			break // No more results
		}

		page++
	}

	return allAssets, nil
}

type assetResponse struct {
	ID               string    `json:"id"`
	DeviceAssetID    string    `json:"deviceAssetId"`
	OwnerID          string    `json:"ownerId"`
	DeviceID         string    `json:"deviceId"`
	Type             string    `json:"type"`
	OriginalPath     string    `json:"originalPath"`
	OriginalFileName string    `json:"originalFileName"`
	FileCreatedAt    time.Time `json:"fileCreatedAt"`
	FileModifiedAt   time.Time `json:"fileModifiedAt"`
	LocalDateTime    time.Time `json:"localDateTime"`
	Duration         string    `json:"duration"`
	ExifInfo         *struct {
		Make            string   `json:"make"`
		Model           string   `json:"model"`
		ExifImageWidth  int      `json:"exifImageWidth"`
		ExifImageHeight int      `json:"exifImageHeight"`
		Orientation     string   `json:"orientation"`
		LensModel       string   `json:"lensModel"`
		FNumber         float64  `json:"fNumber"`
		FocalLength     float64  `json:"focalLength"`
		ISO             int      `json:"iso"`
		ExposureTime    string   `json:"exposureTime"`
		Latitude        *float64 `json:"latitude"`
		Longitude       *float64 `json:"longitude"`
		City            string   `json:"city"`
		State           string   `json:"state"`
		Country         string   `json:"country"`
	} `json:"exifInfo"`
}

func parseAsset(resp assetResponse) models.Asset {
	asset := models.Asset{
		ID:               resp.ID,
		DeviceAssetID:    resp.DeviceAssetID,
		OwnerID:          resp.OwnerID,
		DeviceID:         resp.DeviceID,
		Type:             resp.Type,
		OriginalPath:     resp.OriginalPath,
		OriginalFileName: resp.OriginalFileName,
		FileCreatedAt:    resp.FileCreatedAt,
		FileModifiedAt:   resp.FileModifiedAt,
		LocalDateTime:    resp.LocalDateTime,
		Duration:         resp.Duration,
	}

	if resp.ExifInfo != nil {
		asset.Make = resp.ExifInfo.Make
		asset.Model = resp.ExifInfo.Model
		asset.ExifImageWidth = resp.ExifInfo.ExifImageWidth
		asset.ExifImageHeight = resp.ExifInfo.ExifImageHeight
		asset.Orientation = resp.ExifInfo.Orientation
		asset.LensModel = resp.ExifInfo.LensModel
		asset.FNumber = resp.ExifInfo.FNumber
		asset.FocalLength = resp.ExifInfo.FocalLength
		asset.ISO = resp.ExifInfo.ISO
		asset.ExposureTime = resp.ExifInfo.ExposureTime
		asset.Latitude = resp.ExifInfo.Latitude
		asset.Longitude = resp.ExifInfo.Longitude
		asset.City = resp.ExifInfo.City
		asset.State = resp.ExifInfo.State
		asset.Country = resp.ExifInfo.Country
	}

	return asset
}

// CreateAlbum creates a new album in Immich
func (c *Client) CreateAlbum(name string, description string) (string, error) {
	endpoint := fmt.Sprintf("%s/api/albums", c.baseURL)

	requestBody := map[string]interface{}{
		"albumName":   name,
		"description": description,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create album with status %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		ID string `json:"id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return response.ID, nil
}

// AddAssetsToAlbum adds assets to an album
func (c *Client) AddAssetsToAlbum(albumID string, assetIDs []string) error {
	endpoint := fmt.Sprintf("%s/api/albums/%s/assets", c.baseURL, albumID)

	requestBody := map[string]interface{}{
		"ids": assetIDs,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to add assets to album with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteAlbum deletes an album from Immich
func (c *Client) DeleteAlbum(albumID string) error {
	endpoint := fmt.Sprintf("%s/api/albums/%s", c.baseURL, albumID)

	req, err := http.NewRequest("DELETE", endpoint, nil)
	if err != nil {
		return err
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete album with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
