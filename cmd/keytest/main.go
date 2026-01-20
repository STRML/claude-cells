// keytest is a simple program to test if your terminal supports
// the Kitty keyboard protocol (needed for Shift+Enter to work).
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

type model struct {
	lastKey    string
	enhanced   bool
	keyHistory []string
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
	}
	return m, nil
}

func (m model) View() tea.View {
	status := "❌ NOT detected"
	if m.enhanced {
		status = "✅ ENABLED"
	}

	content := fmt.Sprintf(`Keyboard Protocol Test
======================

Kitty keyboard protocol: %s

Press keys to test. Press 'q' to quit.

If Shift+Enter works, you should see:
  Key: "shift+enter" (Code=13, Mod=1)

If it shows just "enter" with Mod=0, your terminal
doesn't support the Kitty keyboard protocol.

Recent keys:
`, status)

	for _, k := range m.keyHistory {
		content += "  " + k + "\n"
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
