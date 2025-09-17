package prompt

import (
	"fmt"
	"strings"
)

// ColorScheme defines the color configuration for the prompt.
type ColorScheme struct {
	Name       string           `json:"name"`
	Prefix     Color            `json:"prefix"`
	Input      Color            `json:"input"`
	Suggestion SuggestionColors `json:"suggestion"`
	Selected   Color            `json:"selected"`
	Background *Color           `json:"background"` // nil for transparent
	Cursor     Color            `json:"cursor"`
}

// SuggestionColors defines colors for completion suggestions.
type SuggestionColors struct {
	Text        Color  `json:"text"`
	Description Color  `json:"description"`
	Match       Color  `json:"match"`      // Highlight color for matching parts
	Background  *Color `json:"background"` // nil for transparent
}

// Color represents an RGB color with optional formatting.
type Color struct {
	R    uint8 `json:"r"`
	G    uint8 `json:"g"`
	B    uint8 `json:"b"`
	Bold bool  `json:"bold"`
}

// ThemeDefault is the default color scheme with green prefix and white text
var ThemeDefault = &ColorScheme{
	Name:   "default",
	Prefix: Color{R: 0, G: 255, B: 0, Bold: true},
	Input:  Color{R: 255, G: 255, B: 255, Bold: true},
	Suggestion: SuggestionColors{
		Text:        Color{R: 200, G: 200, B: 200, Bold: false},
		Description: Color{R: 128, G: 128, B: 128, Bold: false},
		Match:       Color{R: 255, G: 255, B: 0, Bold: true},
		Background:  nil,
	},
	Selected:   Color{R: 0, G: 255, B: 255, Bold: true},
	Background: nil,
	Cursor:     Color{R: 255, G: 255, B: 255, Bold: true},
}

// ThemeDark is a dark theme with light blue prefix and off-white text
var ThemeDark = &ColorScheme{
	Name:   "Dark",
	Prefix: Color{R: 102, G: 217, B: 239, Bold: true},
	Input:  Color{R: 248, G: 248, B: 242, Bold: false},
	Suggestion: SuggestionColors{
		Text:        Color{R: 189, G: 147, B: 249, Bold: false},
		Description: Color{R: 98, G: 114, B: 164, Bold: false},
		Match:       Color{R: 255, G: 184, B: 108, Bold: true},
		Background:  nil,
	},
	Selected:   Color{R: 80, G: 250, B: 123, Bold: true},
	Background: &Color{R: 40, G: 42, B: 54},
	Cursor:     Color{R: 248, G: 248, B: 242, Bold: false},
}

// ThemeLight is a light theme with blue prefix and dark gray text
var ThemeLight = &ColorScheme{
	Name:   "Light",
	Prefix: Color{R: 0, G: 119, B: 187, Bold: true},
	Input:  Color{R: 36, G: 41, B: 46, Bold: false},
	Suggestion: SuggestionColors{
		Text:        Color{R: 88, G: 96, B: 105, Bold: false},
		Description: Color{R: 149, G: 157, B: 165, Bold: false},
		Match:       Color{R: 215, G: 58, B: 73, Bold: true},
		Background:  nil,
	},
	Selected:   Color{R: 40, G: 167, B: 69, Bold: true},
	Background: &Color{R: 255, G: 255, B: 255},
	Cursor:     Color{R: 36, G: 41, B: 46, Bold: false},
}

// ThemeSolarizedDark is the Solarized Dark color scheme
var ThemeSolarizedDark = &ColorScheme{
	Name:   "Solarized Dark",
	Prefix: Color{R: 133, G: 153, B: 0, Bold: true},
	Input:  Color{R: 147, G: 161, B: 161, Bold: false},
	Suggestion: SuggestionColors{
		Text:        Color{R: 131, G: 148, B: 150, Bold: false},
		Description: Color{R: 88, G: 110, B: 117, Bold: false},
		Match:       Color{R: 181, G: 137, B: 0, Bold: true},
		Background:  nil,
	},
	Selected:   Color{R: 38, G: 139, B: 210, Bold: true},
	Background: &Color{R: 0, G: 43, B: 54},
	Cursor:     Color{R: 253, G: 246, B: 227, Bold: false},
}

