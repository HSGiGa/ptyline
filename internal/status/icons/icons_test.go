package icons

import "testing"

func TestIconPresetSelection(t *testing.T) {
	const nerd = "" // a Nerd-Font private-use glyph
	cases := []struct {
		name          string
		preset        Preset
		primary, fall string
		want          string
	}{
		{"ascii ignores primary", PresetASCII, nerd, "git", "git"},
		{"nerd uses primary glyph", PresetNerdFont, nerd, "git", nerd},
		{"nerd empty primary falls back", PresetNerdFont, "", "git", "git"},
		{"emoji uses primary glyph", PresetEmoji, "🌿", "git", "🌿"},
		{"emoji empty primary falls back", PresetEmoji, "", "git", "git"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := New(c.preset, true)
			if got := s.Icon(c.primary, c.fall); got != c.want {
				t.Errorf("Icon(%q,%q) preset=%s = %q, want %q", c.primary, c.fall, c.preset, got, c.want)
			}
		})
	}
}

func TestForcedWidth(t *testing.T) {
	if got := New(PresetEmoji, true).ForcedWidth(); got != 0 {
		t.Errorf("auto width = %d, want 0", got)
	}
	s := Set{Preset: PresetEmoji, EmojiWidth: EmojiWidthDouble}
	if got := s.ForcedWidth(); got != 2 {
		t.Errorf("forced double width = %d, want 2", got)
	}
}
