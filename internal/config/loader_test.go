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
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Shell != "bash" || cfg.Bar.Format != "{time}" || cfg.Bar.Height != 1 {
		t.Fatalf("Load() = %+v", cfg)
	}
}

func TestLoadRootConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "ptyline.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bar.Height != 2 || len(cfg.Bar.Rows) != 2 {
		t.Fatalf("root config rows = height %d len %d, want 2 rows", cfg.Bar.Height, len(cfg.Bar.Rows))
	}
	if got, want := cfg.Bar.Rows[0].Format, " {command} || || {git} "; got != want {
		t.Fatalf("root config top row = %q, want %q", got, want)
	}
	if got, want := cfg.Bar.Rows[1].Format, "{ssh} || {user}@{hostname} {cwd} || {env} {runtime} {shell} || {time}"; got != want {
		t.Fatalf("root config main row = %q, want %q", got, want)
	}
	if module := cfg.Modules["command"]; !module.Enabled || module.Format != "{active} {last} {exit} {duration}" {
		t.Fatalf("root command module = %+v", module)
	}
	for _, id := range []string{"user", "runtime", "shell", "env"} {
		if module := cfg.Modules[id]; !module.Enabled {
			t.Fatalf("root %s module = %+v, want enabled", id, module)
		}
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
		{name: "format and block", body: "config_version = 1\n[bar]\nformat = \"{time}\"\n[[bar.block]]\nmodule = \"time\"\nanchor = \"left\"\nalign = \"left\"\nwidth = \"auto\"\ntruncate = \"right\"", key: "bar.format"},
		{name: "bad width", body: "config_version = 1\n[bar]\nformat = \"\"\n[[bar.block]]\nmodule = \"time\"\nanchor = \"left\"\nalign = \"left\"\nwidth = \"101%\"\ntruncate = \"right\"", key: "width"},
		{name: "bad env name", body: "config_version = 1\n[module.env]\nenabled = true\nenv = [\"BAD-NAME\"]", key: "module.env.env"},
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
