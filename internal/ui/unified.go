package ui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/neur0map/glazepkg/internal/inventory"
	"github.com/neur0map/glazepkg/internal/model"
)

type unifiedSearchResult struct {
	isLocal bool
	local   inventory.Record
	pkg     model.Package
}

func defaultInventoryConfig() inventory.Config {
	return inventory.Config{
		PathValue:   os.Getenv("PATH"),
		CatalogPath: os.Getenv("GPK_INVENTORY_CATALOG"),
	}
}

func loadLocalInventory(cfg inventory.Config, force bool) tea.Cmd {
	return func() tea.Msg {
		var (
			records []inventory.Record
			err     error
		)
		if force {
			records, err = inventory.Discover(context.Background(), cfg)
		} else {
			records, err = inventory.DiscoverCached(context.Background(), cfg)
		}
		if err != nil {
			return localInventoryDoneMsg{err: err}
		}
		sortLocalRecords(records)
		return localInventoryDoneMsg{records: records}
	}
}

func sortLocalRecords(records []inventory.Record) {
	sort.SliceStable(records, func(i, j int) bool {
		left, right := localPriority(records[i]), localPriority(records[j])
		if left != right {
			return left < right
		}
		return strings.ToLower(records[i].Name) < strings.ToLower(records[j].Name)
	})
}

// localPriority makes self-made/workshop commands the first local results,
// then user-local binaries, then other PATH entries. This ordering survives
// empty searches and is the tie-breaker for equal search relevance.
func localPriority(record inventory.Record) int {
	if strings.EqualFold(record.Origin, "workshop") || strings.EqualFold(record.Origin, "self-made") {
		return 0
	}
	for _, origin := range record.AlternateOrigins {
		if strings.EqualFold(origin, "workshop") || strings.EqualFold(origin, "self-made") {
			return 0
		}
	}
	if strings.EqualFold(record.Origin, "local-bin") {
		return 1
	}
	if strings.EqualFold(record.Origin, "homebrew") || strings.EqualFold(record.Origin, "path") {
		return 2
	}
	if strings.EqualFold(record.Origin, "system") {
		return 4
	}
	return 3
}

func rankLocalRecords(records []inventory.Record, query string) []inventory.Record {
	ordered := append([]inventory.Record(nil), records...)
	sortLocalRecords(ordered)
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return ordered
	}
	type scored struct {
		record inventory.Record
		score  int
		order  int
	}
	matched := make([]scored, 0, len(ordered))
	for i, record := range ordered {
		name := strings.ToLower(record.Name)
		text := strings.ToLower(record.SearchText())
		score := -1
		switch {
		case strings.HasPrefix(name, q):
			score = 0
		case strings.Contains(name, q):
			score = 1
		case strings.Contains(text, q):
			score = 2
		}
		if score >= 0 {
			matched = append(matched, scored{record: record, score: score, order: i})
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].score != matched[j].score {
			return matched[i].score < matched[j].score
		}
		return matched[i].order < matched[j].order
	})
	out := make([]inventory.Record, len(matched))
	for i, item := range matched {
		out[i] = item.record
	}
	return out
}

func (m Model) isLocalTab() bool {
	return m.activeTab >= 0 && m.activeTab < len(m.tabs) && m.tabs[m.activeTab].Inventory
}

func (m *Model) applyUnifiedSearch() {
	query := m.filterInput.Value()
	m.unifiedResults = m.unifiedResults[:0]
	for _, record := range rankLocalRecords(m.localRecords, query) {
		m.unifiedResults = append(m.unifiedResults, unifiedSearchResult{isLocal: true, local: record})
	}
	for _, pkg := range rankPackages(m.allPkgs, query) {
		m.unifiedResults = append(m.unifiedResults, unifiedSearchResult{pkg: pkg})
	}
	if m.unifiedCursor >= len(m.unifiedResults) {
		m.unifiedCursor = max(0, len(m.unifiedResults)-1)
	}
}

func (m *Model) enterUnifiedSearch() tea.Cmd {
	m.view = viewUnifiedSearch
	m.filtering = true
	m.filterInput.Focus()
	m.unifiedCursor = 0
	m.applyUnifiedSearch()
	return textinput.Blink
}

func (m *Model) handleUnifiedSearchKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, tea.Quit
	case "esc":
		m.view = viewList
		m.filtering = false
		m.filterInput.Blur()
		m.filterInput.SetValue("")
		m.unifiedResults = nil
		m.statusMsg = ""
	case "/":
		m.filtering = true
		m.filterInput.Focus()
		return m, textinput.Blink
	case "j", "down":
		if m.unifiedCursor < len(m.unifiedResults)-1 {
			m.unifiedCursor++
		}
	case "k", "up":
		if m.unifiedCursor > 0 {
			m.unifiedCursor--
		}
	case "g", "home":
		m.unifiedCursor = 0
	case "G", "end":
		if len(m.unifiedResults) > 0 {
			m.unifiedCursor = len(m.unifiedResults) - 1
		}
	case "enter":
		if len(m.unifiedResults) == 0 || m.unifiedCursor >= len(m.unifiedResults) {
			return m, nil
		}
		result := m.unifiedResults[m.unifiedCursor]
		m.detailReturn = viewUnifiedSearch
		if result.isLocal {
			m.localDetail = result.local
			m.statusMsg = ""
			m.view = viewLocalDetail
			return m, nil
		}
		m.pendingDetail = result.pkg
		if result.pkg.Source == model.SourcePacman || result.pkg.Source == model.SourceAUR {
			return m, loadDetail(result.pkg.Name)
		}
		m.detailPkg = result.pkg
		m.statusMsg = ""
		m.view = viewDetail
	}
	return m, nil
}

