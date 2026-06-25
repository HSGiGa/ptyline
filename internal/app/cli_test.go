package app

import (
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want options
	}{
		{"empty", nil, options{}},
		{"version", []string{"--version"}, options{ShowVersion: true}},
		{"help long", []string{"--help"}, options{ShowHelp: true}},
		{"help short", []string{"-h"}, options{ShowHelp: true}},
		{"init fish", []string{"init", "fish"}, options{InitShell: "fish"}},
		{"child shell", []string{"fish"}, options{Child: []string{"fish"}}},
		{"double dash", []string{"--", "bash", "-l"}, options{Child: []string{"bash", "-l"}}},
		{"config then child", []string{"--config", "/c.toml", "fish"}, options{ConfigPath: "/c.toml", Child: []string{"fish"}}},
		{"config equals then child", []string{"--config=/c.toml", "fish"}, options{ConfigPath: "/c.toml", Child: []string{"fish"}}},
		{"dash command after delimiter", []string{"--", "-weird"}, options{Child: []string{"-weird"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseArgs(c.args)
			if err != nil {
				t.Fatalf("parseArgs(%v) error: %v", c.args, err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseArgs(%v) = %+v, want %+v", c.args, got, c.want)
			}
		})
	}
}

func TestParseArgsErrors(t *testing.T) {
	for _, args := range [][]string{{"--config"}, {"--config="}, {"--unknown"}, {"-x"}, {"init"}} {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%v) expected error, got nil", args)
		}
	}
}
