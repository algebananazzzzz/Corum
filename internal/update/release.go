package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const GitHubAPI = "https://api.github.com"

// Release is the release metadata needed by the updater.
type Release struct {
	Tag    string
	Assets map[string]string
}

// Latest reads the latest Corum release from the GitHub Releases API.
func Latest(ctx context.Context, client *http.Client, apiBase string) (Release, error) {
	body, err := fetch(ctx, client, strings.TrimRight(apiBase, "/")+"/repos/algebananazzzzz/Corum/releases/latest")
	if err != nil {
		return Release{}, err
	}
	var raw struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Release{}, fmt.Errorf("decode release: %w", err)
	}
	if raw.TagName == "" {
		return Release{}, errors.New("release has no tag_name")
	}
	release := Release{Tag: raw.TagName, Assets: make(map[string]string, len(raw.Assets))}
	for _, asset := range raw.Assets {
		release.Assets[asset.Name] = asset.URL
	}
	return release, nil
}

func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "corum-updater")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, response.Status)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	return data, nil
}

// Newer reports whether latest is a newer strict X.Y.Z semantic version.
func Newer(latest, current string) bool {
	latestParts, ok := parseVersion(latest)
	if !ok {
		return false
	}
	currentParts, ok := parseVersion(current)
	if !ok {
		return false
	}
	for index := range latestParts {
		if latestParts[index] != currentParts[index] {
			return latestParts[index] > currentParts[index]
		}
	}
	return false
}

func parseVersion(value string) ([3]int, bool) {
	parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var parsed [3]int
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return [3]int{}, false
		}
		parsed[index] = number
	}
	return parsed, true
}

func withV(value string) string {
	if strings.HasPrefix(value, "v") {
		return value
	}
	return "v" + value
}
