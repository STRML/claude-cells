package tui

// ScrollPageUp scrolls the viewport up by one page
func (p *PaneModel) ScrollPageUp() {
	p.scrollMode = true
	p.viewport.PageUp()
}

// ScrollPageDown scrolls the viewport down by one page
func (p *PaneModel) ScrollPageDown() {
	p.viewport.PageDown()
	// Exit scroll mode if at bottom
	if p.viewport.AtBottom() {
		p.scrollMode = false
	}
}

// ScrollLineUp scrolls the viewport up by one line
func (p *PaneModel) ScrollLineUp() {
	p.scrollMode = true
	p.viewport.ScrollUp(1)
}

// ScrollLineDown scrolls the viewport down by one line
func (p *PaneModel) ScrollLineDown() {
	p.viewport.ScrollDown(1)
	// Exit scroll mode if at bottom
	if p.viewport.AtBottom() {
		p.scrollMode = false
	}
}

// ScrollHalfPageUp scrolls the viewport up by half a page (vim-style ctrl+u)
func (p *PaneModel) ScrollHalfPageUp() {
	p.scrollMode = true
	halfPage := p.viewport.Height() / 2
	if halfPage < 1 {
		halfPage = 1
	}
	p.viewport.ScrollUp(halfPage)
}

// ScrollHalfPageDown scrolls the viewport down by half a page (vim-style ctrl+d)
func (p *PaneModel) ScrollHalfPageDown() {
	halfPage := p.viewport.Height() / 2
	if halfPage < 1 {
		halfPage = 1
	}
	p.viewport.ScrollDown(halfPage)
	// Exit scroll mode if at bottom
	if p.viewport.AtBottom() {
		p.scrollMode = false
	}
}

// EnterScrollMode enters scroll mode without changing position
func (p *PaneModel) EnterScrollMode() {
	p.scrollMode = true
}

// ScrollToBottom scrolls to the bottom and exits scroll mode
func (p *PaneModel) ScrollToBottom() {
	p.viewport.GotoBottom()
	p.scrollMode = false
}

// ClearScrollback clears the scrollback buffer.
// This is called when Claude Code becomes ready to give the user a fresh pane
// without setup messages visible when scrolling up.
func (p *PaneModel) ClearScrollback() {
	p.scrollback = nil
}

// IsScrollMode returns true if the pane is in scroll mode (not following live output)
func (p *PaneModel) IsScrollMode() bool {
	return p.scrollMode
}
