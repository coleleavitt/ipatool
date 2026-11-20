package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const versionMetadataCacheFileName = "version-metadata.json"

// versionMetadataCache maps app ID -> external version ID -> metadata. An external
// version ID always names the same release, so entries never need to expire.
type versionMetadataCache map[string]map[string]versionMetadata

func versionMetadataCachePath() string {
	if dependencies.StateDirectory == "" {
		return ""
	}

	return filepath.Join(dependencies.StateDirectory, versionMetadataCacheFileName)
}

func loadVersionMetadataCache(path string) (versionMetadataCache, error) {
	cache := versionMetadataCache{}
	if path == "" {
		return cache, nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cache, nil
	}

	if err != nil {
		return cache, fmt.Errorf("failed to read version metadata cache: %w", err)
	}

	if err := json.Unmarshal(data, &cache); err != nil {
		return versionMetadataCache{}, fmt.Errorf("failed to parse version metadata cache: %w", err)
	}

	return cache, nil
}

func (c versionMetadataCache) get(appID int64, versionID string) (versionMetadata, bool) {
	v, ok := c[strconv.FormatInt(appID, 10)][versionID]

	return v, ok
}

func (c versionMetadataCache) put(appID int64, v versionMetadata) {
	key := strconv.FormatInt(appID, 10)
	if c[key] == nil {
		c[key] = map[string]versionMetadata{}
	}

	c[key][v.ExternalVersionID] = v
}

func (c versionMetadataCache) save(path string) error {
	if path == "" {
		return nil
	}

	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to encode version metadata cache: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("failed to write version metadata cache: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to write version metadata cache: %w", err)
	}

	return nil
}
