package main

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// A barWarning is a size in a sticky or fixed box that isn't a whole pixel.
// While the page scrolls, browsers round such a box to the screen's pixels
// differently from frame to frame, so the bar and its text twitch.
type barWarning struct {
	File string // relative to the app's root
	Line int
	Prop string
	Val  string
	PX   float64
}

func (w barWarning) String() string {
	return fmt.Sprintf("%s:%d\nWarn: %s: %s is %spx in a sticky or fixed box; use whole pixels (%dpx), or it twitches while the page scrolls. (tern)",
		w.File, w.Line, w.Prop, w.Val, strconv.FormatFloat(w.PX, 'f', -1, 64), int(math.Round(w.PX)))
}

// rootFontPX is what rem and em are taken to be: the browsers' default.
const rootFontPX = 16

var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssBlock   = regexp.MustCompile(`\{([^{}]*)\}`)
	styleTag   = regexp.MustCompile(`(?is)<style[^>]*>(.*?)</style>`)
	stickyPos  = regexp.MustCompile(`(?i)(^|;|\s)position\s*:\s*(-webkit-)?(sticky|fixed)\b`)
	cssLength  = regexp.MustCompile(`^(-?\d*\.?\d+)(px|rem|em)?$`)
)

// barProps are the properties that place a bar or size it and its text.
var barProps = map[string]bool{
	"top": true, "bottom": true, "inset": true, "inset-block": true, "inset-block-start": true, "inset-block-end": true,
	"height": true, "min-height": true, "block-size": true, "min-block-size": true,
	"padding": true, "padding-top": true, "padding-bottom": true, "padding-block": true, "padding-block-start": true, "padding-block-end": true,
	"border-bottom-width": true, "border-top-width": true,
	"font-size": true, "line-height": true,
}

// checkBars looks through the app's styles for sticky and fixed boxes
// with sizes off the pixel grid.
func checkBars(root string) ([]barWarning, error) {
	src := filepath.Join(root, "src")
	var out []barWarning
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".svelte" && ext != ".css" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		text := string(raw)
		if ext == ".svelte" {
			text = onlyStyles(text)
		}
		out = append(out, barsIn(rel, text)...)
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

// onlyStyles blanks a component's markup and scripts, keeping newlines,
// so what's left is its CSS on the lines it has in the file.
func onlyStyles(text string) string {
	keep := make([]bool, len(text))
	for _, m := range styleTag.FindAllStringSubmatchIndex(text, -1) {
		for i := m[2]; i < m[3]; i++ {
			keep[i] = true
		}
	}
	b := []byte(text)
	for i := range b {
		if !keep[i] && b[i] != '\n' {
			b[i] = ' '
		}
	}
	return string(b)
}

// barsIn checks the rules in css: a stylesheet, or a component with
// everything but its styles blanked.
func barsIn(file, css string) []barWarning {
	// Blank out comments, keeping their newlines so lines still count.
	css = cssComment.ReplaceAllStringFunc(css, func(c string) string {
		return strings.Map(func(r rune) rune {
			if r == '\n' {
				return r
			}
			return ' '
		}, c)
	})
	var out []barWarning
	for _, m := range cssBlock.FindAllStringSubmatchIndex(css, -1) {
		body := css[m[2]:m[3]]
		if !stickyPos.MatchString(body) {
			continue
		}
		fontPX := float64(rootFontPX)
		decls := splitDecls(body, m[2])
		for _, d := range decls {
			if d.prop == "font-size" {
				if px, ok := toPX(d.val, rootFontPX, false); ok {
					fontPX = px
				}
			}
		}
		for _, d := range decls {
			if !barProps[d.prop] {
				continue
			}
			for _, part := range strings.Fields(d.val) {
				px, ok := toPX(part, fontPX, d.prop == "line-height")
				if d.prop == "font-size" {
					px, ok = toPX(part, rootFontPX, false)
				}
				if !ok || math.Abs(px-math.Round(px)) < 0.01 {
					continue
				}
				line := strings.Count(css[:d.at], "\n") + 1
				out = append(out, barWarning{File: file, Line: line, Prop: d.prop, Val: d.val, PX: math.Round(px*100) / 100})
				break
			}
		}
	}
	return out
}

type cssDecl struct {
	prop, val string
	at        int // the byte offset of the property in the CSS text
}

// splitDecls splits a rule body that starts at offset in css.
func splitDecls(body string, offset int) []cssDecl {
	var out []cssDecl
	pos := offset
	for _, part := range strings.Split(body, ";") {
		prop, val, ok := strings.Cut(part, ":")
		if ok {
			lead := len(part) - len(strings.TrimLeft(part, " \t\r\n"))
			out = append(out, cssDecl{
				prop: strings.ToLower(strings.TrimSpace(prop)),
				val:  strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(val), "!important")),
				at:   pos + lead,
			})
		}
		pos += len(part) + 1
	}
	return out
}

// toPX converts a length to pixels. Unitless numbers are pixels only for
// line-height, where they multiply the font size. calc(), var() and
// percentages can't be known and answer false.
func toPX(v string, fontPX float64, lineHeight bool) (float64, bool) {
	m := cssLength.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	switch m[2] {
	case "px":
		return n, true
	case "rem":
		return n * rootFontPX, true
	case "em":
		return n * fontPX, true
	}
	if lineHeight {
		return n * fontPX, true
	}
	return n, n == 0
}
