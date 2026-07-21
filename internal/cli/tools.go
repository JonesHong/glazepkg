package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/neur0map/glazepkg/internal/inventory"
	"github.com/neur0map/glazepkg/internal/manager"
)

func init() {
	subcommands["tools"] = runTools
}

type toolsOptions struct {
	JSON          bool
	NoCache       bool
	IncludeSystem bool
	CatalogPath   string
	PathValue     string
	Quiet         bool
}

// runTools is an explicit read-only namespace. It intentionally does not
// reuse package list/info/search handlers, whose fallback behavior can install.
func runTools(args []string, _ []manager.Manager, version string, stdout, stderr io.Writer, _ io.Reader) int {
	action := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "list", "search", "info":
			action, args = args[0], args[1:]
		case "install", "remove", "upgrade", "downgrade", "clean", "autoremove", "hold", "unhold":
			fmt.Fprintf(stderr, "error: tools is read-only; action %q is not available\n", args[0])
			return ExitErr
		default:
			// A bare term inside the explicit namespace is always a read-only
			// search, never the package install fallback.
			action = "search"
		}
	}

	opts, terms, ok, help := parseToolsFlags(action, args, stderr)
	if help {
		return ExitOK
	}
	if !ok {
		return ExitErr
	}
	records, err := loadInventory(context.Background(), opts, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "error: inventory scan failed: %v\n", err)
		return ExitErr
	}
	switch action {
	case "list":
		if len(terms) > 0 {
			records = filterInventory(records, strings.Join(terms, " "))
		}
		return writeToolsList(stdout, stderr, version, records, opts.JSON)
	case "search":
		if len(terms) == 0 {
			fmt.Fprintln(stderr, "error: tools search requires a query")
			return ExitErr
		}
		records = rankInventory(records, strings.Join(terms, " "))
		if len(records) == 0 {
			fmt.Fprintf(stderr, "no tools found for %q\n", strings.Join(terms, " "))
			return ExitNegative
		}
		return writeToolsList(stdout, stderr, version, records, opts.JSON)
	case "info":
		if len(terms) != 1 {
			fmt.Fprintln(stderr, "error: tools info requires exactly one name or id")
			return ExitErr
		}
		for _, record := range records {
			if record.Name == terms[0] || record.ID == terms[0] || containsString(record.Aliases, terms[0]) {
				if opts.JSON {
					if err := writeEnvelope(stdout, version, record); err != nil {
						fmt.Fprintf(stderr, "error: encoding JSON: %v\n", err)
						return ExitErr
					}
					return ExitOK
				}
				writeToolHuman(stdout, record)
				return ExitOK
			}
		}
		fmt.Fprintf(stderr, "tool %q not found\n", terms[0])
		return ExitNegative
	default:
		fmt.Fprintf(stderr, "error: unsupported tools action %q\n", action)
		return ExitErr
	}
}

func parseToolsFlags(action string, args []string, stderr io.Writer) (toolsOptions, []string, bool, bool) {
	fs := flag.NewFlagSet("tools "+action, flag.ContinueOnError)
	fs.SetOutput(stderr)
	opts := toolsOptions{}
	fs.BoolVar(&opts.JSON, "json", false, "emit JSON envelope")
	fs.BoolVar(&opts.NoCache, "no-cache", false, "bypass the inventory cache")
	fs.BoolVar(&opts.IncludeSystem, "include-system", false, "include system PATH commands")
	fs.StringVar(&opts.CatalogPath, "catalog", os.Getenv("GPK_INVENTORY_CATALOG"), "catalog JSON manifest")
	fs.StringVar(&opts.PathValue, "path", os.Getenv("PATH"), "PATH value to scan")
	fs.BoolVar(&opts.Quiet, "quiet", false, "suppress progress messages")
	args = reorderFlagsFirst(args, []string{"catalog", "path"})
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, nil, true, true
		}
		return opts, nil, false, false
	}
	return opts, fs.Args(), true, false
}

