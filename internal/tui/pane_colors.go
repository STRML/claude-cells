package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ansiRegex matches ANSI CSI (Control Sequence Introducer) escape sequences.
// Pattern matches: ESC [ <params> <intermediates> <final>
// - params: 0x30-0x3F (digits, semicolon, and private markers like ?)
// - intermediates: 0x20-0x2F (space through /)
// - final: 0x40-0x7E (@ through ~)
// This handles standard sequences like colors (\x1b[31m) and private sequences
// like cursor visibility (\x1b[?25l, \x1b[?25h).
var ansiRegex = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

// clampByte clamps an integer to the valid byte range [0, 255]
func clampByte(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// stripANSI removes all ANSI escape sequences from a string
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// ansiSGRRegex matches SGR (Select Graphic Rendition) sequences specifically
var ansiSGRRegex = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// basic16Colors maps basic ANSI color codes to RGB values
// Indexes 0-7 are normal colors, 8-15 are bright colors
var basic16Colors = []struct{ r, g, b int }{
	{0, 0, 0},       // 0: Black
	{205, 49, 49},   // 1: Red
	{13, 188, 121},  // 2: Green
	{229, 229, 16},  // 3: Yellow
	{36, 114, 200},  // 4: Blue
	{188, 63, 188},  // 5: Magenta
	{17, 168, 205},  // 6: Cyan
	{229, 229, 229}, // 7: White
	{102, 102, 102}, // 8: Bright Black (Gray)
	{241, 76, 76},   // 9: Bright Red
	{35, 209, 139},  // 10: Bright Green
	{245, 245, 67},  // 11: Bright Yellow
	{59, 142, 234},  // 12: Bright Blue
	{214, 112, 214}, // 13: Bright Magenta
	{41, 184, 219},  // 14: Bright Cyan
	{255, 255, 255}, // 15: Bright White
}

// muteANSI transforms colors in ANSI sequences to be muted (desaturated)
// saturation: 0.0 = grayscale, 1.0 = original
// brightness: multiplier for lightness
// mutedDefault: RGB values for the muted default foreground color (used when terminal
// would normally show its default color, e.g., after reset or with code 39)
func muteANSI(s string, saturation, brightness float64, mutedDefault [3]int) string {
	return ansiSGRRegex.ReplaceAllStringFunc(s, func(match string) string {
		// Extract the parameters between \x1b[ and m
		submatch := ansiSGRRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		params := submatch[1]
		if params == "" {
			// Reset sequence (\x1b[m) - add muted default foreground after reset
			return fmt.Sprintf("\x1b[0;38;2;%d;%d;%dm", mutedDefault[0], mutedDefault[1], mutedDefault[2])
		}

		// Parse parameters
		parts := strings.Split(params, ";")
		result := make([]string, 0, len(parts))

		for i := 0; i < len(parts); i++ {
			code, err := strconv.Atoi(parts[i])
			if err != nil {
				result = append(result, parts[i])
				continue
			}

			// Handle extended color sequences: 38;5;N or 38;2;R;G;B (foreground)
			// and 48;5;N or 48;2;R;G;B (background)
			if (code == 38 || code == 48) && i+1 < len(parts) {
				colorType, err := strconv.Atoi(parts[i+1])
				if err != nil {
					// Malformed color type, keep original code
					result = append(result, parts[i])
					continue
				}
				if colorType == 5 && i+2 < len(parts) {
					// 256-color mode: 38;5;N or 48;5;N
					colorIndex, err := strconv.Atoi(parts[i+2])
					if err != nil || colorIndex < 0 || colorIndex > 255 {
						// Malformed or out-of-range color index, keep original sequence
						result = append(result, parts[i])
						continue
					}
					r, g, b := color256ToRGB(colorIndex)
					mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
					result = append(result, fmt.Sprintf("%d;2;%d;%d;%d", code, mr, mg, mb))
					i += 2
					continue
				} else if colorType == 2 && i+4 < len(parts) {
					// True color mode: 38;2;R;G;B or 48;2;R;G;B
					r, errR := strconv.Atoi(parts[i+2])
					g, errG := strconv.Atoi(parts[i+3])
					b, errB := strconv.Atoi(parts[i+4])
					if errR != nil || errG != nil || errB != nil {
						// Malformed RGB values, keep original code
						result = append(result, parts[i])
						continue
					}
					// Clamp RGB values to valid range
					r = clampByte(r)
					g = clampByte(g)
					b = clampByte(b)
					mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
					result = append(result, fmt.Sprintf("%d;2;%d;%d;%d", code, mr, mg, mb))
					i += 4
					continue
				}
			}

			// Handle basic foreground colors (30-37, 90-97)
			if code >= 30 && code <= 37 {
				colorIndex := code - 30
				r, g, b := basic16Colors[colorIndex].r, basic16Colors[colorIndex].g, basic16Colors[colorIndex].b
				mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
				result = append(result, fmt.Sprintf("38;2;%d;%d;%d", mr, mg, mb))
				continue
			}
			if code >= 90 && code <= 97 {
				colorIndex := code - 90 + 8 // Bright colors are 8-15
				r, g, b := basic16Colors[colorIndex].r, basic16Colors[colorIndex].g, basic16Colors[colorIndex].b
				mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
				result = append(result, fmt.Sprintf("38;2;%d;%d;%d", mr, mg, mb))
				continue
			}

			// Handle basic background colors (40-47, 100-107)
			if code >= 40 && code <= 47 {
				colorIndex := code - 40
				r, g, b := basic16Colors[colorIndex].r, basic16Colors[colorIndex].g, basic16Colors[colorIndex].b
				mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
				result = append(result, fmt.Sprintf("48;2;%d;%d;%d", mr, mg, mb))
				continue
			}
			if code >= 100 && code <= 107 {
				colorIndex := code - 100 + 8
				r, g, b := basic16Colors[colorIndex].r, basic16Colors[colorIndex].g, basic16Colors[colorIndex].b
				mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
				result = append(result, fmt.Sprintf("48;2;%d;%d;%d", mr, mg, mb))
				continue
			}

			// Handle reset (code 0) - preserve reset but add muted default foreground
			if code == 0 {
				result = append(result, fmt.Sprintf("0;38;2;%d;%d;%d", mutedDefault[0], mutedDefault[1], mutedDefault[2]))
				continue
			}

			// Handle default foreground (code 39) - replace with muted default
			if code == 39 {
				result = append(result, fmt.Sprintf("38;2;%d;%d;%d", mutedDefault[0], mutedDefault[1], mutedDefault[2]))
				continue
			}

			// Keep other codes as-is (bold, underline, default background, etc.)
			result = append(result, parts[i])
		}

		return "\x1b[" + strings.Join(result, ";") + "m"
	})
}

// color256ToRGB converts a 256-color palette index to RGB.
// Index must be in range [0, 255]. Out-of-range values are clamped.
func color256ToRGB(index int) (r, g, b int) {
	// Clamp to valid range for safety
	if index < 0 {
		index = 0
	} else if index > 255 {
		index = 255
	}

	if index < 16 {
		// Standard colors (same as basic16Colors)
		return basic16Colors[index].r, basic16Colors[index].g, basic16Colors[index].b
	} else if index < 232 {
		// 216-color cube (6x6x6)
		index -= 16
		r = (index / 36) * 51
		g = ((index / 6) % 6) * 51
		b = (index % 6) * 51
		return r, g, b
	} else {
		// Grayscale (24 shades)
		gray := (index-232)*10 + 8
		return gray, gray, gray
	}
}
