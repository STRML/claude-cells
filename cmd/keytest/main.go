// keytest is a simple program to test if your terminal supports
// the Kitty keyboard protocol (needed for Shift+Enter to work).
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

type model struct {
	lastKey      string
	enhanced     bool
	keyHistory   []string
	pasteHistory []string
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyboardEnhancementsMsg:
		m.enhanced = true
		m.keyHistory = append(m.keyHistory, "[Keyboard enhancements ENABLED]")
		return m, nil

	case tea.KeyPressMsg:
		keyStr := msg.String()
		m.lastKey = keyStr
		m.keyHistory = append(m.keyHistory, fmt.Sprintf("Key: %q (Code=%d, Mod=%d)", keyStr, msg.Code, msg.Mod))

		// Keep only last 10 keys
		if len(m.keyHistory) > 12 {
			m.keyHistory = m.keyHistory[len(m.keyHistory)-12:]
		}

		if keyStr == "q" || keyStr == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil

	case tea.WindowSizeMsg:
		return m, nil

	case tea.PasteMsg:
		// Truncate for display
		content := msg.Content
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		m.pasteHistory = append(m.pasteHistory, fmt.Sprintf("Paste: %q (%d bytes)", content, len(msg.Content)))
		if len(m.pasteHistory) > 5 {
			m.pasteHistory = m.pasteHistory[len(m.pasteHistory)-5:]
		}
		return m, nil

	case tea.PasteStartMsg:
		m.pasteHistory = append(m.pasteHistory, "[Paste START]")
		return m, nil

	case tea.PasteEndMsg:
		m.pasteHistory = append(m.pasteHistory, "[Paste END]")
		return m, nil
	}
	return m, nil
}

func (m model) View() tea.View {
	status := "❌ NOT detected"
	if m.enhanced {
		status = "✅ ENABLED"
	}

	content := fmt.Sprintf(`Keyboard & Paste Protocol Test
===============================

Kitty keyboard protocol: %s

Press keys to test. Press 'q' to quit.

Test Shift+Enter: Should show Key: "shift+enter" (Code=13, Mod=1)
Test Paste: Use Ctrl+Shift+V (Linux) or Cmd+V (macOS)

If Ctrl+V shows as a key press, your terminal is NOT
sending bracketed paste - it's sending Ctrl+V as a key.
Use Ctrl+Shift+V instead for paste on Linux terminals.

Recent keys:
`, status)

	for _, k := range m.keyHistory {
		content += "  " + k + "\n"
	}

	content += "\nRecent paste events:\n"
	if len(m.pasteHistory) == 0 {
		content += "  (none yet - try pasting with Ctrl+Shift+V)\n"
	}
	for _, p := range m.pasteHistory {
		content += "  " + p + "\n"
	}

	if !m.enhanced {
		content += `
⚠️  Keyboard enhancements NOT detected!

For iTerm2, enable in:
  Settings → Profiles → Keys → General
  ☑ "Apps can change how keys are reported"
`
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func main() {
	p := tea.NewProgram(model{})
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
