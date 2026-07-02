// Package format parses ptyline bar format strings into renderable blocks.
// It is a leaf package with no ptyline-internal imports so that both
// config validation and the layout engine can import it without a cycle
// (spec §8.8, ARCHITECTURE.md §7).
package format

import (
	"strconv"
	"strings"
)

// Anchor is the terminal side a block is pinned to.
type Anchor string

const (
	AnchorLeft   Anchor = "left"
	AnchorCenter Anchor = "center"
	AnchorRight  Anchor = "right"
)

// Align is text alignment within a block's allocated area.
type Align string

const (
	AlignLeft   Align = "left"
	AlignCenter Align = "center"
	AlignRight  Align = "right"
)

// WidthKind enumerates width unit types (spec §8.8).
type WidthKind int

const (
	WidthAuto    WidthKind = iota // size to content
	WidthFill                     // take remaining space
	WidthCells                    // fixed N cells
	WidthPercent                  // N% of bar width
)

// Width is a resolved width spec.
type Width struct {
	Kind  WidthKind
	Value int // cells or percent depending on Kind
}

// Block is one renderable unit with layout metadata. Priority drives graceful
// degradation when the terminal is narrow (ARCHITECTURE.md §8). A block is either
// a module reference (ModuleID set) or a literal run of text (Text set, ModuleID
// empty) produced by parsing the format string.
type Block struct {
	ModuleID  string
	Text      string // literal content when ModuleID == ""
	Separator bool
	Anchor    Anchor
	Align     Align
	Width     Width
	MinWidth  int
	MaxWidth  int
	Truncate  string
	Priority  int
	StyleID   string
}

// IsLiteral reports whether the block is a literal text run (no module).
func (b Block) IsLiteral() bool { return b.ModuleID == "" && !b.Separator }

// IsSeparator reports whether the block is a separator marker (`|` in format).
func (b Block) IsSeparator() bool { return b.Separator }

// ParseFormat parses a bar format string into a slice of Blocks.
//
// Format syntax:
//
//	{moduleid}         — module reference with auto width
//	{moduleid:>8}      — module reference with fixed cell width, right-aligned
//	{moduleid:>20%}    — module reference with percentage width
//	||                 — section boundary (left | center | right)
//	|                  — section-local separator (blank when adjacent modules empty)
//	\|                 — literal "|"
//	anything else      — literal text
func ParseFormat(format string) []Block {
	tokens := tokenize(format)

	var sections [][]Block
	var current []Block
	for _, t := range tokens {
		switch t.kind {
		case tokSplit:
			sections = append(sections, current)
			current = nil
		case tokSep:
			trimTrailingSpace(&current)
			current = append(current, separatorBlock(""))
		case tokPlaceholder:
			current = append(current, placeholderBlock(t.text, ""))
		case tokLiteral:
			text := t.text
			if len(current) > 0 && current[len(current)-1].IsSeparator() {
				text = strings.TrimLeft(text, " \t")
			}
			if text != "" {
				current = append(current, literalBlock(text, ""))
			}
		}
	}
	sections = append(sections, current)

	anchors := sectionAnchors(len(sections))
	var blocks []Block
	for i, section := range sections {
		for j := range section {
			if section[j].Anchor == "" {
				section[j].Anchor = anchors[i]
			}
		}
		blocks = append(blocks, section...)
	}
	return blocks
}

// tokKind identifies token categories produced by the format string scanner.
type tokKind int

const (
	tokLiteral     tokKind = iota // run of literal text (escape-decoded)
	tokPlaceholder                // {name} or {name:spec} expression
	tokSep                        // | separator marker within a section
	tokSplit                      // || section boundary (left/center/right)
)

type fmtTok struct {
	kind tokKind
	text string // set for tokLiteral and tokPlaceholder
}

