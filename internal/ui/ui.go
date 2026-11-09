// Package ui formats the Tern CLI's terminal output: colors, symbols,
// spinners and tables. Color and animation turn off automatically when
// output is not a terminal, and NO_COLOR and FORCE_COLOR are honored.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Printer writes styled output to w.
type Printer struct {
	w     io.Writer
	color bool
	tty   bool
	mu    sync.Mutex
}

// New returns a Printer for w, detecting whether w is a color terminal.
func New(w io.Writer) *Printer {
	tty := isTerminal(w)
	color := tty
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		color = false
	}
	if v := os.Getenv("FORCE_COLOR"); v != "" && v != "0" {
		color = true
	}
	return &Printer{w: w, color: color, tty: tty}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// Writer returns the underlying writer.
func (p *Printer) Writer() io.Writer { return p.w }

func (p *Printer) style(code, s string) string {
	if !p.color || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Bold styles s in bold.
func (p *Printer) Bold(s string) string { return p.style("1", s) }

// Dim styles s faintly, for secondary information.
func (p *Printer) Dim(s string) string { return p.style("2", s) }

// Green styles s in green, for success.
func (p *Printer) Green(s string) string { return p.style("32", s) }

// Red styles s in red, for errors.
func (p *Printer) Red(s string) string { return p.style("31", s) }

// Yellow styles s in yellow, for warnings.
func (p *Printer) Yellow(s string) string { return p.style("33", s) }

// Cyan styles s in cyan, for commands and URLs.
func (p *Printer) Cyan(s string) string { return p.style("36", s) }

// Brand is Tern's accent color, a teal.
func (p *Printer) Brand(s string) string { return p.style("38;5;37", s) }

// Printf writes formatted output.
func (p *Printer) Printf(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.w, format, args...)
}

// Println writes a line.
func (p *Printer) Println(args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.w, args...)
}

// Success prints a line with a green check.
func (p *Printer) Success(format string, args ...any) {
	p.Printf("  %s %s\n", p.Green("✓"), fmt.Sprintf(format, args...))
}

// Failure prints a line with a red cross.
func (p *Printer) Failure(format string, args ...any) {
	p.Printf("  %s %s\n", p.Red("✗"), fmt.Sprintf(format, args...))
}

// Warn prints a line with a yellow marker.
func (p *Printer) Warn(format string, args ...any) {
	p.Printf("  %s %s\n", p.Yellow("!"), fmt.Sprintf(format, args...))
}

// Event prints a timestamped status line, as used by the dev server.
func (p *Printer) Event(symbol, format string, args ...any) {
	p.Printf("%s %s %s\n", p.Dim(time.Now().Format("15:04:05")), symbol, fmt.Sprintf(format, args...))
}

// Error prints an error with a red label, indenting continuation lines.
func (p *Printer) Error(err error) {
	msg := strings.TrimRight(err.Error(), "\n")
	lines := strings.Split(msg, "\n")
	p.Printf("%s %s\n", p.Red(p.Bold("error")), lines[0])
	for _, l := range lines[1:] {
		p.Printf("      %s\n", l)
	}
}

// Duration formats d compactly: "812ms", "1.4s".
func Duration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// Size formats a byte count: "512 B", "14.2 kB", "10.3 MB".
func Size(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1f kB", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/1000/1000)
	}
}

// Spinner shows an animated step on terminals and a plain line elsewhere.
type Spinner struct {
	p     *Printer
	label string
	start time.Time
	stop  chan struct{}
	done  sync.WaitGroup
}

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spin starts a spinner for a step.
func (p *Printer) Spin(label string) *Spinner {
	s := &Spinner{p: p, label: label, start: time.Now(), stop: make(chan struct{})}
	if !p.tty {
		return s
	}
	s.done.Add(1)
	go func() {
		defer s.done.Done()
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		for i := 0; ; i++ {
			p.Printf("\r  %s %s", p.Brand(frames[i%len(frames)]), label)
			select {
			case <-s.stop:
				return
			case <-t.C:
			}
		}
	}()
	return s
}

// Done ends the spinner with a check, the elapsed time and an optional note.
func (s *Spinner) Done(note string) {
	s.finish(s.p.Green("✓"), note)
}

// Fail ends the spinner with a cross.
func (s *Spinner) Fail() {
	s.finish(s.p.Red("✗"), "")
}

func (s *Spinner) finish(symbol, note string) {
	if s.p.tty {
		close(s.stop)
		s.done.Wait()
		s.p.Printf("\r\x1b[2K")
	}
	extra := s.p.Dim(Duration(time.Since(s.start)))
	if note != "" {
		extra = s.p.Dim(note+" · ") + extra
	}
	s.p.Printf("  %s %s %s\n", symbol, s.label, extra)
}

// Table writes rows as aligned columns. The first row is a header.
func (p *Printer) Table(rows [][]string, style func(col int, cell string) string) {
	if len(rows) == 0 {
		return
	}
	widths := make([]int, len(rows[0]))
	for _, r := range rows {
		for i, c := range r {
			if n := utf8.RuneCountInString(c); n > widths[i] {
				widths[i] = n
			}
		}
	}
	for ri, r := range rows {
		var b strings.Builder
		b.WriteString("  ")
		for i, c := range r {
			pad := strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c))
			switch {
			case ri == 0:
				c = p.Dim(c)
			case style != nil:
				c = style(i, c)
			}
			b.WriteString(c)
			if i < len(r)-1 {
				b.WriteString(pad + "  ")
			}
		}
		p.Println(b.String())
	}
}
