package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

// Output handles formatted output to a writer in either human-readable or JSON mode.
type Output struct {
	JSON bool
	W    io.Writer
}

// Print writes a single value. In JSON mode it pretty-prints; otherwise uses fmt.
func (o *Output) Print(v any) {
	if o.JSON {
		enc := json.NewEncoder(o.W)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
		return
	}
	fmt.Fprintln(o.W, v)
}

// PrintTable writes a tabular list. In JSON mode it emits a list of objects
// keyed by the column headers.
func (o *Output) PrintTable(headers []string, rows [][]string) {
	if o.JSON {
		list := make([]map[string]string, 0, len(rows))
		for _, row := range rows {
			m := map[string]string{}
			for i, h := range headers {
				if i < len(row) {
					m[h] = row[i]
				}
			}
			list = append(list, m)
		}
		o.Print(list)
		return
	}
	tw := tabwriter.NewWriter(o.W, 0, 4, 2, ' ', 0)
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(tw, "\t")
		}
		fmt.Fprint(tw, h)
	}
	fmt.Fprintln(tw)
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(tw, "\t")
			}
			fmt.Fprint(tw, cell)
		}
		fmt.Fprintln(tw)
	}
	_ = tw.Flush()
}