func tokenize(format string) []fmtTok {
	var tokens []fmtTok
	i := 0
	for i < len(format) {
		switch {
		case format[i] == '|' && i+1 < len(format) && format[i+1] == '|':
			tokens = append(tokens, fmtTok{kind: tokSplit})
			i += 2
		case format[i] == '|':
			tokens = append(tokens, fmtTok{kind: tokSep})
			i++
		case format[i] == '{':
			close := strings.IndexByte(format[i:], '}')
			if close < 0 {
				tokens = append(tokens, fmtTok{kind: tokLiteral, text: format[i:]})
				i = len(format)
			} else {
				close += i
				tokens = append(tokens, fmtTok{kind: tokPlaceholder, text: format[i+1 : close]})
				i = close + 1
			}
		default:
			var b strings.Builder
			for i < len(format) {
				if format[i] == '|' || format[i] == '{' {
					break
				}
				if format[i] == '\\' && i+1 < len(format) && format[i+1] == '|' {
					b.WriteByte('|')
					i += 2
				} else {
					b.WriteByte(format[i])
					i++
				}
			}
			tokens = append(tokens, fmtTok{kind: tokLiteral, text: b.String()})
		}
	}
	return tokens
}

func sectionAnchors(n int) []Anchor {
	switch n {
	case 1:
		return []Anchor{AnchorLeft}
	case 2:
		return []Anchor{AnchorLeft, AnchorRight}
	default:
		anchors := make([]Anchor, n)
		anchors[0] = AnchorLeft
		anchors[n-1] = AnchorRight
		for i := 1; i < n-1; i++ {
			anchors[i] = AnchorCenter
		}
		return anchors
	}
}

func trimTrailingSpace(blocks *[]Block) {
	if len(*blocks) == 0 {
		return
	}
	last := &(*blocks)[len(*blocks)-1]
	if !last.IsLiteral() {
		return
	}
	last.Text = strings.TrimRight(last.Text, " \t")
	if last.Text == "" {
		*blocks = (*blocks)[:len(*blocks)-1]
	}
}

func placeholderBlock(expr string, anchor Anchor) Block {
	name, spec, hasSpec := strings.Cut(expr, ":")
	block := Block{
		ModuleID: name,
		Anchor:   anchor,
		Align:    AlignLeft,
		Width:    Width{Kind: WidthAuto},
		Truncate: "right",
	}
	if !hasSpec {
		return block
	}
	if len(spec) == 1 {
		switch spec[0] {
		case '>':
			block.Anchor = AnchorRight
		case '^':
			block.Anchor = AnchorCenter
		case '<':
			// no-op
		}
		return block
	}
	if strings.HasSuffix(spec[1:], "%") {
		pct, err := strconv.Atoi(strings.TrimSuffix(spec[1:], "%"))
		if err != nil || pct <= 0 || pct > 100 {
			return block
		}
		switch spec[0] {
		case '<':
			block.Align = AlignLeft
		case '^':
			block.Align = AlignCenter
		case '>':
			block.Align = AlignRight
		default:
			return block
		}
		block.Width = Width{Kind: WidthPercent, Value: pct}
		return block
	}
	cells, err := strconv.Atoi(spec[1:])
	if err != nil || cells <= 0 {
		return block
	}
	switch spec[0] {
	case '<':
		block.Align = AlignLeft
	case '^':
		block.Align = AlignCenter
	case '>':
		block.Align = AlignRight
	default:
		return block
	}
	block.Width = Width{Kind: WidthCells, Value: cells}
	return block
}

func literalBlock(text string, anchor Anchor) Block {
	return Block{
		Text:     text,
		Anchor:   anchor,
		Align:    AlignLeft,
		Width:    Width{Kind: WidthAuto},
		Truncate: "none",
		Priority: 1,
	}
}

func separatorBlock(anchor Anchor) Block {
	return Block{
		Separator: true,
		Anchor:    anchor,
		Align:     AlignLeft,
		Width:     Width{Kind: WidthAuto},
		Truncate:  "none",
		Priority:  1,
	}
}
