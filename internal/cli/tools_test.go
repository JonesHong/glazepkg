package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neur0map/glazepkg/internal/inventory"
)

func TestToolsListJSONIncludesCatalogMetadata(t *testing.T) {
	pathDir := t.TempDir()
	toolPath := filepath.Join(pathDir, "tool")
	if err := os.WriteFile(toolPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(t.TempDir(), "inventory.json")
	catalog := `{"schema_version":1,"executables":[{"name":"workshop:tool","path":"` + toolPath + `","source":"workshop","purpose":"custom launcher","examples":["tool --help"]}],"workshop_entrypoints":[],"packages":{"ignored":true}}`
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Dispatch([]string{"tools", "list", "--json", "--no-cache", "--path", pathDir, "--catalog", catalogPath}, nil, "test", &out, &errOut, nil)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr=%q", code, errOut.String())
	}
	var env struct {
		Data []inventory.Record `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data) != 1 || env.Data[0].Name != "tool" {
		t.Fatalf("records = %#v", env.Data)
	}
	if env.Data[0].Description != "custom launcher" || len(env.Data[0].Examples) != 1 || env.Data[0].Origin != "path" {
		t.Fatalf("metadata = %#v", env.Data[0])
	}
	if !containsString(env.Data[0].AlternateOrigins, "workshop") {
		t.Fatalf("alternate origins = %#v", env.Data[0].AlternateOrigins)
	}
	if !containsString(env.Data[0].Aliases, "workshop:tool") {
		t.Fatalf("catalog alias = %#v", env.Data[0].Aliases)
	}
	out.Reset()
	errOut.Reset()
	code = Dispatch([]string{"tools", "info", "workshop:tool", "--json", "--no-cache", "--path", pathDir, "--catalog", catalogPath}, nil, "test", &out, &errOut, nil)
	if code != ExitOK {
		t.Fatalf("catalog alias lookup exit %d, stderr=%q", code, errOut.String())
	}
}

func TestToolsSearchIsMetadataAwareAndReadOnly(t *testing.T) {
	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathDir, "plain-tool"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(t.TempDir(), "inventory.json")
	catalog := `{"schema_version":1,"executables":[],"workshop_entrypoints":[{"name":"workshop:agent","path":"/missing/agent","source":"workshop","category":"ai","purpose":"dispatch coding agents","examples":["agent run"]}]}`
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Dispatch([]string{"tools", "search", "dispatch", "--json", "--no-cache", "--path", pathDir, "--catalog", catalogPath}, nil, "test", &out, &errOut, nil)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "workshop:agent") || !strings.Contains(out.String(), "dispatch coding agents") {
		t.Fatalf("search output = %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	code = Dispatch([]string{"tools", "install", "workshop:agent", "--no-cache", "--path", pathDir}, nil, "test", &out, &errOut, nil)
	if code != ExitErr || !strings.Contains(errOut.String(), "tools is read-only") {
		t.Fatalf("tools install was not rejected safely: code=%d stderr=%q", code, errOut.String())
	}
}

func TestToolsInfoReportsUnavailableCatalogEntry(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), "inventory.json")
	catalog := `{"schema_version":1,"executables":[],"workshop_entrypoints":[{"name":"custom-cli","source":"workshop","purpose":"manual entry"}]}`
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Dispatch([]string{"tools", "info", "custom-cli", "--json", "--no-cache", "--path", t.TempDir(), "--catalog", catalogPath}, nil, "test", &out, &errOut, nil)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr=%q", code, errOut.String())
	}
	var env struct {
		Data inventory.Record `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Available || env.Data.ReadOnly != true || env.Data.Description != "manual entry" {
		t.Fatalf("entry = %#v", env.Data)
	}
}
