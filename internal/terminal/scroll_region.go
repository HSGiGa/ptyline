package terminal

import "github.com/hsgiga/ptyline/internal/reserved"

// ApplyScrollRegion sets the real terminal scroll region to exclude the reserved
// rows: 1..(rows-reserved). This is what keeps normal scrolling above the status
// bar (spec §6, §10.1). It must be re-applied after alternate-screen transitions
// and resizes.
//
// DECSTBM homes the cursor as a side effect, which would jump the user's cursor to
// row 1 on every resize and alt-screen exit; the save/restore around it keeps the
// cursor where it was (spec §8.1).
func (c *Controller) ApplyScrollRegion(size Size, area reserved.Area) {
	bottom := area.ChildRows(size.Rows)
	c.write(SaveCursor)
	c.write(SetScrollRegion(1, bottom))
	c.write(RestoreCursor)
}

// ResetScrollRegion clears scroll margins (used during cleanup).
func (c *Controller) ResetScrollRegion() {
	c.write(ResetScrollRegion)
}
