//go:build linux

package platform

import (
	"errors"
	"testing"
)

func TestIsWSLFrom(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		release string
		readErr error
		want    bool
	}{
		{name: "WSL environment", env: map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}, want: true},
		{name: "Microsoft release", release: "5.15.153.1-microsoft-standard-WSL2", want: true},
		{name: "WSL release", release: "4.4.0-19041-Microsoft", want: true},
		{name: "native Linux", release: "6.8.0-31-generic", want: false},
		{name: "release unavailable", readErr: errors.New("not found"), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookupEnv := func(key string) (string, bool) {
				value, ok := test.env[key]
				return value, ok
			}
			readFile := func(string) ([]byte, error) {
				return []byte(test.release), test.readErr
			}

			if got := isWSLFrom(lookupEnv, readFile); got != test.want {
				t.Errorf("isWSLFrom() = %t, want %t", got, test.want)
			}
		})
	}
}
