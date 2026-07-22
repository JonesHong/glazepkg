package ui

import (
	"fmt"
	"strings"

	"github.com/neur0map/glazepkg/internal/inventory"
)

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
