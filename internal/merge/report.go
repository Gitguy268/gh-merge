package merge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode"
)

// Style colours output; the zero value prints plain text.
type Style struct{ Color bool }

func (s Style) wrap(code, text string) string {
	if !s.Color {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// Action returns the action word, coloured by outcome.
func (s Style) Action(a string) string {
	switch a {
	case ActMerged, ActWouldMerge:
		return s.wrap("32", a)
	case ActAutoMerge:
		return s.wrap("36", a)
	case ActFailed:
		return s.wrap("31", a)
	default:
		return s.wrap("33", a)
	}
}

// Dim greys out text.
func (s Style) Dim(t string) string { return s.wrap("2", t) }

// clean removes control characters (escape sequences, bells, newlines) from
// text that comes from GitHub, so a pull request title can't drive the terminal.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || unicode.Is(unicode.Bidi_Control, r) {
			return -1
		}
		return r
	}, s)
}

func truncate(s string, n int) string {
	s = clean(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Line formats one result as a single line, for progress output.
func Line(s Style, r Result) string {
	pad := strings.Repeat(" ", max(0, len(ActAutoMerge)-len(r.Action)))
	line := fmt.Sprintf("%s%s  %s#%d  %s", s.Action(r.Action), pad, clean(r.Repo), r.Number, truncate(r.Title, 56))
	if r.Reason != "" {
		line += "  " + s.Dim("("+clean(r.Reason)+")")
	}
	return line
}

// Table prints results as aligned columns.
func Table(w io.Writer, s Style, rs []Result) {
	// Lay out plain text first; colour codes would throw off tabwriter's widths.
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ACTION\tPR\tAUTHOR\tMETHOD\tTITLE\tNOTE")
	for _, r := range rs {
		fmt.Fprintf(tw, "%s\t%s#%d\t%s\t%s\t%s\t%s\n",
			r.Action, clean(r.Repo), r.Number, truncate(r.Author, 16), r.Method,
			truncate(r.Title, 46), clean(r.Reason))
	}
	tw.Flush()
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, " ")
		if i == 0 {
			line = s.Dim(line)
		} else {
			a := rs[i-1].Action
			line = s.Action(a) + line[len(a):]
		}
		io.WriteString(w, line+"\n")
	}
}

// Report is the --json document.
type Report struct {
	DryRun    bool        `json:"dryRun"`
	Confirmed bool        `json:"confirmed"`
	Repos     []string    `json:"repos"`
	Results   []Result    `json:"results"`
	Errors    []RepoError `json:"errors"`
	Totals    Totals      `json:"totals"`
}

// WriteJSON prints the report as indented JSON.
func WriteJSON(w io.Writer, r Report) error {
	if r.Results == nil {
		r.Results = []Result{}
	}
	if r.Errors == nil {
		r.Errors = []RepoError{}
	}
	if r.Repos == nil {
		r.Repos = []string{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
