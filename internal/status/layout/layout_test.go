package layout

import "testing"

// `||` splits a format into anchor sections: 1→left, 2→left/right, 3→l/c/r.
func TestParseFormatSections(t *testing.T) {
	blocks := ParseFormat("{cwd} || {time}")
	got := map[string]Anchor{}
	for _, b := range blocks {
		if !b.IsLiteral() {
			got[b.ModuleID] = b.Anchor
		}
	}
	if got["cwd"] != AnchorLeft {
		t.Fatalf("cwd anchor = %q, want left", got["cwd"])
	}
	if got["time"] != AnchorRight {
		t.Fatalf("time anchor = %q, want right", got["time"])
	}
}

func TestParseFormatPlaceholderWidthAlign(t *testing.T) {
	blocks := ParseFormat("{cwd:<30} || {git:^20} || {time:>8}")
	got := map[string]Block{}
	for _, b := range blocks {
		if !b.IsLiteral() {
			got[b.ModuleID] = b
		}
	}

	if b := got["cwd"]; b.Width.Kind != WidthCells || b.Width.Value != 30 || b.Align != AlignLeft {
		t.Fatalf("cwd block = %+v, want width=30 align=left", b)
	}
	if b := got["git"]; b.Width.Kind != WidthCells || b.Width.Value != 20 || b.Align != AlignCenter {
		t.Fatalf("git block = %+v, want width=20 align=center", b)
	}
	if b := got["time"]; b.Width.Kind != WidthCells || b.Width.Value != 8 || b.Align != AlignRight {
		t.Fatalf("time block = %+v, want width=8 align=right", b)
	}
}

func TestParseFormatPlaceholderAnchorOverride(t *testing.T) {
	// {b:>} and {c:^} override anchor; {a:<} is a no-op (keeps section anchor = left).
	blocks := ParseFormat("{a:<} {b:>} {c:^}")
	got := map[string]Block{}
	for _, b := range blocks {
		if !b.IsLiteral() {
			got[b.ModuleID] = b
		}
	}

	if b := got["a"]; b.Anchor != AnchorLeft || b.Width.Kind != WidthAuto {
		t.Fatalf("a: got anchor=%q width=%v, want AnchorLeft WidthAuto", b.Anchor, b.Width.Kind)
	}
	if b := got["b"]; b.Anchor != AnchorRight || b.Width.Kind != WidthAuto {
		t.Fatalf("b: got anchor=%q width=%v, want AnchorRight WidthAuto", b.Anchor, b.Width.Kind)
	}
	if b := got["c"]; b.Anchor != AnchorCenter || b.Width.Kind != WidthAuto {
		t.Fatalf("c: got anchor=%q width=%v, want AnchorCenter WidthAuto", b.Anchor, b.Width.Kind)
	}
}

func TestParseFormatPlaceholderLtNoReorder(t *testing.T) {
	// {shell:<} in the center section must NOT move shell to left anchor.
	blocks := ParseFormat("{identity} || {runtime} {shell:<} || {time}")
	got := map[string]Block{}
	for _, b := range blocks {
		if !b.IsLiteral() {
			got[b.ModuleID] = b
		}
	}
	if b := got["shell"]; b.Anchor != AnchorCenter {
		t.Fatalf("shell anchor = %q, want center (no reorder from <)", b.Anchor)
	}
}

func TestParseFormatPlaceholderPercentAlign(t *testing.T) {
	blocks := ParseFormat("{a:>20%} || {b:^10%} || {c:<5%}")
	got := map[string]Block{}
	for _, b := range blocks {
		if !b.IsLiteral() {
			got[b.ModuleID] = b
		}
	}

	if b := got["a"]; b.Width.Kind != WidthPercent || b.Width.Value != 20 || b.Align != AlignRight {
		t.Fatalf("a block = %+v, want WidthPercent(20) AlignRight", b)
	}
	if b := got["b"]; b.Width.Kind != WidthPercent || b.Width.Value != 10 || b.Align != AlignCenter {
		t.Fatalf("b block = %+v, want WidthPercent(10) AlignCenter", b)
	}
	if b := got["c"]; b.Width.Kind != WidthPercent || b.Width.Value != 5 || b.Align != AlignLeft {
		t.Fatalf("c block = %+v, want WidthPercent(5) AlignLeft", b)
	}
}

func TestParseFormatPlaceholderInvalidSuffix(t *testing.T) {
	// Unknown single char, invalid percent, unknown align with percent → all fall back to WidthAuto AlignLeft
	cases := []struct {
		format string
		id     string
	}{
		{"{a:!}", "a"},
		{"{b:>0%}", "b"},
		{"{c:>101%}", "c"},
		{"{d:x20%}", "d"},
	}
	for _, tc := range cases {
		blocks := ParseFormat(tc.format)
		var b Block
		for _, bl := range blocks {
			if bl.ModuleID == tc.id {
				b = bl
				break
			}
		}
		if b.Width.Kind != WidthAuto || b.Align != AlignLeft {
			t.Fatalf("format %q: block = %+v, want WidthAuto AlignLeft", tc.format, b)
		}
	}
}

