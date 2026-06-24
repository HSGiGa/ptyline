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
