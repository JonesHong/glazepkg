package inventory

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const CacheVersion = 1
const CacheTTL = 10 * 24 * time.Hour

type cacheFile struct {
	Version     int       `json:"version"`
	Timestamp   time.Time `json:"timestamp"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Records     []Record  `json:"records"`
}

// DefaultCachePath is separate from the package manager scan cache.
func DefaultCachePath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "glazepkg", "cache", "inventory.json")
}

func LoadCache(path string, now time.Time) ([]Record, bool, error) {
	return LoadCacheWithFingerprint(path, now, "")
}

func LoadCacheWithFingerprint(path string, now time.Time, fingerprint string) ([]Record, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var file cacheFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, false, fmt.Errorf("decode inventory cache: %w", err)
	}
	if file.Version != CacheVersion || file.Timestamp.IsZero() || now.Sub(file.Timestamp) > CacheTTL {
		return nil, false, nil
	}
	if fingerprint != "" && file.Fingerprint != fingerprint {
		return nil, false, nil
	}
	return file.Records, true, nil
}

func SaveCache(path string, records []Record, now time.Time) error {
	return SaveCacheWithFingerprint(path, records, now, "")
}

func SaveCacheWithFingerprint(path string, records []Record, now time.Time, fingerprint string) error {
	file := cacheFile{Version: CacheVersion, Timestamp: now, Fingerprint: fingerprint, Records: records}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".inventory-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// ConfigFingerprint prevents a cache generated from a different PATH or
// catalog from being presented as current inventory.
func ConfigFingerprint(cfg Config) string {
	parts := []string{cfg.PathValue, strconv.FormatBool(cfg.IncludeSystem), strings.Join(cfg.ExcludeDirs, "\x00"), cfg.CatalogPath}
	if cfg.CatalogPath != "" {
		if info, err := os.Stat(cfg.CatalogPath); err == nil {
			parts = append(parts, info.ModTime().UTC().String(), strconv.FormatInt(info.Size(), 10))
		}
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x01"))))
}
