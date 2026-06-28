package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.toml")
	content := `config_version = 1
shell = "bash"
[bar]
format = "{time}"
[module.time]
enabled = true
animation = true
[style.time]
animation = "pulse"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Shell != "bash" || cfg.Bar.Format != "{time}" {
		t.Fatalf("Load() = %+v", cfg)
	}
	if got := cfg.Modules["time"].Animation; got != AnimationDefault {
		t.Fatalf("module.time.animation = %q, want default", got)
	}
	if got := cfg.Styles["time"].Animation; got != "pulse" {
		t.Fatalf("style.time.animation = %q, want pulse", got)
	}
}

func TestLoadRootConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Bar.Rows) != 2 {
		t.Fatalf("root config rows = %d, want 2", len(cfg.Bar.Rows))
	}
	if got, want := cfg.Bar.Rows[0].Format, " {command} || || {git} "; got != want {
		t.Fatalf("root config top row = %q, want %q", got, want)
	}
	if got, want := cfg.Bar.Rows[1].Format, "{identity} || {env} | {runtime} | {shell} || {gh} | {time}"; got != want {
		t.Fatalf("root config main row = %q, want %q", got, want)
	}
	if got, want := cfg.Bar.Rows[1].Separator, ":"; got != want {
		t.Fatalf("root config main row separator = %q, want %q", got, want)
	}
	if module := cfg.Modules["command"]; !module.Enabled || module.Format != "{active} {last} | {duration} | {exit}" || module.Separator != "•" {
		t.Fatalf("root command module = %+v", module)
	}
	if got := cfg.Modules["command"].Animation; got != AnimationDefault {
		t.Fatalf("root command animation = %q, want default", got)
	}
	if module := cfg.Modules["gh"]; module.Source != "exec" || module.Command == "" {
		t.Fatalf("root gh module = %+v, want source=exec with command", module)
	}

	if got, want := cfg.Modules["env"].Env, []string{"PTYLINE_ENV"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root env module env = %q, want %q", got, want)
	}
}

func TestDefaultTopRowSpacing(t *testing.T) {
	cfg := Default()
	if len(cfg.Bar.Rows) == 0 {
		t.Fatal("Default() has no bar rows")
	}
	if got, want := cfg.Bar.Rows[0].Format, ""; got != want {
		t.Fatalf("top row format = %q, want %q", got, want)
	}
	if got, want := cfg.Bar.Rows[1].Format, "|| {time}"; got != want {
		t.Fatalf("main row format = %q, want %q", got, want)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		body string
		key  string
	}{
		{name: "missing version", body: "shell = \"bash\"", key: "config_version"},
		{name: "unknown key", body: "config_version = 1\nunknown = true", key: "unknown"},
		{name: "bad justify", body: "config_version = 1\n[bar]\nformat = \"{time}\"\njustify = \"middle\"", key: "bar.justify"},
		{name: "bad env name", body: "config_version = 1\n[module.env]\nenabled = true\nenv = [\"BAD-NAME\"]", key: "module.env.env"},
		{name: "bad source", body: "config_version = 1\n[module.foo]\nsource = \"socket\"\ncommand = \"echo hi\"", key: "module.foo.source"},
		{name: "exec source without command", body: "config_version = 1\n[module.foo]\nsource = \"exec\"", key: "module.foo.command"},
		{name: "refresh command without command", body: "config_version = 1\n[module.foo]\nrefresh_on_command = [\"foo login\"]", key: "module.foo.command"},
		{name: "empty refresh command", body: "config_version = 1\n[module.foo]\ncommand = \"echo hi\"\nrefresh_on_command = [\"  \"]", key: "module.foo.refresh_on_command"},
		{name: "bad icon position", body: "config_version = 1\n[module.git]\nicon = \"top\"", key: "module.git.icon"},
		{name: "template without format", body: "config_version = 1\n[module.identity]\nsource = \"template\"", key: "module.identity.format"},
		{name: "template self-reference", body: "config_version = 1\n[module.identity]\nsource = \"template\"\nformat = \"{identity} foo\"", key: "module.identity"},
		{name: "template-in-template", body: "config_version = 1\n[module.a]\nsource = \"template\"\nformat = \"{user}\"\n[module.b]\nsource = \"template\"\nformat = \"{a} bar\"", key: "module.b"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("Load() error = %v, want key %q", err, test.key)
			}
		})
	}
}

func TestLoadCustomModuleSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `config_version = 1

[bar]
format = "{gh} || {time_local}"

[module.gh]
command = "printf octo"
refresh_on_command = ["gh auth login"]
icon = "left"
icon_glyph = ""
icon_fallback = "gh"

