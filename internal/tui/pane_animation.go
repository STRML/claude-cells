package tui

import (
	"strings"
	"time"
)

// SummarizePhase represents the animation phase during title generation
type SummarizePhase int

const (
	SummarizePhasePrompt SummarizePhase = iota // Showing prompt with spinner
	SummarizePhaseReveal                       // Title revealed, brief highlight
	SummarizePhaseFading                       // Title fading out over initialization
	SummarizePhaseDone                         // Animation complete
)

// fadeDuration is how long the fade animation takes
const fadeDuration = 400 * time.Millisecond

// SetInitializing sets the initializing state with a status message
func (p *PaneModel) SetInitializing(initializing bool) {
	wasInitializing := p.initializing
	p.initializing = initializing
	if initializing {
		p.initStartTime = time.Now()
		p.initSteps = 3
		if p.initStatus == "" {
			p.initStatus = "Starting..."
			p.initStep = 1
		}
		// Reset fade state when starting initialization
		p.fading = false
		p.fadeProgress = 0
	} else if wasInitializing {
		// Transition from initializing to ready - start fade animation
		p.fading = true
		p.fadeStartTime = time.Now()
		p.fadeProgress = 0
	}
}

// IsFading returns true if the pane is in fade animation
func (p *PaneModel) IsFading() bool {
	return p.fading
}

// TickFade advances the fade animation, returns true if still fading
func (p *PaneModel) TickFade() bool {
	if !p.fading {
		return false
	}
	elapsed := time.Since(p.fadeStartTime)
	p.fadeProgress = float64(elapsed) / float64(fadeDuration)
	if p.fadeProgress >= 1.0 {
		p.fadeProgress = 1.0
		p.fading = false
		return false
	}
	return true
}

// InitTimedOut returns true if initialization has taken longer than initTimeout
func (p *PaneModel) InitTimedOut() bool {
	if !p.initializing || p.initStartTime.IsZero() {
		return false
	}
	return time.Since(p.initStartTime) > initTimeout
}

// InitElapsed returns how long initialization has been running
func (p *PaneModel) InitElapsed() time.Duration {
	if p.initStartTime.IsZero() {
		return 0
	}
	return time.Since(p.initStartTime)
}

// GetInitStatus returns the current initialization status message
func (p *PaneModel) GetInitStatus() string {
	return p.initStatus
}

// GetInitStartTime returns when initialization started
func (p *PaneModel) GetInitStartTime() time.Time {
	return p.initStartTime
}

// SetInitStatus sets the initialization status message and step
func (p *PaneModel) SetInitStatus(status string) {
	p.initStatus = status
	// Auto-advance step based on status
	switch {
	case strings.Contains(status, "container"):
		p.initStep = 1
	case strings.Contains(status, "Claude Code"):
		p.initStep = 2
	case strings.Contains(status, "Resuming"):
		p.initStep = 2
	default:
		if p.initStep == 0 {
			p.initStep = 1
		}
	}
}

// IsInitializing returns true if the pane is still initializing
func (p *PaneModel) IsInitializing() bool {
	return p.initializing
}

// IsSummarizing returns true if the pane is generating a title
func (p *PaneModel) IsSummarizing() bool {
	return p.summarizing
}

// SetSummarizing starts the summarizing animation
func (p *PaneModel) SetSummarizing(summarizing bool) {
	p.summarizing = summarizing
	if summarizing {
		p.summarizeStart = time.Now()
		p.summarizePhase = SummarizePhasePrompt
		p.summarizeTitle = ""
	}
}

// SetSummarizeTitle sets the generated title
func (p *PaneModel) SetSummarizeTitle(title string) {
	p.summarizeTitle = title
}

// StartSummarizeFade starts the fading animation (called when container starts)
func (p *PaneModel) StartSummarizeFade() {
	p.summarizePhase = SummarizePhaseFading
	p.summarizeStart = time.Now()                          // Reset start time for fade progress
	p.summarizeFadeEndAt = time.Now().Add(4 * time.Second) // Fade out over 4 seconds
}

// SummarizeComplete marks summarization as done and returns the title
func (p *PaneModel) SummarizeComplete() string {
	p.summarizing = false
	p.summarizePhase = SummarizePhaseDone
	return p.summarizeTitle
}

// IsSummarizeFading returns true if in the fading phase
func (p *PaneModel) IsSummarizeFading() bool {
	return p.summarizePhase == SummarizePhaseFading
}

// SummarizeFadeProgress returns 0.0-1.0 progress through the fade (1.0 = fully faded)
func (p *PaneModel) SummarizeFadeProgress() float64 {
	if p.summarizePhase != SummarizePhaseFading {
		return 0
	}
	total := p.summarizeFadeEndAt.Sub(p.summarizeStart)
	elapsed := time.Since(p.summarizeStart)
	if elapsed >= total {
		return 1.0
	}
	return float64(elapsed) / float64(total)
}

// ShouldFinishFade returns true if the fading phase is complete
func (p *PaneModel) ShouldFinishFade() bool {
	return p.summarizePhase == SummarizePhaseFading && time.Now().After(p.summarizeFadeEndAt)
}

// TickSpinner advances the spinner animation
func (p *PaneModel) TickSpinner() {
	p.spinnerFrame = (p.spinnerFrame + 1) % 4
}
