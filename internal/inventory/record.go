package inventory

import (
	"path/filepath"
	"sort"
	"strings"
)

// Kind identifies the semantic kind of an inventory record. Catalog is a
// provider, not a visible kind: catalog metadata enriches command records.
type Kind string

const KindCommand Kind = "command"

// Record is the read-only discovery-plane representation. It deliberately does
// not embed model.Package: package identity, snapshots, and mutation
// capabilities must not leak into PATH/catalog discovery.
type Record struct {
	ID                 string   `json:"id"`
	Kind               Kind     `json:"kind"`
	Name               string   `json:"name"`
	Aliases            []string `json:"aliases,omitempty"`
	Version            string   `json:"version,omitempty"`
	Description        string   `json:"description,omitempty"`
	ProviderID         string   `json:"provider"`
	Origin             string   `json:"origin,omitempty"`
	AlternateOrigins   []string `json:"alternate_origins,omitempty"`
	Location           string   `json:"location,omitempty"`
	ResolvedPath       string   `json:"resolved_path,omitempty"`
	AlternateLocations []string `json:"alternate_locations,omitempty"`
	Homepage           string   `json:"homepage,omitempty"`
	Examples           []string `json:"examples,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	Available          bool     `json:"available"`
	ReadOnly           bool     `json:"read_only"`
}

// NewID creates a provider-scoped identity. Location is included for catalog
// records so two catalog-only commands with the same display name cannot
// overwrite each other. PATH records intentionally use name identity because
// PATH precedence chooses one primary command per name.
func NewID(provider string, kind Kind, name, location string) string {
	base := string(kind) + ":" + provider + ":" + name
	if provider == "catalog" && location != "" {
		return base + ":" + normalizePath(location)
	}
	return base
}

func (r Record) SearchText() string {
	parts := []string{
		r.Name, strings.Join(r.Aliases, " "), r.Version, r.Description, r.ProviderID, r.Origin,
		r.Location, r.ResolvedPath, r.Homepage,
		strings.Join(r.AlternateOrigins, " "),
		strings.Join(r.Examples, " "),
		strings.Join(r.Tags, " "),
	}
	return strings.Join(parts, " ")
}

func (r Record) Clone() Record {
	r.AlternateOrigins = append([]string(nil), r.AlternateOrigins...)
	r.Aliases = append([]string(nil), r.Aliases...)
	r.AlternateLocations = append([]string(nil), r.AlternateLocations...)
	r.Examples = append([]string(nil), r.Examples...)
	r.Tags = append([]string(nil), r.Tags...)
	return r
}

func normalizePath(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if absolute, err := filepath.Abs(clean); err == nil {
		clean = absolute
	}
	return clean
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sortRecords(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		left := strings.ToLower(records[i].Name)
		right := strings.ToLower(records[j].Name)
		if left != right {
			return left < right
		}
		return records[i].ID < records[j].ID
	})
}
