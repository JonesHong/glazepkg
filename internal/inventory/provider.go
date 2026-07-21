package inventory

import (
	"context"
	"time"
)

// Provider is a read-only discovery source. Providers must not implement any
// package install, upgrade, remove, hold, or autoremove capability.
type Provider interface {
	ID() string
	Available() bool
	Scan(context.Context) ([]Record, error)
}

type Config struct {
	PathValue     string
	IncludeSystem bool
	ExcludeDirs   []string
	CatalogPath   string
}

// Discover scans the configured providers and merges their records. The
// package-manager plane is intentionally not part of this function.
func Discover(ctx context.Context, cfg Config) ([]Record, error) {
	pathRecords, err := NewPathProvider(cfg.PathValue, cfg.IncludeSystem, cfg.ExcludeDirs).Scan(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.CatalogPath == "" {
		return pathRecords, nil
	}
	catalogProvider := NewCatalogProvider(cfg.CatalogPath)
	catalogProvider.IncludeSystem = cfg.IncludeSystem
	catalogRecords, err := catalogProvider.Scan(ctx)
	if err != nil {
		return nil, err
	}
	return Merge(pathRecords, catalogRecords), nil
}

// DiscoverCached provides the shared read-only cache path used by the TUI.
// Cache errors are recoverable: a fresh scan is safer than failing discovery.
func DiscoverCached(ctx context.Context, cfg Config) ([]Record, error) {
	fingerprint := ConfigFingerprint(cfg)
	if records, fresh, err := LoadCacheWithFingerprint(DefaultCachePath(), time.Now(), fingerprint); err == nil && fresh {
		return records, nil
	}
	records, err := Discover(ctx, cfg)
	if err != nil {
		return nil, err
	}
	_ = SaveCacheWithFingerprint(DefaultCachePath(), records, time.Now(), fingerprint)
	return records, nil
}
