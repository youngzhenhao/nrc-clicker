package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// accentColor is the single-colour override we apply on top of Fyne's
// default theme. Phase 1 deliberately ships one accent only — adding a
// full theme switch is a v2 feature.
var accentColor = color.NRGBA{R: 0x33, G: 0x88, B: 0xFF, A: 0xFF}

// customTheme wraps theme.DefaultTheme and overrides ColorNamePrimary.
type customTheme struct {
	fyne.Theme
}

// newCustomTheme returns a Theme that delegates everything except
// ColorNamePrimary to the Fyne default.
func newCustomTheme() fyne.Theme { return &customTheme{Theme: theme.DefaultTheme()} }

func (t *customTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if name == theme.ColorNamePrimary {
		return accentColor
	}
	return t.Theme.Color(name, fyne.ThemeVariant(0))
}

// TextSize is bumped slightly over the Fyne default so the dashboard
// reads cleanly on a HiDPI laptop screen. We override via SetTheme; the
// actual font size choice happens via a custom theme wrapper too.
func (t *customTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameText {
		return 14
	}
	return t.Theme.Size(name)
}
