package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// TableWriter wraps tabwriter for formatted CLI table output.
type TableWriter struct {
	w *tabwriter.Writer
}

// NewTableWriter creates a new TableWriter writing to the given output.
func NewTableWriter(out io.Writer) *TableWriter {
	return &TableWriter{
		w: tabwriter.NewWriter(out, 0, 8, 2, ' ', 0),
	}
}

// WriteHeader writes a header row to the table.
func (t *TableWriter) WriteHeader(headers ...string) {
	for i, h := range headers {
		if i > 0 {
			_, _ = fmt.Fprint(t.w, "\t")
		}
		_, _ = fmt.Fprint(t.w, h)
	}
	_, _ = fmt.Fprintln(t.w)
}

// WriteRow writes a data row to the table.
func (t *TableWriter) WriteRow(cols ...string) {
	for i, c := range cols {
		if i > 0 {
			_, _ = fmt.Fprint(t.w, "\t")
		}
		_, _ = fmt.Fprint(t.w, c)
	}
	_, _ = fmt.Fprintln(t.w)
}

// Flush flushes the underlying tabwriter.
func (t *TableWriter) Flush() {
	_ = t.w.Flush()
}

// printJSONTo encodes data as indented JSON to w.
func printJSONTo(w io.Writer, data interface{}) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
	}
}

// printJSON encodes data as indented JSON to stdout.
func printJSON(data interface{}) { printJSONTo(os.Stdout, data) }

// printCSVTo writes rows as CSV to w.
func printCSVTo(w io.Writer, headers []string, rows [][]string) {
	cw := csv.NewWriter(w)
	_ = cw.Write(headers)
	for _, row := range rows {
		_ = cw.Write(row)
	}
	cw.Flush()
}

// printCSV writes rows as CSV to stdout.
func printCSV(headers []string, rows [][]string) { printCSVTo(os.Stdout, headers, rows) }