func (m *Model) handleLocalListKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.filterInput.Value() != "" {
			m.filterInput.SetValue("")
			m.applyFilter()
		}
	case "/", "ctrl+f":
		return m, m.enterUnifiedSearch()
	case "?", "h":
		return m, m.openModal(ModalHelp)
	case "tab":
		m.cycleTab(1)
	case "shift+tab":
		m.cycleTab(-1)
	case "j", "down":
		if m.cursor < len(m.localFiltered)-1 {
			m.cursor++
		}
		m.scroll = m.calculateLocalScroll()
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		m.scroll = m.calculateLocalScroll()
	case "g", "home":
		m.cursor, m.scroll = 0, 0
	case "G", "end":
		if len(m.localFiltered) > 0 {
			m.cursor = len(m.localFiltered) - 1
			m.scroll = m.calculateLocalScroll()
		}
	case "ctrl+d", "pgdown":
		m.cursor += max(m.tableHeight/2, 1)
		if m.cursor >= len(m.localFiltered) {
			m.cursor = max(0, len(m.localFiltered)-1)
		}
		m.scroll = m.calculateLocalScroll()
	case "ctrl+u", "pgup":
		m.cursor -= max(m.tableHeight/2, 1)
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.scroll = m.calculateLocalScroll()
	case "enter":
		if len(m.localFiltered) > 0 && m.cursor < len(m.localFiltered) {
			m.localDetail = m.localFiltered[m.cursor]
			m.detailReturn = viewList
			m.statusMsg = ""
			m.view = viewLocalDetail
		}
	case "r":
		if m.localLoading {
			m.statusMsg = "local command scan already in progress"
			return m, nil
		}
		m.localLoading = true
		m.localErr = nil
		m.statusMsg = "rescanning local commands..."
		return m, tea.Batch(m.spinner.Tick, loadLocalInventory(m.localConfig, true))
	case "t":
		return m, m.openThemePicker()
	case "i", "u", "x", "m", "s", "d", "e", "U", "Q":
		m.statusMsg = "read-only local commands: package action unavailable"
	}
	return m, nil
}

func (m *Model) calculateLocalScroll() int {
	visible := max(m.tableHeight, 1)
	maxScroll := max(0, len(m.localFiltered)-visible)
	if m.cursor >= len(m.localFiltered) {
		m.cursor = max(0, len(m.localFiltered)-1)
	}
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+visible {
		m.scroll = m.cursor - visible + 1
	}
	return min(max(m.scroll, 0), maxScroll)
}

func (m *Model) cycleTab(delta int) {
	if len(m.tabs) == 0 {
		return
	}
	m.activeTab = (m.activeTab + delta) % len(m.tabs)
	if m.activeTab < 0 {
		m.activeTab = len(m.tabs) - 1
	}
	m.cursor, m.scroll = 0, 0
	m.applyFilter()
}

func (m *Model) handleLocalDetailKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.view = m.detailReturn
		m.statusMsg = ""
	case "i", "u", "x", "m", "s", "d", "e", "U", "Q":
		m.statusMsg = "read-only local command: no package mutation or command execution"
	}
	return m, nil
}

func renderUnifiedSearchView(m Model) string {
	title := StyleTitle.Render("GlazePKG") + " " + StyleDim.Render("unified search")
	query := m.filterInput.View()
	if !m.filtering && m.filterInput.Value() == "" {
		query = StyleDim.Render("/ type to search local commands and packages")
	}
	lines := []string{lipgloss.PlaceHorizontal(m.width, lipgloss.Center, title), "", lipgloss.PlaceHorizontal(m.width, lipgloss.Center, query), ""}
	if len(m.unifiedResults) == 0 {
		lines = append(lines, StyleDim.Render("No matching local commands or packages."))
	} else {
		maxRows := max(1, m.height-9)
		start := 0
		if m.unifiedCursor >= maxRows {
			start = m.unifiedCursor - maxRows + 1
		}
		end := min(len(m.unifiedResults), start+maxRows)
		lines = append(lines, StyleTableHeader.Render("  NAME                         PLANE      SOURCE / ORIGIN"))
		lines = append(lines, StyleDim.Render(strings.Repeat("─", min(max(m.width-4, 40), 120))))
		for i := start; i < end; i++ {
			result := m.unifiedResults[i]
			name, plane, source := "", "package", ""
			if result.isLocal {
				name, plane, source = result.local.Name, "local", result.local.Origin
			} else {
				name, source = result.pkg.Name, string(result.pkg.Source)
			}
			line := fmt.Sprintf("  %-28s  %-9s  %s", truncateToWidth(name, 28), plane, truncateToWidth(source, max(m.width-45, 12)))
			if i == m.unifiedCursor {
				lines = append(lines, StyleSelected.Render("▎ "+line))
			} else {
				lines = append(lines, StyleNormal.Render(line))
			}
		}
	}
	footer := "j/k navigate   enter detail   / search   esc back   q quit · local commands first"
	lines = append(lines, "", StyleStatusBar.Render(footer))
	return strings.Join(lines, "\n")
}

func renderInventoryRecordDetail(record inventory.Record, status string) string {
	lines := []string{
		StyleTitle.Render("GlazePKG") + " " + StyleDim.Render("local command · read-only"),
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
	footer := "esc back   q quit   · read-only; no install/remove/help execution"
	if status != "" {
		footer = status + " · " + footer
	}
	lines = append(lines, "", StyleStatusBar.Render(footer))
	return strings.Join(lines, "\n")
}
