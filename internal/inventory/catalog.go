package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type CatalogProvider struct {
	Path          string
	IncludeSystem bool
}

func NewCatalogProvider(path string) *CatalogProvider { return &CatalogProvider{Path: path} }

func (p *CatalogProvider) ID() string { return "catalog" }

func (p *CatalogProvider) Available() bool {
	if p.Path == "" {
		return false
	}
	info, err := os.Stat(p.Path)
	return err == nil && info.Mode().IsRegular()
}

func (p *CatalogProvider) Scan(ctx context.Context) ([]Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.Open(p.Path)
	if err != nil {
		return nil, err
	}
	defer data.Close()
	records, err := parseCatalog(data, filepath.Dir(p.Path))
	if err != nil || p.IncludeSystem {
		return records, err
	}
	filtered := records[:0]
	for _, record := range records {
		if record.Origin == "system" || isSystemPath(record.Location) || isSystemPath(record.ResolvedPath) {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered, nil
}

type catalogFile struct {
	SchemaVersion       int            `json:"schema_version"`
	Executables         []catalogEntry `json:"executables"`
	WorkshopEntryPoints []catalogEntry `json:"workshop_entrypoints"`
}

type catalogEntry struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	RealPath    string   `json:"real_path"`
	Source      string   `json:"source"`
	Kind        string   `json:"kind"`
	Category    string   `json:"category"`
	Purpose     string   `json:"purpose"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Homepage    string   `json:"homepage"`
	Examples    []string `json:"examples"`
	Tags        []string `json:"tags"`
}

// ParseCatalog parses the generic/current catalog format without executing or
// resolving any declared command. Relative paths are interpreted from the
// current directory; CatalogProvider supplies the manifest directory.
func ParseCatalog(r io.Reader) ([]Record, error) { return parseCatalog(r, "") }

func parseCatalog(r io.Reader, baseDir string) ([]Record, error) {
	var file catalogFile
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	if file.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported catalog schema_version %d", file.SchemaVersion)
	}
	entries := append(file.Executables, file.WorkshopEntryPoints...)
	records := make([]Record, 0, len(entries))
	for i, entry := range entries {
		if strings.TrimSpace(entry.Name) == "" {
			return nil, fmt.Errorf("catalog entry %d: name is required", i)
		}
		location := entry.Path
		if location == "" {
			location = entry.RealPath
		}
		if location != "" && baseDir != "" && !filepath.IsAbs(location) {
			location = filepath.Join(baseDir, location)
		}
		resolved := location
		if location != "" {
			if real, err := filepath.EvalSymlinks(location); err == nil {
				resolved = real
			}
		}
		available := location != ""
		if location != "" {
			available = isExecutableFile(location)
		}
		description := entry.Description
		if description == "" {
			description = entry.Purpose
		}
		tags := append([]string(nil), entry.Tags...)
		if entry.Category != "" {
			tags = appendUnique(tags, entry.Category)
		}
		origin := entry.Source
		if origin == "" {
			origin = "catalog"
		}
		records = append(records, Record{
			ID:           NewID("catalog", KindCommand, entry.Name, location),
			Kind:         KindCommand,
			Name:         entry.Name,
			Version:      entry.Version,
			Description:  description,
			ProviderID:   "catalog",
			Origin:       origin,
			Location:     location,
			ResolvedPath: resolved,
			Homepage:     entry.Homepage,
			Examples:     append([]string(nil), entry.Examples...),
			Tags:         tags,
			Available:    available,
			ReadOnly:     true,
		})
	}
	sortRecords(records)
	return records, nil
}
