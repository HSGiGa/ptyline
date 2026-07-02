package app

import (
	"fmt"

	"github.com/hsgiga/ptyline/internal/app/bar"
	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/pty"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/status/renderer"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// reloadConfig rebuilds the resolved config and bar layout when the project
// overlay path changes. force=true skips the path equality guard (used on
// explicit --reload). Returns true when the config was successfully applied.
//
// Must be called from the event-loop goroutine — it mutates appState fields
// without locking.
func (as *appState) reloadConfig(newProjectPath string, force bool) bool {
	old, _ := as.projectOverlayPath.Load().(string)
	if !force && old == newProjectPath {
		return false
	}
	newCfg, err := config.ApplyOverlays(as.cfg, as.cliOverlay, newProjectPath)
	if err != nil {
		// Don't store newProjectPath: a bad .ptyline should not prevent a retry
		// when the file is fixed and the user cds back into the directory.
		as.diagState.RecordConfigWarning(fmt.Sprintf("overlay %s: %v", newProjectPath, err))
		return false
	}
	as.projectOverlayPath.Store(newProjectPath)
	// Invalidate the per-cwd cache so the next cwd event re-scans parent dirs
	// with the new config in place (the .ptyline locations may have changed).
	for k := range *as.projectConfigCache {
		delete(*as.projectConfigCache, k)
	}
	*as.resolvedCfg = newCfg
	newBarRows := bar.BuildRows(*as.resolvedCfg)
	newArea := reserved.Area{Edge: reserved.Bottom, Rows: uint16(len(newBarRows))}
	if newArea.Rows != as.area.Rows {
		curSize := terminal.Size{Cols: as.termState.Terminal.Cols, Rows: as.termState.Terminal.Rows}
		_ = as.writer.ClearBar()
		*as.area = newArea
		as.filter.SetArea(*as.area)
		as.sup.SetArea(*as.area)
		top, count := bar.Geometry(*as.area, curSize.Rows, len(newBarRows))
		as.writer.SetBarRows(top, count)
		_ = as.sup.Resize(pty.Size{Cols: curSize.Cols, Rows: curSize.Rows})
		as.ctrl.ApplyScrollRegion(curSize, *as.area)
	}
	*as.barRows = newBarRows
	if newVisuals, err := bar.VisualsFromConfig(*as.resolvedCfg, colorMode(as.profile.Capabilities.Color), as.opts.ConfigPath); err == nil {
		*as.visuals = newVisuals
	}
	*as.animState = (*as.render).TakeAnimationState()
	*as.render = renderer.NewWithState(as.newEngine(int(as.termState.Terminal.Cols)), as.visuals.Theme, *as.animState)
	as.configureRenderer(*as.render)
	as.updateModules()
	return true
}
