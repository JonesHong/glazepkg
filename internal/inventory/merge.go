package inventory

import "strings"

// Merge keeps PATH records as the primary effective-command view and enriches
// them with catalog metadata only when paths match. Catalog-only records remain
// visible and are marked unavailable by the catalog provider.
func Merge(pathRecords, catalogRecords []Record) []Record {
	result := make([]Record, 0, len(pathRecords)+len(catalogRecords))
	byPath := make(map[string]int)
	byID := make(map[string]int)
	for _, record := range pathRecords {
		copy := record.Clone()
		if copy.ID == "" {
			copy.ID = NewID(copy.ProviderID, copy.Kind, copy.Name, copy.Location)
		}
		byID[copy.ID] = len(result)
		if key := recordPathKey(copy); key != "" {
			byPath[key] = len(result)
		}
		result = append(result, copy)
	}
	for _, record := range catalogRecords {
		copy := record.Clone()
		if index, ok := byPath[recordPathKey(copy)]; ok && recordPathKey(copy) != "" {
			result[index] = enrich(result[index], copy)
			continue
		}
		if index, ok := byID[copy.ID]; ok {
			result[index] = enrich(result[index], copy)
			continue
		}
		byID[copy.ID] = len(result)
		result = append(result, copy)
	}
	sortRecords(result)
	return result
}

func recordPathKey(record Record) string {
	if record.ResolvedPath != "" {
		return normalizePath(record.ResolvedPath)
	}
	return normalizePath(record.Location)
}

func enrich(primary, metadata Record) Record {
	primary = primary.Clone()
	if metadata.Name != "" && metadata.Name != primary.Name {
		primary.Aliases = appendUnique(primary.Aliases, metadata.Name)
	}
	if primary.Description == "" {
		primary.Description = metadata.Description
	}
	if primary.Version == "" {
		primary.Version = metadata.Version
	}
	if primary.Homepage == "" {
		primary.Homepage = metadata.Homepage
	}
	for _, value := range metadata.Examples {
		primary.Examples = appendUnique(primary.Examples, value)
	}
	for _, value := range metadata.Tags {
		primary.Tags = appendUnique(primary.Tags, value)
	}
	if metadata.Origin != "" && !strings.EqualFold(primary.Origin, metadata.Origin) {
		primary.AlternateOrigins = appendUnique(primary.AlternateOrigins, metadata.Origin)
	}
	primary.Available = primary.Available || metadata.Available
	return primary
}
