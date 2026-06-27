package renderer

import (
	"strings"

	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/layout"
	"github.com/hsgiga/ptyline/internal/status/width"
)

// TemplateSpec is a pre-parsed template module: its inner blocks, and the
// display options that apply to the assembled result.
type TemplateSpec struct {
	Blocks             []layout.Block
	HideWhenEmpty      bool
	CollapseWhitespace bool
	MaxWidth           int
}

// resolveTemplate assembles a template value from cached module snapshots.
// It never calls any provider — it only reads st.Modules. Template modules
// cannot reference other template modules (enforced at config validation).
func resolveTemplate(st status.StatusState, tmpl TemplateSpec) string {
	var sb strings.Builder
	allEmpty := true
	for _, b := range tmpl.Blocks {
		if b.IsLiteral() {
			sb.WriteString(b.Text)
			continue
		}
		snap, ok := st.Modules[status.ModuleID(b.ModuleID)]
		v := ""
		if ok && snap.Err == nil {
			v = sanitizeDisplayText(snap.Value.Text)
		}
		if v != "" {
			allEmpty = false
		}
		sb.WriteString(v)
	}
	if tmpl.HideWhenEmpty && allEmpty {
		return ""
	}
	result := sb.String()
	if tmpl.CollapseWhitespace {
		result = strings.Join(strings.Fields(result), " ")
	}
	if tmpl.MaxWidth > 0 {
		result = width.Truncate(result, tmpl.MaxWidth, "right")
	}
	return result
}
