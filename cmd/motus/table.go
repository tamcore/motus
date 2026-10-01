package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

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
	for _, row := range rows {
		_ = cw.Write(row)
	}
	cw.Flush()
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
