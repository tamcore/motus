package main

import (
	"cmp"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
)

// listFilter is a --filter field: exact matches the whole value, otherwise a
// substring matches. Both compare case-insensitively.
type listFilter[T any] struct {
	get   func(T) string
	exact bool
}

// filterList returns the items matching a field=value filter.
func filterList[T any](items []T, filter string, fields map[string]listFilter[T]) []T {
	name, value, ok := strings.Cut(filter, "=")
	if !ok {
		fatalFn("invalid filter format (expected field=value)", slog.String("filter", filter))
		return nil
	}
	f, ok := fields[strings.ToLower(name)]
	if !ok {
		fatalFn("unknown filter field", slog.String("field", name),
			slog.String("supported", strings.Join(slices.Sorted(maps.Keys(fields)), ", ")))
		return nil
	}
	value = strings.ToLower(value)
	return slices.DeleteFunc(slices.Clone(items), func(item T) bool {
		v := strings.ToLower(f.get(item))
		if f.exact {
			return v != value
		}
		return !strings.Contains(v, value)
	})
}

// sortList sorts items in place by a --sort field; unknown fields sort by "id".
func sortList[T any](items []T, field string, sorts map[string]func(a, b T) int) {
	byField, ok := sorts[strings.ToLower(field)]
	if !ok {
		byField = sorts["id"]
	}
	slices.SortFunc(items, byField)
}

// by returns a comparator on key.
func by[T any, K cmp.Ordered](key func(T) K) func(a, b T) int {
	return func(a, b T) int { return cmp.Compare(key(a), key(b)) }
}

func printJSONTo(w io.Writer, data any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
	}
}

func printCSVTo(w io.Writer, headers []string, rows [][]string) {
	cw := csv.NewWriter(w)
	_ = cw.Write(headers)
	_ = cw.WriteAll(rows)
}

func printTableTo(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
}

// render prints a list in the --output format: json, csv, or table (default).
func render(output string, jsonItems any, headers []string, rows [][]string) {
	switch output {
	case "json":
		printJSONTo(os.Stdout, jsonItems)
	case "csv":
		printCSVTo(os.Stdout, headers, rows)
	default:
		printTableTo(os.Stdout, headers, rows)
	}
}
