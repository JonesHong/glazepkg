package inventory

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// PathProvider inventories the effective PATH without executing any command.
type PathProvider struct {
	PathValue     string
	IncludeSystem bool
	ExcludeDirs   []string
}

func NewPathProvider(pathValue string, includeSystem bool, excludeDirs []string) *PathProvider {
	if pathValue == "" {
		pathValue = os.Getenv("PATH")
	}
	return &PathProvider{
		PathValue:     pathValue,
		IncludeSystem: includeSystem,
		ExcludeDirs:   append([]string(nil), excludeDirs...),
	}
}

func (p *PathProvider) ID() string { return "path" }

func (p *PathProvider) Available() bool { return strings.TrimSpace(p.PathValue) != "" }

func (p *PathProvider) Scan(ctx context.Context) ([]Record, error) {
	if !p.Available() {
		return []Record{}, nil
	}
	dirs := uniquePathDirs(p.PathValue)
	seenNames := make(map[string]int)
	records := make([]Record, 0)
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if excludedDir(dir, p.ExcludeDirs) {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) {
				continue
			}
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			name := entry.Name()
			if name == "" || strings.HasPrefix(name, ".") {
				continue
			}
			path := filepath.Join(dir, name)
			if !isExecutableFile(path) {
				continue
			}
			if index, ok := seenNames[name]; ok {
				if index < 0 {
					continue
				}
				records[index].AlternateLocations = appendUnique(records[index].AlternateLocations, path)
				continue
			}
			seenNames[name] = len(records)
			origin := classifyOrigin(path)
			if origin == "system" && !p.IncludeSystem {
				// The system command still wins PATH precedence. Do not expose a
				// later shadow as the effective command by accident.
				seenNames[name] = -1
				continue
			}
			resolved := path
			if real, err := filepath.EvalSymlinks(path); err == nil {
				resolved = real
			}
			records = append(records, Record{
				ID:           NewID("path", KindCommand, name, ""),
				Kind:         KindCommand,
				Name:         name,
				ProviderID:   "path",
				Origin:       origin,
				Location:     path,
				ResolvedPath: resolved,
				Available:    true,
				ReadOnly:     true,
			})
		}
	}
	sortRecords(records)
	return records, nil
}

func uniquePathDirs(pathValue string) []string {
	seen := make(map[string]bool)
	var dirs []string
	for _, raw := range strings.Split(pathValue, string(os.PathListSeparator)) {
		if raw == "" {
			continue
		}
		dir := normalizePath(raw)
		key := dir
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			key = normalizePath(real)
		}
		if !seen[key] {
			seen[key] = true
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

func excludedDir(dir string, excludes []string) bool {
	for _, raw := range excludes {
		if raw == "" {
			continue
		}
		exclude := normalizePath(raw)
		rel, err := filepath.Rel(exclude, dir)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".exe", ".com", ".bat", ".cmd":
			return true
		}
	}
	return info.Mode()&0o111 != 0
}

func classifyOrigin(path string) string {
	clean := normalizePath(path)
	if isSystemPath(clean) {
		return "system"
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		for _, suffix := range []string{".cargo/bin", "go/bin", ".local/bin", "Library/pnpm"} {
			candidate := filepath.Join(home, suffix)
			rel, err := filepath.Rel(candidate, clean)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return "local-bin"
			}
		}
	}
	if runtime.GOOS == "darwin" && (pathWithin(clean, "/opt/homebrew") || pathWithin(clean, "/usr/local/Cellar")) {
		return "homebrew"
	}
	return "path"
}

func pathWithin(path, root string) bool {
	rel, err := filepath.Rel(normalizePath(root), normalizePath(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func isSystemPath(path string) bool {
	prefixes := []string{"/bin", "/sbin", "/usr/bin", "/usr/sbin", "/usr/libexec", "/System", "/Library/Apple/usr/bin"}
	if runtime.GOOS == "windows" {
		prefixes = []string{`C:\Windows`, `C:\Program Files\WindowsApps`}
	}
	for _, prefix := range prefixes {
		if pathWithin(path, prefix) {
			return true
		}
	}
	return false
}