[module.time_local]
source = "time"
format = "%H:%M"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Modules["gh"].Source; got != "" {
		t.Fatalf("gh source = %q, want empty config value so app defaults unknown IDs to exec", got)
	}
	if got := cfg.Modules["gh"].Command; got != "printf octo" {
		t.Fatalf("gh command = %q", got)
	}
	if got := cfg.Modules["gh"].RefreshOnCommand; !reflect.DeepEqual(got, []string{"gh auth login"}) {
		t.Fatalf("gh refresh_on_command = %q", got)
	}
	if got := cfg.Modules["gh"].Icon; got != "left" {
		t.Fatalf("gh icon = %q, want left", got)
	}
	if got := cfg.Modules["gh"].IconGlyph; got != "" {
		t.Fatalf("gh icon_glyph = %q", got)
	}
	if got := cfg.Modules["gh"].IconFallback; got != "gh" {
		t.Fatalf("gh icon_fallback = %q", got)
	}
	if got := cfg.Modules["time_local"].Source; got != "time" {
		t.Fatalf("time_local source = %q, want time", got)
	}
}

func TestLoadTemplateModule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `config_version = 1

[bar]
format = "{identity} || {time}"

[module.identity]
source = "template"
format = "{user}@{hostname} {cwd}"
collapse_whitespace = true
hide_when_empty = true
max_width = 60
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	m := cfg.Modules["identity"]
	if m.Source != "template" {
		t.Fatalf("source = %q, want template", m.Source)
	}
	if m.Format != "{user}@{hostname} {cwd}" {
		t.Fatalf("format = %q", m.Format)
	}
	if !m.CollapseWhitespace {
		t.Fatalf("collapse_whitespace not set")
	}
	if !m.HideWhenEmpty {
		t.Fatalf("hide_when_empty not set")
	}
	if m.MaxWidth != 60 {
		t.Fatalf("max_width = %d, want 60", m.MaxWidth)
	}
}

func TestMigrateToLatest(t *testing.T) {
	got, err := migrateToLatest([]byte("config_version = 0\nshell = \"bash\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "config_version = 1\nshell = \"bash\"\n" {
		t.Fatalf("migrateToLatest() = %q", got)
	}
}

func TestFindProjectConfig(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	child := filepath.Join(project, "nested")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(project, ".ptyline")
	if err := os.WriteFile(configPath, []byte("config_version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := FindProjectConfig(child)
	if !ok || got != configPath {
		t.Fatalf("FindProjectConfig() = (%q, %t), want (%q, true)", got, ok, configPath)
	}
}

func TestResolveOverlayPath(t *testing.T) {
	// Full path returned as-is.
	if got := ResolveOverlayPath("/tmp/my.ptyline"); got != "/tmp/my.ptyline" {
		t.Errorf("ResolveOverlayPath(/tmp/my.ptyline) = %q, want as-is", got)
	}
	// Relative path with separator returned as-is.
	if got := ResolveOverlayPath("./compact.ptyline"); got != "./compact.ptyline" {
		t.Errorf("ResolveOverlayPath(./compact.ptyline) = %q, want as-is", got)
	}
	// Empty string returns empty.
	if got := ResolveOverlayPath(""); got != "" {
		t.Errorf("ResolveOverlayPath() = %q, want empty", got)
	}
	// Short name resolves to config dir.
	got := ResolveOverlayPath("compact")
	if !strings.HasSuffix(got, filepath.Join("ptyline", "compact.ptyline")) {
		t.Errorf("ResolveOverlayPath(compact) = %q, want ...ptyline/compact.ptyline", got)
	}
}

func TestValidateOverlayScope_ForbiddenFields(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	tests := []struct {
		name string
		body string
		want string // substring expected in error
	}{
		{
			name: "shell forbidden",
			body: "config_version = 1\nshell = \"zsh\"\n",
			want: "shell",
		},
		{
			name: "refresh_interval_ms forbidden",
			body: "config_version = 1\nrefresh_interval_ms = 500\n",
			want: "refresh_interval_ms",
		},
		{
			name: "module command forbidden",
			body: "config_version = 1\n[module.kube]\ncommand = \"kubectl config current-context\"\n",
			want: "module.kube.command",
		},
		{
			name: "module timeout_ms forbidden",
			body: "config_version = 1\n[module.kube]\ntimeout_ms = 200\n",
			want: "module.kube.timeout_ms",
		},
		{
			name: "module refresh_on_command forbidden",
			body: "config_version = 1\n[module.kube]\nrefresh_on_command = [\"kubectl config use-context\"]\n",
			want: "module.kube.refresh_on_command",
		},
		{
			name: "module source forbidden",
			body: "config_version = 1\n[module.kube]\nsource = \"time\"\n",
			want: "module.kube.source",
		},
		{
			name: "module provider=command forbidden",
			body: "config_version = 1\n[module.kube]\nprovider = \"command\"\n",
			want: "module.kube.provider",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := write(tc.name+".ptyline", tc.body)
			_, err := ApplyOverlays(Default(), path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ApplyOverlays() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestMergeOverlay_MapMergeByKey(t *testing.T) {
	dir := t.TempDir()
	overlayPath := filepath.Join(dir, "overlay.ptyline")
	overlayBody := `config_version = 1