func TestParseFormatSeparatorMarkers(t *testing.T) {
	blocks := ParseFormat("{env} | {runtime}||{time}")
	if len(blocks) != 4 {
		t.Fatalf("block count = %d, want 4: %+v", len(blocks), blocks)
	}
	if !blocks[1].IsSeparator() {
		t.Fatalf("second block = %+v, want separator marker", blocks[1])
	}
	if blocks[1].Anchor != AnchorLeft {
		t.Fatalf("separator anchor = %q, want left", blocks[1].Anchor)
	}
	if blocks[3].ModuleID != "time" || blocks[3].Anchor != AnchorRight {
		t.Fatalf("right section time block = %+v", blocks[3])
	}
}

func TestParseFormatEscapedPipeLiteral(t *testing.T) {
	blocks := ParseFormat(`{env} \| {runtime}`)
	if len(blocks) != 3 {
		t.Fatalf("block count = %d, want 3: %+v", len(blocks), blocks)
	}
	if !blocks[1].IsLiteral() || blocks[1].Text != " | " {
		t.Fatalf("escaped pipe block = %+v, want literal pipe text", blocks[1])
	}
}

func TestResolveWidthPercent(t *testing.T) {
	e := New(100)
	w := e.resolveWidth(Block{Width: Width{Kind: WidthPercent, Value: 25}}, 10)
	if w != 25 {
		t.Fatalf("25%% of 100 = %d, want 25", w)
	}
}

// On overflow the lowest-priority block is dropped instead of wrapping the bar.
func TestArrangePriorityOverflow(t *testing.T) {
	e := New(10)
	blocks := []Block{
		{Text: "low", Width: Width{Kind: WidthCells, Value: 6}, Priority: 1},
		{Text: "high", Width: Width{Kind: WidthCells, Value: 6}, Priority: 2},
	}
	p := e.Arrange(blocks, []int{6, 6})
	if p[0].Visible {
		t.Fatal("low-priority block should be dropped on overflow")
	}
	if !p[1].Visible {
		t.Fatal("high-priority block should stay visible")
	}
}

// Right-anchored blocks are dropped before center, which is dropped before left,
// when all blocks share the same priority and the bar is too narrow.
func TestArrangeDropFromRight(t *testing.T) {
	// bar=13; three 6-cell blocks total 18 > 13. Two fit (6+6=12 ≤ 13).
	// Sort order (descending anchor): left, center, right → right is dropped.
	e := New(13)
	blocks := []Block{
		{ModuleID: "left1", Anchor: AnchorLeft, Width: Width{Kind: WidthCells, Value: 6}},
		{ModuleID: "center1", Anchor: AnchorCenter, Width: Width{Kind: WidthCells, Value: 6}},
		{ModuleID: "right1", Anchor: AnchorRight, Width: Width{Kind: WidthCells, Value: 6}},
	}
	p := e.Arrange(blocks, []int{6, 6, 6})
	if !p[0].Visible {
		t.Error("left block should be visible")
	}
	if !p[1].Visible {
		t.Error("center block should be visible (left+center=12 fits in 13)")
	}
	if p[2].Visible {
		t.Error("right block should be dropped first")
	}

	// bar=7; only one 6-cell block fits → left kept, center and right dropped.
	e2 := New(7)
	p2 := e2.Arrange(blocks, []int{6, 6, 6})
	if !p2[0].Visible {
		t.Error("left block should be kept when only one slot fits")
	}
	if p2[1].Visible || p2[2].Visible {
		t.Error("center and right should be dropped when only left fits")
	}
}

// Blocks narrower than minBlockWidth are hidden even when they fit.
func TestArrangeMinBlockWidth(t *testing.T) {
	e := NewWithMinBlock(20, 5)
	blocks := []Block{
		{ModuleID: "big", Anchor: AnchorLeft, Width: Width{Kind: WidthCells, Value: 10}},
		{ModuleID: "tiny", Anchor: AnchorRight, Width: Width{Kind: WidthCells, Value: 3}},
	}
	p := e.Arrange(blocks, []int{10, 3})
	if !p[0].Visible {
		t.Error("big block (10 >= 5) should be visible")
	}
	if p[1].Visible {
		t.Error("tiny block (3 < 5) should be hidden by minBlockWidth")
	}
}

// WidthFill blocks are exempt from minBlockWidth (they expand to fill anyway).
func TestArrangeMinBlockWidthFillExempt(t *testing.T) {
	e := NewWithMinBlock(20, 5)
	blocks := []Block{
		{ModuleID: "filler", Anchor: AnchorLeft, Width: Width{Kind: WidthFill}},
	}
	p := e.Arrange(blocks, []int{0})
	if !p[0].Visible {
		t.Error("WidthFill block should be visible regardless of minBlockWidth")
	}
}

// A fill block expands to consume the leftover bar width.
func TestArrangeFillExpands(t *testing.T) {
	e := New(20)
	blocks := []Block{
		{Text: "x", Width: Width{Kind: WidthCells, Value: 5}},
		{Text: "y", Width: Width{Kind: WidthFill}},
	}
	p := e.Arrange(blocks, []int{5, 1})
	if !p[1].Visible || p[1].Width != 15 {
		t.Fatalf("fill width = %d (visible=%t), want 15", p[1].Width, p[1].Visible)
	}
}
