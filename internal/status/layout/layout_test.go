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
