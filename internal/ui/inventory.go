package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/neur0map/glazepkg/internal/config"
	"github.com/neur0map/glazepkg/internal/inventory"
)

type inventoryLoadedMsg struct {
	records []inventory.Record
	err     error
}

// InventoryModel is deliberately separate from the package mutation model.
// It reuses GPK styles but has no install/upgrade/remove command path.
type InventoryModel struct {
	version   string
	cfg       inventory.Config
	records   []inventory.Record
	filtered  []inventory.Record
	cursor    int
	width     int
	height    int
	loading   bool
	err       error
	filtering bool
	query     string
	detail    bool
	status    string
}

func NewInventoryModel(version string, cfg inventory.Config) *InventoryModel {
	appCfg := config.Load()
	ApplyTheme(config.ResolveTheme(appCfg.Appearance.Theme))
	return &InventoryModel{version: version, cfg: cfg, loading: true}
}

func (m *InventoryModel) Init() tea.Cmd {
	return loadInventoryCmd(m.cfg)
}

func loadInventoryCmd(cfg inventory.Config) tea.Cmd {
	return func() tea.Msg {
		records, err := inventory.DiscoverCached(context.Background(), cfg)
		return inventoryLoadedMsg{records: records, err: err}
	}
}

func (m *InventoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case inventoryLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.records = msg.records
		m.cursor = 0
		m.detail = false
		m.status = ""
		m.applyFilter()
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m *InventoryModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "q" || key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.detail {
		if key == "esc" || key == "backspace" {
			m.detail = false
			return m, nil
		}
		// The detail screen is intentionally read-only. In particular, i/u/x
		// are accepted as no-ops with a visible explanation rather than being
		// routed to package operations.
		if key == "i" || key == "u" || key == "x" || key == "r" || key == "m" {
			m.status = "read-only inventory: no mutation or command execution"
		}
		return m, nil
	}
	if m.filtering {
		switch key {
		case "esc":
			m.filtering = false
			m.query = ""
			m.applyFilter()
		case "enter":
			m.filtering = false
		case "backspace":
			if len(m.query) > 0 {
				m.query = string([]rune(m.query)[:len([]rune(m.query))-1])
				m.applyFilter()
			}
		default:
			if msg := key; len(msg) == 1 && msg[0] >= 32 && msg[0] != 127 {
				m.query += msg
				m.applyFilter()
			}
		}
		return m, nil
	}
	switch key {
	case "/":
		m.filtering = true
		m.status = ""
	case "enter":
		if len(m.filtered) > 0 {
			m.detail = true
		}
	case "esc":
		if m.query != "" {
			m.query = ""
			m.applyFilter()
		}
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
	case "r":
		m.loading = true
		m.status = "rescanning..."
		return m, loadInventoryCmd(m.cfg)
	case "i", "u", "x", "m", "s", "d", "e", "t":
		m.status = "read-only inventory: action unavailable"
	}
	return m, nil
}

func (m *InventoryModel) moveCursor(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
}

