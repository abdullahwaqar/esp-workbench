package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Every palette color must render as its pinned 256-color index, never as
// the near-black 232 that automatic conversion picked for some of them.
func TestPaletteSurvives256Colors(t *testing.T) {
	r := lipgloss.NewRenderer(nil)
	r.SetColorProfile(termenv.ANSI256)
	r.SetHasDarkBackground(true)

	colors := map[string]lipgloss.CompleteAdaptiveColor{
		"accent": colorAccent, "purple": colorPurple, "success": colorSuccess, "error": colorError,
		"warn": colorWarn, "info": colorInfo, "muted": colorMuted, "text": colorText, "dim": colorDim,
		"border": colorBorder, "cyan": colorPartCyan, "pink": colorPartPink,
	}
	for name, c := range colors {
		out := r.NewStyle().Foreground(c).Render("x")
		if strings.Contains(out, "38;5;232m") || strings.Contains(out, "38;5;16m") {
			t.Errorf("%s renders near-black on a dark 256-color terminal: %q", name, out)
		}
		if !strings.Contains(out, "38;5;"+c.Dark.ANSI256+"m") {
			t.Errorf("%s ignored its pinned 256-color value %s: %q", name, c.Dark.ANSI256, out)
		}
	}

	r.SetHasDarkBackground(false)
	for name, c := range colors {
		if out := r.NewStyle().Foreground(c).Render("x"); !strings.Contains(out, "38;5;"+c.Light.ANSI256+"m") {
			t.Errorf("%s (light) ignored its pinned 256-color value: %q", name, out)
		}
	}
}