[module.env]
env = ["PROJECT_ENV"]

[style.myblock]
fg = "#ff0000"
`
	if err := os.WriteFile(overlayPath, []byte(overlayBody), 0o600); err != nil {
		t.Fatal(err)
	}
	base := Default()
	base.Modules["time"] = ModuleConfig{Enabled: true, Format: "%H:%M:%S"}

	result, err := ApplyOverlays(base, overlayPath)
	if err != nil {
		t.Fatal(err)
	}
	// time module should still exist (not replaced by overlay)
	if m := result.Modules["time"]; !m.Enabled || m.Format != "%H:%M:%S" {
		t.Errorf("time module = %+v, want preserved", m)
	}
	// env module env list should be replaced
	if got, want := result.Modules["env"].Env, []string{"PROJECT_ENV"}; !reflect.DeepEqual(got, want) {
		t.Errorf("env.Env = %v, want %v", got, want)
	}
	// style added by overlay
	if s, ok := result.Styles["myblock"]; !ok || s.FG != "#ff0000" {
		t.Errorf("styles[myblock] = %+v, want fg=#ff0000", s)
	}
}

func TestMergeOverlay_SliceReplaces(t *testing.T) {
	dir := t.TempDir()
	overlayPath := filepath.Join(dir, "overlay.ptyline")
	overlayBody := `config_version = 1

[[bar.row]]
format = "{hostname} || {time}"
`
	if err := os.WriteFile(overlayPath, []byte(overlayBody), 0o600); err != nil {
		t.Fatal(err)
	}
	base := Default() // has 2 rows
	result, err := ApplyOverlays(base, overlayPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(result.Bar.Rows); got != 1 {
		t.Errorf("bar.row count = %d, want 1 (overlay replaced the slice)", got)
	}
	if got, want := result.Bar.Rows[0].Format, "{hostname} || {time}"; got != want {
		t.Errorf("bar.row[0].format = %q, want %q", got, want)
	}
}

func TestMergeOverlay_BoolExplicit(t *testing.T) {
	dir := t.TempDir()
	disablePath := filepath.Join(dir, "disable.ptyline")
	disableBody := `config_version = 1

[module.git]
enabled = false
`
	if err := os.WriteFile(disablePath, []byte(disableBody), 0o600); err != nil {
		t.Fatal(err)
	}
	base := Default()
	base.Bar.Format = "{cwd} || {git} || {time}"
	// git is referenced in bar format, but overlay explicitly disables it
	result, err := ApplyOverlays(base, disablePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Modules["git"].Enabled {
		t.Error("git module should remain disabled (explicit enabled=false beats inference)")
	}
}

// min_block_width = 0 in an overlay must disable a non-zero base value.
// Uses meta.IsDefined so that explicitly writing 0 is treated as a reset.
func TestMergeOverlay_MinBlockWidthReset(t *testing.T) {
	dir := t.TempDir()
	overlayPath := filepath.Join(dir, "reset.ptyline")
	overlayBody := `config_version = 1

[bar]
min_block_width = 0
`
	if err := os.WriteFile(overlayPath, []byte(overlayBody), 0o600); err != nil {
		t.Fatal(err)
	}
	base := Default()
	base.Bar.MinBlockWidth = 10
	result, err := ApplyOverlays(base, overlayPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Bar.MinBlockWidth != 0 {
		t.Errorf("min_block_width = %d after overlay reset, want 0", result.Bar.MinBlockWidth)
	}
}

func TestInferActiveModules(t *testing.T) {
	dir := t.TempDir()
	overlayPath := filepath.Join(dir, "overlay.ptyline")
	// overlay changes the format to include hostname; hostname is not enabled in Default
	overlayBody := `config_version = 1

[bar]
format = "{hostname} || {time}"
`
	if err := os.WriteFile(overlayPath, []byte(overlayBody), 0o600); err != nil {
		t.Fatal(err)
	}
	base := Default()
	result, err := ApplyOverlays(base, overlayPath)
	if err != nil {
		t.Fatal(err)
	}
	// hostname not in Default modules but referenced in format → should be auto-enabled
	if m := result.Modules["hostname"]; !m.Enabled {
		t.Error("hostname module should be auto-enabled by bar format reference")
	}
	// time still enabled
	if m := result.Modules["time"]; !m.Enabled {
		t.Error("time module should remain enabled")
	}
}

func TestApplyOverlays_Layering(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	// CLI overlay sets format to single block
	cliPath := write("cli.ptyline", "config_version = 1\n[bar]\nformat = \"{time}\"\n")
	// Project overlay overrides format (highest precedence)
	projPath := write("proj.ptyline", "config_version = 1\n[bar]\nformat = \"{hostname}\"\n")

	result, err := ApplyOverlays(Default(), cliPath, projPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Bar.Format, "{hostname}"; got != want {
		t.Errorf("bar.format = %q, want %q (project overlay wins)", got, want)
	}
}
