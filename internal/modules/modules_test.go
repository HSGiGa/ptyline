package modules

import "testing"

func TestGoTimeLayout(t *testing.T) {
	if got := goTimeLayout("%H:%M:%S"); got != "15:04:05" {
		t.Fatalf("goTimeLayout() = %q", got)
	}
}

func TestAbbreviateHome(t *testing.T) {
	if got := AbbreviateHome("/home/user/project", "/home/user"); got != "~/project" {
		t.Fatalf("AbbreviateHome() = %q", got)
	}
}

func TestFormatActiveCommand(t *testing.T) {
	if got := FormatActiveCommand("go test ./...", "[{command}]", 20); got != "[go test ./...]" {
		t.Fatalf("FormatActiveCommand() = %q", got)
	}
	if got := FormatActiveCommand("abcdefghijklmnopqrstuvwxyz", "{command}", 10); got != "abcdefghi…" {
		t.Fatalf("truncated command = %q", got)
	}
	if got := FormatActiveCommand("", "{command}", 10); got != "" {
		t.Fatalf("empty command = %q", got)
	}
}
