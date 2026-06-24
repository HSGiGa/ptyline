package terminal

import "github.com/hsgiga/ptyline/internal/reserved"

// ApplyScrollRegion sets the real terminal scroll region to exclude the reserved
// rows: 1..(rows-reserved). This is what keeps normal scrolling above the status
// bar (spec §6, §10.1). It must be re-applied after alternate-screen transitions
// and resizes.
func (c *Controller) ApplyScrollRegion(size Size, area reserved.Area) {
	bottom := area.ChildRows(size.Rows)
	c.write(SetScrollRegion(1, bottom))
}

// ResetScrollRegion clears scroll margins (used during cleanup).
func (c *Controller) ResetScrollRegion() {
	c.write(ResetScrollRegion)
}
