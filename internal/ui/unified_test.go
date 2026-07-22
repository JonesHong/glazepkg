package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/neur0map/glazepkg/internal/inventory"
	"github.com/neur0map/glazepkg/internal/model"
)

func TestUnifiedTabsPutLocalCommandsFirst(t *testing.T) {
	tabs := buildUnifiedTabs(
		[]model.Package{{Name: "git", Source: model.SourceBrew}},
		[]inventory.Record{{Name: "my-cli", Kind: inventory.KindCommand, ReadOnly: true}},
	)
	if len(tabs) != 2 || tabs[0].Label != "本地命令" || !tabs[0].Inventory {
		t.Fatalf("unified tabs = %#v, want local first", tabs)
	}
	if tabs[1].Label != "brew" || tabs[1].Inventory {
		t.Fatalf("package tab = %#v", tabs[1])
	}
}

func TestUnifiedSearchPrioritizesSelfMadeLocalCommands(t *testing.T) {
	m := Model{
		tabs: buildUnifiedTabs(
			[]model.Package{{Name: "cli", Source: model.SourceNpm}},
			[]inventory.Record{
				{Name: "brew-cli", Kind: inventory.KindCommand, Origin: "homebrew", ReadOnly: true},
				{Name: "workshop-cli", Kind: inventory.KindCommand, Origin: "workshop", ReadOnly: true},
			},
		),
		localRecords: []inventory.Record{
			{Name: "brew-cli", Kind: inventory.KindCommand, Origin: "homebrew", ReadOnly: true},
			{Name: "workshop-cli", Kind: inventory.KindCommand, Origin: "workshop", ReadOnly: true},
		},
		allPkgs: []model.Package{{Name: "cli", Source: model.SourceNpm}},
	}
	m.applyUnifiedSearch()
	if len(m.unifiedResults) != 3 || !m.unifiedResults[0].isLocal || m.unifiedResults[0].local.Name != "workshop-cli" {
		t.Fatalf("unified results = %#v, want workshop local first", m.unifiedResults)
	}
	if m.unifiedResults[2].isLocal || m.unifiedResults[2].pkg.Name != "cli" {
		t.Fatalf("package result ordering = %#v", m.unifiedResults)
	}
}

func TestLocalCommandEnterOpensReadOnlyDetail(t *testing.T) {
	m := Model{
		view:          viewList,
		tabs:          []tabItem{{Label: "本地命令", Source: localCommandsTabSource, Inventory: true}},
		localFiltered: []inventory.Record{{Name: "my-cli", Kind: inventory.KindCommand, Origin: "workshop", ReadOnly: true}},
	}
	listModel := m
	for _, binding := range listModel.contextKeys().ShortHelp() {
		if strings.Contains(binding.Help().Desc, "install") || strings.Contains(binding.Help().Desc, "upgrade") || strings.Contains(binding.Help().Desc, "remove") {
			t.Fatalf("local tab advertises mutation binding: %#v", binding.Help())
		}
	}
	updated, cmd := m.handleListKey("enter")
	if cmd != nil {
		t.Fatalf("local enter returned command: %v", cmd)
	}
	got := updated.(*Model)
	if got.view != viewLocalDetail || got.localDetail.Name != "my-cli" {
		t.Fatalf("local detail state = %#v", got)
	}
	updated, cmd = got.handleLocalDetailKey("u")
	if cmd != nil || !strings.Contains(updated.(*Model).statusMsg, "read-only") {
		t.Fatalf("local mutation key was not contained: cmd=%v model=%#v", cmd, updated)
	}
	if updated.(*Model).view != viewLocalDetail {
		t.Fatal("mutation key left local detail view")
	}
}

func TestSlashFromPackageTabUsesUnifiedSearch(t *testing.T) {
	m := Model{
		view:        viewList,
		tabs:        []tabItem{{Label: "brew", Source: string(model.SourceBrew)}},
		filterInput: textinput.New(),
	}
	updated, cmd := m.handleListKey("/")
	if cmd == nil || updated.(*Model).view != viewUnifiedSearch || !updated.(*Model).filtering {
		t.Fatalf("slash did not enter unified search: cmd=%v model=%#v", cmd, updated)
	}
}
