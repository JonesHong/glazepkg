package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/neur0map/glazepkg/internal/inventory"
)

func TestInventoryModelUsesReadOnlyKeymap(t *testing.T) {
	m := NewInventoryModel("test", inventory.Config{})
	m.loading = false
	m.records = []inventory.Record{{ID: "command:path:tool", Kind: inventory.KindCommand, Name: "tool", ProviderID: "path", Origin: "local-bin", Location: "/tmp/tool", Available: true, ReadOnly: true}}
	m.applyFilter()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if cmd != nil {
		t.Fatalf("mutation key returned command: %v", cmd)
	}
	got := updated.(*InventoryModel)
	if got.detail || got.records[0].Name != "tool" || !strings.Contains(got.status, "read-only") {
		t.Fatalf("model changed unexpectedly: %+v", got)
	}
}

func TestInventoryDetailHasMetadataAndNoMutationHints(t *testing.T) {
	m := NewInventoryModel("test", inventory.Config{})
	m.loading = false
	m.records = []inventory.Record{{ID: "command:path:tool", Kind: inventory.KindCommand, Name: "tool", ProviderID: "path", Origin: "local-bin", Location: "/tmp/tool", Description: "custom", Tags: []string{"ai"}, Examples: []string{"tool run"}, Available: true, ReadOnly: true}}
	m.applyFilter()
	m.detail = true
	view := m.View()
	if !strings.Contains(view, "custom") || !strings.Contains(view, "tool run") || !strings.Contains(view, "read-only") {
		t.Fatalf("detail view missing metadata/safety text: %s", view)
	}
	if strings.Contains(view, "i install") || strings.Contains(view, "x remove") || strings.Contains(view, "u upgrade") {
		t.Fatalf("detail view advertises mutation keys: %s", view)
	}
}

func TestInventoryEnterOpensReadOnlyDetail(t *testing.T) {
	m := NewInventoryModel("test", inventory.Config{})
	m.loading = false
	m.records = []inventory.Record{{
		ID: "command:path:tool", Kind: inventory.KindCommand, Name: "tool",
		ProviderID: "path", Origin: "local-bin", Location: "/tmp/tool",
		Available: true, ReadOnly: true,
	}}
	m.applyFilter()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("enter returned unexpected command: %v", cmd)
	}
	if !updated.(*InventoryModel).detail {
		t.Fatal("enter did not open the read-only detail view")
	}
}

func TestInventoryViewHandlesNarrowTerminal(t *testing.T) {
	m := NewInventoryModel("test", inventory.Config{})
	m.loading = false
	m.width, m.height = 40, 8
	m.records = []inventory.Record{{
		ID: "command:path:tool", Kind: inventory.KindCommand, Name: "tool",
		ProviderID: "path", Origin: "local-bin", Location: "/tmp/a-very-long-tool-location",
		Available: true, ReadOnly: true,
	}}
	m.applyFilter()
	view := m.View()
	if !strings.Contains(view, "tool") || !strings.Contains(view, "read-only") {
		t.Fatalf("narrow view lost core content: %s", view)
	}
}