// ThemeAccessible is a colorblind-safe theme with high contrast
var ThemeAccessible = &ColorScheme{
	Name:   "Accessible",
	Prefix: Color{R: 0, G: 114, B: 178, Bold: true},
	Input:  Color{R: 255, G: 255, B: 255, Bold: false},
	Suggestion: SuggestionColors{
		Text:        Color{R: 255, G: 255, B: 255, Bold: false},
		Description: Color{R: 204, G: 204, B: 204, Bold: false},
		Match:       Color{R: 240, G: 228, B: 66, Bold: true},
		Background:  nil,
	},
	Selected:   Color{R: 230, G: 159, B: 0, Bold: true},
	Background: nil,
	Cursor:     Color{R: 255, G: 255, B: 255, Bold: false},
}

// ThemeVSCode is the VS Code dark theme colors
var ThemeVSCode = &ColorScheme{
	Name:   "VS Code",
	Prefix: Color{R: 0, G: 122, B: 204, Bold: true},
	Input:  Color{R: 255, G: 255, B: 255, Bold: false},
	Suggestion: SuggestionColors{
		Text:        Color{R: 156, G: 220, B: 254, Bold: false},
		Description: Color{R: 106, G: 153, B: 85, Bold: false},
		Match:       Color{R: 255, G: 206, B: 84, Bold: true},
		Background:  nil,
	},
	Selected:   Color{R: 0, G: 122, B: 204, Bold: true},
	Background: &Color{R: 30, G: 30, B: 30},
	Cursor:     Color{R: 255, G: 255, B: 255, Bold: true},
}

// ThemeNightOwl is the Night Owl color scheme
var ThemeNightOwl = &ColorScheme{
	Name:   "Night Owl",
	Prefix: Color{R: 130, G: 170, B: 255, Bold: true},
	Input:  Color{R: 214, G: 222, B: 235, Bold: true},
	Suggestion: SuggestionColors{
		Text:        Color{R: 197, G: 228, B: 120, Bold: false},
		Description: Color{R: 127, G: 219, B: 202, Bold: false},
		Match:       Color{R: 199, G: 146, B: 234, Bold: true},
		Background:  nil,
	},
	Selected:   Color{R: 34, G: 218, B: 110, Bold: true},
	Background: &Color{R: 1, G: 22, B: 39},
	Cursor:     Color{R: 214, G: 222, B: 235, Bold: true},
}

// ThemeDracula is the Dracula color scheme
var ThemeDracula = &ColorScheme{
	Name:   "Dracula",
	Prefix: Color{R: 255, G: 121, B: 198, Bold: true},
	Input:  Color{R: 248, G: 248, B: 242, Bold: false},
	Suggestion: SuggestionColors{
		Text:        Color{R: 139, G: 233, B: 253, Bold: false},
		Description: Color{R: 98, G: 114, B: 164, Bold: false},
		Match:       Color{R: 241, G: 250, B: 140, Bold: true},
		Background:  nil,
	},
	Selected:   Color{R: 80, G: 250, B: 123, Bold: true},
	Background: &Color{R: 40, G: 42, B: 54},
	Cursor:     Color{R: 248, G: 248, B: 242, Bold: false},
}

// ThemeMonokai is the Monokai color scheme
var ThemeMonokai = &ColorScheme{
	Name:   "Monokai",
	Prefix: Color{R: 249, G: 38, B: 114, Bold: true},
	Input:  Color{R: 248, G: 248, B: 242, Bold: false},
	Suggestion: SuggestionColors{
		Text:        Color{R: 166, G: 226, B: 46, Bold: false},
		Description: Color{R: 117, G: 113, B: 94, Bold: false},
		Match:       Color{R: 253, G: 151, B: 31, Bold: true},
		Background:  nil,
	},
	Selected:   Color{R: 102, G: 217, B: 239, Bold: true},
	Background: &Color{R: 39, G: 40, B: 34},
	Cursor:     Color{R: 248, G: 248, B: 242, Bold: false},
}

// ToANSI converts a Color to an ANSI escape sequence.
func (c Color) ToANSI() string {
	var codes []string

	// Bold formatting comes first
	if c.Bold {
		codes = append(codes, "1")
	}

	// RGB color (true color support)
	codes = append(codes, fmt.Sprintf("38;2;%d;%d;%d", c.R, c.G, c.B))

	return fmt.Sprintf("\x1b[%sm", strings.Join(codes, ";"))
}

// Reset returns the ANSI reset sequence.
func Reset() string {
	return "\x1b[0m"
}