func (m *InventoryModel) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(m.query))
	m.filtered = m.filtered[:0]
	for _, record := range m.records {
		if q == "" || strings.Contains(strings.ToLower(record.SearchText()), q) {
			m.filtered = append(m.filtered, record)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *InventoryModel) View() string {
	if m.loading {
		return StyleTitle.Render("GPK Tools") + "\n\n" + StyleDim.Render("Scanning read-only command inventory...") + "\n"
	}
	if m.err != nil {
		return StyleTitle.Render("GPK Tools") + "\n\n" + StyleRemoved.Render("Inventory scan failed: "+m.err.Error()) + "\n\n" + StyleStatusBar.Render("r rescan   q quit")
	}
	if m.detail && len(m.filtered) > 0 {
		return m.detailView(m.filtered[m.cursor])
	}
	return m.listView()
}

func (m *InventoryModel) listView() string {
	title := StyleTitle.Render("GPK Tools") + " " + StyleDim.Render("read-only command inventory")
	meta := StyleDim.Render(fmt.Sprintf("%d tools", len(m.filtered)))
	if m.query != "" || m.filtering {
		meta += "   " + StyleFilterPrompt.Render("/ ") + StyleFilterText.Render(m.query)
	}
	if m.status != "" {
		meta += "   " + StyleDim.Render(m.status)
	}
	lines := []string{title, meta, ""}
	lines = append(lines, renderInventoryTable(m.filtered, m.cursor, m.width, m.height-7)...)
	lines = append(lines, "", StyleStatusBar.Render("j/k navigate   / search metadata   enter details   r rescan   q quit   · read-only"))
	return strings.Join(lines, "\n")
}

func (m *InventoryModel) detailView(record inventory.Record) string {
	lines := []string{
		StyleTitle.Render("GPK Tools") + " " + StyleDim.Render("read-only detail"),
		"",
		StyleDetailKey.Render("Name") + StyleDetailVal.Render(record.Name),
		StyleDetailKey.Render("Kind") + StyleDetailVal.Render(string(record.Kind)),
		StyleDetailKey.Render("Provider") + StyleDetailVal.Render(record.ProviderID),
		StyleDetailKey.Render("Origin") + StyleDetailVal.Render(record.Origin),
		StyleDetailKey.Render("Available") + StyleDetailVal.Render(fmt.Sprintf("%t", record.Available)),
	}
	if record.Version != "" {
		lines = append(lines, StyleDetailKey.Render("Version")+StyleDetailVal.Render(record.Version))
	}
	if record.Description != "" {
		lines = append(lines, StyleDetailKey.Render("Description")+StyleDetailVal.Render(record.Description))
	}
	if record.Location != "" {
		lines = append(lines, StyleDetailKey.Render("Location")+StyleDetailVal.Render(record.Location))
	}
	if record.ResolvedPath != "" && record.ResolvedPath != record.Location {
		lines = append(lines, StyleDetailKey.Render("Resolved")+StyleDetailVal.Render(record.ResolvedPath))
	}
	if len(record.AlternateLocations) > 0 {
		lines = append(lines, StyleDetailKey.Render("Shadowed")+StyleDetailVal.Render(strings.Join(record.AlternateLocations, ", ")))
	}
	if len(record.AlternateOrigins) > 0 {
		lines = append(lines, StyleDetailKey.Render("Also from")+StyleDetailVal.Render(strings.Join(record.AlternateOrigins, ", ")))
	}
	if len(record.Aliases) > 0 {
		lines = append(lines, StyleDetailKey.Render("Aliases")+StyleDetailVal.Render(strings.Join(record.Aliases, ", ")))
	}
	if record.Homepage != "" {
		lines = append(lines, StyleDetailKey.Render("Homepage")+StyleDetailVal.Render(record.Homepage))
	}
	if len(record.Tags) > 0 {
		lines = append(lines, StyleDetailKey.Render("Tags")+StyleDetailVal.Render(strings.Join(record.Tags, ", ")))
	}
	for i, example := range record.Examples {
		lines = append(lines, StyleDetailKey.Render(fmt.Sprintf("Example %d", i+1))+StyleDetailVal.Render(example))
	}
	lines = append(lines, "", StyleStatusBar.Render("esc back   q quit   · read-only; no install/remove/help execution"))
	return strings.Join(lines, "\n")
}

func renderInventoryTable(records []inventory.Record, cursor, width, height int) []string {
	if len(records) == 0 {
		return []string{StyleDim.Render("No tools found.")}
	}
	if width < 60 {
		width = 60
	}
	nameW := width * 28 / 100
	originW := width * 15 / 100
	if nameW < 16 {
		nameW = 16
	}
	if originW < 10 {
		originW = 10
	}
	locationW := width - nameW - originW - 16
	if locationW < 12 {
		locationW = 12
	}
	lines := []string{StyleTableHeader.Render(fmt.Sprintf("%-*s  %-8s  %-*s  %s", nameW, "NAME", "KIND", originW, "ORIGIN", "LOCATION"))}
	lines = append(lines, StyleDim.Render(strings.Repeat("─", width-2)))
	max := height
	if max < 1 {
		max = 1
	}
	if max > len(records) {
		max = len(records)
	}
	start := 0
	if cursor >= max {
		start = cursor - max + 1
	}
	for i := start; i < start+max && i < len(records); i++ {
		record := records[i]
		location := record.Location
		if location == "" {
			location = "(catalog-only)"
		}
		line := fmt.Sprintf("%-*s  %-8s  %-*s  %s", nameW, truncateToWidth(record.Name, nameW), record.Kind, originW, truncateToWidth(record.Origin, originW), truncateToWidth(location, locationW))
		if i == cursor {
			lines = append(lines, StyleSelected.Render("▎ "+line))
		} else {
			lines = append(lines, StyleNormal.Render("  "+line))
		}
	}
	return lines
}

var _ tea.Model = (*InventoryModel)(nil)
