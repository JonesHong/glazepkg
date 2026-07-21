package inventory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPathProviderRespectsPrecedenceAndDoesNotExecute(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	firstPath := writeExecutable(t, first, "tool", "#!/bin/sh\nexit 99\n")
	secondPath := writeExecutable(t, second, "tool", "#!/bin/sh\nexit 98\n")
	writeExecutable(t, second, "other", "#!/bin/sh\nexit 97\n")
	p := NewPathProvider(first+string(os.PathListSeparator)+second+string(os.PathListSeparator)+first, true, nil)
	records, err := p.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2: %+v", len(records), records)
	}
	var tool Record
	for _, record := range records {
		if record.Name == "tool" {
			tool = record
		}
	}
	if tool.Location != firstPath {
		t.Fatalf("primary location = %q, want %q", tool.Location, firstPath)
	}
	if len(tool.AlternateLocations) != 1 || tool.AlternateLocations[0] != secondPath {
		t.Fatalf("shadow locations = %#v, want [%q]", tool.AlternateLocations, secondPath)
	}
}

func TestPathProviderResolvesSymlinksAndHonorsExcludes(t *testing.T) {
	dir, excluded := t.TempDir(), t.TempDir()
	target := writeExecutable(t, dir, "real-tool", "#!/bin/sh\n")
	link := filepath.Join(dir, "tool-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeExecutable(t, excluded, "hidden-tool", "#!/bin/sh\n")
	records, err := NewPathProvider(dir+string(os.PathListSeparator)+excluded, true, []string{excluded}).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %#v, want real tool and symlink", records)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Name == "tool-link" && record.ResolvedPath != resolvedTarget {
			t.Fatalf("resolved symlink = %q, want %q", record.ResolvedPath, resolvedTarget)
		}
		if record.Name == "hidden-tool" {
			t.Fatal("excluded directory leaked into inventory")
		}
	}
}

func TestPathProviderSystemFilterDoesNotRevealLaterShadow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses Unix system path")
	}
	user := t.TempDir()
	path := "/bin" + string(os.PathListSeparator) + user
	writeExecutable(t, user, "shadows-system", "#!/bin/sh\n")
	// There is no fixture write under /bin; use a known system executable only
	// when it exists, and verify the provider does not substitute the later one.
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh unavailable")
	}
	path = "/bin" + string(os.PathListSeparator) + user
	writeExecutable(t, user, "sh", "#!/bin/sh\n")
	records, err := NewPathProvider(path, false, nil).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Name == "sh" {
			t.Fatalf("hidden system command was replaced by later user shadow: %+v", record)
		}
	}
}

func TestCatalogParsesCurrentShapeAndIgnoresPackages(t *testing.T) {
	data := `{"schema_version":1,"executables":[{"name":"rg","path":"/tmp/rg","source":"homebrew","category":"search","purpose":"find","examples":["rg term"]}],"workshop_entrypoints":[{"name":"workshop:foo","path":"/tmp/foo.py","source":"workshop","kind":"self-made","purpose":"custom"}],"packages":{"homebrew":{"formulae":[{"name":"rg"}]}}}`
	records, err := ParseCatalog(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].Kind != KindCommand || !records[0].ReadOnly {
		t.Fatalf("record kind/read-only = %+v", records[0])
	}
	var found bool
	for _, record := range records {
		if record.Name == "rg" {
			found = true
			if record.Description != "find" || len(record.Examples) != 1 || !contains(record.Tags, "search") {
				t.Fatalf("metadata not mapped: %+v", record)
			}
		}
	}
	if !found {
		t.Fatal("missing rg record")
	}
}

func TestCatalogRejectsMalformedOrUnsupportedSchema(t *testing.T) {
	for _, input := range []string{`{"schema_version":2}`, `{"schema_version":1,"executables":[{}]}`} {
		if _, err := ParseCatalog(strings.NewReader(input)); err == nil {
			t.Errorf("ParseCatalog(%s) returned nil error", input)
		}
	}
}

func TestCatalogProviderHidesSystemRecordsByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	data := `{"schema_version":1,"executables":[{"name":"system-tool","path":"/usr/bin/system-tool","source":"system"}],"workshop_entrypoints":[]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := NewCatalogProvider(path).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("system catalog records = %#v, want none", records)
	}
}

func TestPathProviderRecognizesWindowsLauncherExtensions(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows launcher extensions are platform-specific")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.exe")
	if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := NewPathProvider(dir, true, nil).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Name != "tool.exe" {
		t.Fatalf("Windows launcher records = %#v", records)
	}
}

func TestMergeOnlyJoinsCatalogByPath(t *testing.T) {
	path := Record{ID: NewID("path", KindCommand, "tool", ""), Kind: KindCommand, Name: "tool", ProviderID: "path", Origin: "local-bin", Location: "/tmp/tool", ResolvedPath: "/tmp/tool", Available: true, ReadOnly: true}
	matching := Record{ID: NewID("catalog", KindCommand, "catalog-name", "/tmp/tool"), Kind: KindCommand, Name: "catalog-name", ProviderID: "catalog", Origin: "workshop", Location: "/tmp/tool", ResolvedPath: "/tmp/tool", Description: "metadata", Examples: []string{"tool --help"}, Available: false, ReadOnly: true}
	nonmatching := Record{ID: NewID("catalog", KindCommand, "tool", "/tmp/other"), Kind: KindCommand, Name: "tool", ProviderID: "catalog", Origin: "workshop", Location: "/tmp/other", ResolvedPath: "/tmp/other", Description: "wrong", Available: false, ReadOnly: true}
	merged := Merge([]Record{path}, []Record{matching, nonmatching})
	if len(merged) != 2 {
		t.Fatalf("merged len = %d, want 2: %+v", len(merged), merged)
	}
	var primary, other Record
	for _, record := range merged {
		if record.ID == path.ID {
			primary = record
		} else {
			other = record
		}
	}
	if primary.Description != "metadata" || !contains(primary.AlternateOrigins, "workshop") || !contains(primary.Aliases, "catalog-name") || len(primary.Examples) != 1 {
		t.Fatalf("matching metadata not merged: %+v", primary)
	}
	if other.Description != "wrong" || other.Available {
		t.Fatalf("nonmatching catalog record incorrectly merged: %+v", other)
	}
}

func TestCacheRoundTripAndExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache", "inventory.json")
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	records := []Record{{ID: "command:path:rg", Kind: KindCommand, Name: "rg", ProviderID: "path", ReadOnly: true}}
	if err := SaveCache(path, records, now); err != nil {
		t.Fatal(err)
	}
	got, fresh, err := LoadCache(path, now.Add(time.Hour))
	if err != nil || !fresh || len(got) != 1 || got[0].Name != "rg" {
		t.Fatalf("round trip = %#v, fresh=%v, err=%v", got, fresh, err)
	}
	if _, fresh, err := LoadCache(path, now.Add(CacheTTL+time.Hour)); err != nil || fresh {
		t.Fatalf("expired cache fresh=%v err=%v", fresh, err)
	}
}

func TestRecordJSONIsAdditiveAndStable(t *testing.T) {
	record := Record{ID: "command:path:rg", Kind: KindCommand, Name: "rg", ProviderID: "path", Available: true, ReadOnly: true}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"kind":"command"`) || !strings.Contains(string(data), `"read_only":true`) {
		t.Fatalf("unexpected JSON: %s", data)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
