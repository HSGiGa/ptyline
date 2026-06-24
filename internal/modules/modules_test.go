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