func loadInventory(ctx context.Context, opts toolsOptions, stderr io.Writer) ([]inventory.Record, error) {
	cfg := inventory.Config{
		PathValue:     opts.PathValue,
		IncludeSystem: opts.IncludeSystem,
		CatalogPath:   opts.CatalogPath,
	}
	cachePath := inventory.DefaultCachePath()
	fingerprint := inventory.ConfigFingerprint(cfg)
	if !opts.NoCache {
		if records, fresh, err := inventory.LoadCacheWithFingerprint(cachePath, time.Now(), fingerprint); err != nil {
			if !opts.Quiet {
				// A corrupt cache is recoverable; a fresh scan is safer than
				// failing a read-only inventory command.
				fmt.Fprintf(stderr, "warning: ignoring inventory cache: %v\n", err)
			}
		} else if fresh {
			return records, nil
		}
	}
	records, err := inventory.Discover(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := inventory.SaveCacheWithFingerprint(cachePath, records, time.Now(), fingerprint); err != nil && !opts.Quiet {
		fmt.Fprintf(stderr, "warning: cannot save inventory cache: %v\n", err)
	}
	return records, nil
}

func writeToolsList(stdout, stderr io.Writer, version string, records []inventory.Record, asJSON bool) int {
	if asJSON {
		if err := writeEnvelope(stdout, version, records); err != nil {
			fmt.Fprintf(stderr, "error: encoding JSON: %v\n", err)
			return ExitErr
		}
		return ExitOK
	}
	if len(records) == 0 {
		fmt.Fprintln(stdout, "(no tools)")
		return ExitOK
	}
	nameW, originW := len("NAME"), len("ORIGIN")
	for _, record := range records {
		if len(record.Name) > nameW {
			nameW = len(record.Name)
		}
		if len(record.Origin) > originW {
			originW = len(record.Origin)
		}
	}
	fmt.Fprintf(stdout, "%-*s  %-8s  %-*s  %s\n", nameW, "NAME", "KIND", originW, "ORIGIN", "LOCATION")
	fmt.Fprintln(stdout, strings.Repeat("-", nameW+2+8+2+originW+2+8))
	for _, record := range records {
		location := record.Location
		if location == "" {
			location = "(catalog-only)"
		}
		fmt.Fprintf(stdout, "%-*s  %-8s  %-*s  %s\n", nameW, record.Name, record.Kind, originW, record.Origin, location)
	}
	return ExitOK
}

func writeToolHuman(w io.Writer, record inventory.Record) {
	fmt.Fprintf(w, "Name:        %s\n", record.Name)
	fmt.Fprintf(w, "Kind:        %s\n", record.Kind)
	fmt.Fprintf(w, "Provider:    %s\n", record.ProviderID)
	fmt.Fprintf(w, "Origin:      %s\n", record.Origin)
	fmt.Fprintf(w, "Available:   %t\n", record.Available)
	if record.Version != "" {
		fmt.Fprintf(w, "Version:     %s\n", record.Version)
	}
	if record.Description != "" {
		fmt.Fprintf(w, "Description: %s\n", record.Description)
	}
	if record.Location != "" {
		fmt.Fprintf(w, "Location:    %s\n", record.Location)
	}
	if record.ResolvedPath != "" && record.ResolvedPath != record.Location {
		fmt.Fprintf(w, "Resolved:    %s\n", record.ResolvedPath)
	}
	if len(record.AlternateLocations) > 0 {
		fmt.Fprintf(w, "Shadowed:    %s\n", strings.Join(record.AlternateLocations, ", "))
	}
	if len(record.Aliases) > 0 {
		fmt.Fprintf(w, "Aliases:     %s\n", strings.Join(record.Aliases, ", "))
	}
	if record.Homepage != "" {
		fmt.Fprintf(w, "Homepage:    %s\n", record.Homepage)
	}
	if len(record.Tags) > 0 {
		fmt.Fprintf(w, "Tags:        %s\n", strings.Join(record.Tags, ", "))
	}
	for _, example := range record.Examples {
		fmt.Fprintf(w, "Example:     %s\n", example)
	}
}

func filterInventory(records []inventory.Record, query string) []inventory.Record {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return records
	}
	out := make([]inventory.Record, 0, len(records))
	for _, record := range records {
		if strings.Contains(strings.ToLower(record.SearchText()), q) {
			out = append(out, record)
		}
	}
	return out
}

func rankInventory(records []inventory.Record, query string) []inventory.Record {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	type scored struct {
		record inventory.Record
		score  int
	}
	var scoredRecords []scored
	for _, record := range records {
		name := strings.ToLower(record.Name)
		text := strings.ToLower(record.SearchText())
		score := 0
		switch {
		case strings.HasPrefix(name, q):
			score = 0
		case strings.Contains(name, q):
			score = 1
		case strings.Contains(text, q):
			score = 2
		default:
			continue
		}
		scoredRecords = append(scoredRecords, scored{record: record, score: score})
	}
	sort.SliceStable(scoredRecords, func(i, j int) bool {
		if scoredRecords[i].score != scoredRecords[j].score {
			return scoredRecords[i].score < scoredRecords[j].score
		}
		return strings.ToLower(scoredRecords[i].record.Name) < strings.ToLower(scoredRecords[j].record.Name)
	})
	out := make([]inventory.Record, len(scoredRecords))
	for i, item := range scoredRecords {
		out[i] = item.record
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
