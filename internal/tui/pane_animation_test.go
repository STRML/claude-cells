package tui

import (
	"testing"
	"time"

	"github.com/STRML/claude-cells/internal/workstream"
)

func TestSetInitializing_IsInitializing(t *testing.T) {
	tests := []struct {
		name         string
		initializing bool
		wantInit     bool
		wantFading   bool
	}{
		{
			name:         "set initializing true",
			initializing: true,
			wantInit:     true,
			wantFading:   false,
		},
		{
			name:         "set initializing false from true starts fading",
			initializing: false, // will be set after first setting to true
			wantInit:     false,
			wantFading:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &workstream.Workstream{}
			p := NewPaneModel(ws)

			if tt.name == "set initializing false from true starts fading" {
				// First set to true, then false
				p.SetInitializing(true)
				p.SetInitializing(false)
			} else {
				p.SetInitializing(tt.initializing)
			}

			if got := p.IsInitializing(); got != tt.wantInit {
				t.Errorf("IsInitializing() = %v, want %v", got, tt.wantInit)
			}
			if got := p.IsFading(); got != tt.wantFading {
				t.Errorf("IsFading() = %v, want %v", got, tt.wantFading)
			}
		})
	}
}

func TestSetInitializing_SetsDefaultStatus(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	p.SetInitializing(true)

	if got := p.GetInitStatus(); got != "Starting..." {
		t.Errorf("GetInitStatus() = %q, want %q", got, "Starting...")
	}
	if p.initStep != 1 {
		t.Errorf("initStep = %d, want 1", p.initStep)
	}
	if p.initSteps != 3 {
		t.Errorf("initSteps = %d, want 3", p.initSteps)
	}
}

func TestIsFading_TickFade(t *testing.T) {
	tests := []struct {
		name           string
		setupFading    bool
		fadeStartTime  time.Time
		wantTickResult bool
		wantFading     bool
	}{
		{
			name:           "not fading returns false",
			setupFading:    false,
			wantTickResult: false,
			wantFading:     false,
		},
		{
			name:           "fading just started returns true",
			setupFading:    true,
			fadeStartTime:  time.Now(),
			wantTickResult: true,
			wantFading:     true,
		},
		{
			name:           "fading completed returns false",
			setupFading:    true,
			fadeStartTime:  time.Now().Add(-fadeDuration - time.Second),
			wantTickResult: false,
			wantFading:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &workstream.Workstream{}
			p := NewPaneModel(ws)

			p.fading = tt.setupFading
			if !tt.fadeStartTime.IsZero() {
				p.fadeStartTime = tt.fadeStartTime
			}

			gotTick := p.TickFade()

			if gotTick != tt.wantTickResult {
				t.Errorf("TickFade() = %v, want %v", gotTick, tt.wantTickResult)
			}
			if got := p.IsFading(); got != tt.wantFading {
				t.Errorf("IsFading() = %v, want %v", got, tt.wantFading)
			}
		})
	}
}

func TestTickFade_Progress(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	// Set fading with a known start time
	p.fading = true
	p.fadeStartTime = time.Now().Add(-fadeDuration / 2) // 50% through

	p.TickFade()

	// Progress should be around 0.5 (some tolerance for test timing)
	if p.fadeProgress < 0.4 || p.fadeProgress > 0.6 {
		t.Errorf("fadeProgress = %v, want ~0.5", p.fadeProgress)
	}
}

func TestInitTimedOut(t *testing.T) {
	tests := []struct {
		name          string
		initializing  bool
		initStartTime time.Time
		want          bool
	}{
		{
			name:          "not initializing",
			initializing:  false,
			initStartTime: time.Now().Add(-initTimeout - time.Hour),
			want:          false,
		},
		{
			name:          "zero start time",
			initializing:  true,
			initStartTime: time.Time{},
			want:          false,
		},
		{
			name:          "recently started",
			initializing:  true,
			initStartTime: time.Now(),
			want:          false,
		},
		{
			name:          "timed out",
			initializing:  true,
			initStartTime: time.Now().Add(-initTimeout - time.Second),
			want:          true,
		},
		{
			name:          "just before timeout boundary",
			initializing:  true,
			initStartTime: time.Now().Add(-initTimeout + time.Second),
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &workstream.Workstream{}
			p := NewPaneModel(ws)
			p.initializing = tt.initializing
			p.initStartTime = tt.initStartTime

			if got := p.InitTimedOut(); got != tt.want {
				t.Errorf("InitTimedOut() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInitElapsed(t *testing.T) {
	tests := []struct {
		name          string
		initStartTime time.Time
		wantZero      bool
	}{
		{
			name:          "zero start time returns zero",
			initStartTime: time.Time{},
			wantZero:      true,
		},
		{
			name:          "valid start time returns positive duration",
			initStartTime: time.Now().Add(-time.Second),
			wantZero:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &workstream.Workstream{}
			p := NewPaneModel(ws)
			p.initStartTime = tt.initStartTime

			got := p.InitElapsed()
			if tt.wantZero && got != 0 {
				t.Errorf("InitElapsed() = %v, want 0", got)
			}
			if !tt.wantZero && got <= 0 {
				t.Errorf("InitElapsed() = %v, want > 0", got)
			}
		})
	}
}

func TestGetInitStartTime(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	// Initially zero
	if got := p.GetInitStartTime(); !got.IsZero() {
		t.Errorf("GetInitStartTime() = %v, want zero", got)
	}

	// After SetInitializing, should be non-zero
	p.SetInitializing(true)
	if got := p.GetInitStartTime(); got.IsZero() {
		t.Errorf("GetInitStartTime() = zero, want non-zero")
	}
}

func TestSetInitStatus_GetInitStatus(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		wantStep    int
		initialStep int
	}{
		{
			name:        "container status sets step 1",
			status:      "Creating container...",
			wantStep:    1,
			initialStep: 0,
		},
		{
			name:        "Claude Code status sets step 2",
			status:      "Installing Claude Code...",
			wantStep:    2,
			initialStep: 0,
		},
		{
			name:        "Resuming status sets step 2",
			status:      "Resuming session...",
			wantStep:    2,
			initialStep: 0,
		},
		{
			name:        "default status with step 0 sets step 1",
			status:      "Some other message",
			wantStep:    1,
			initialStep: 0,
		},
		{
			name:        "default status preserves existing step",
			status:      "Some other message",
			wantStep:    3,
			initialStep: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &workstream.Workstream{}
			p := NewPaneModel(ws)
			p.initStep = tt.initialStep

			p.SetInitStatus(tt.status)

			if got := p.GetInitStatus(); got != tt.status {
				t.Errorf("GetInitStatus() = %q, want %q", got, tt.status)
			}
			if p.initStep != tt.wantStep {
				t.Errorf("initStep = %d, want %d", p.initStep, tt.wantStep)
			}
		})
	}
}

func TestSetSummarizing_IsSummarizing(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	// Initially false
	if got := p.IsSummarizing(); got {
		t.Errorf("IsSummarizing() = %v, want false", got)
	}

	// Set to true
	p.SetSummarizing(true)
	if got := p.IsSummarizing(); !got {
		t.Errorf("IsSummarizing() = %v, want true", got)
	}
	if p.summarizePhase != SummarizePhasePrompt {
		t.Errorf("summarizePhase = %v, want SummarizePhasePrompt", p.summarizePhase)
	}
	if p.summarizeStart.IsZero() {
		t.Error("summarizeStart should be set")
	}

	// Set to false
	p.SetSummarizing(false)
	if got := p.IsSummarizing(); got {
		t.Errorf("IsSummarizing() = %v, want false", got)
	}
}

func TestSetSummarizeTitle(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	p.SetSummarizeTitle("Test Title")

	if p.summarizeTitle != "Test Title" {
		t.Errorf("summarizeTitle = %q, want %q", p.summarizeTitle, "Test Title")
	}
}

func TestSummarizeComplete(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	p.SetSummarizing(true)
	p.SetSummarizeTitle("My Title")

	got := p.SummarizeComplete()

	if got != "My Title" {
		t.Errorf("SummarizeComplete() = %q, want %q", got, "My Title")
	}
	if p.IsSummarizing() {
		t.Error("IsSummarizing() should be false after SummarizeComplete()")
	}
	if p.summarizePhase != SummarizePhaseDone {
		t.Errorf("summarizePhase = %v, want SummarizePhaseDone", p.summarizePhase)
	}
}

func TestStartSummarizeFade_IsSummarizeFading(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	// Initially not fading
	if got := p.IsSummarizeFading(); got {
		t.Errorf("IsSummarizeFading() = %v, want false", got)
	}

	p.StartSummarizeFade()

	if got := p.IsSummarizeFading(); !got {
		t.Errorf("IsSummarizeFading() = %v, want true", got)
	}
	if p.summarizePhase != SummarizePhaseFading {
		t.Errorf("summarizePhase = %v, want SummarizePhaseFading", p.summarizePhase)
	}
	if p.summarizeFadeEndAt.IsZero() {
		t.Error("summarizeFadeEndAt should be set")
	}
}

func TestSummarizeFadeProgress(t *testing.T) {
	tests := []struct {
		name                  string
		phase                 SummarizePhase
		summarizeStart        time.Time
		summarizeFadeEndAt    time.Time
		wantProgressApprox    float64
		wantProgressTolerance float64
	}{
		{
			name:                  "not in fading phase returns 0",
			phase:                 SummarizePhasePrompt,
			wantProgressApprox:    0,
			wantProgressTolerance: 0.01,
		},
		{
			name:                  "at start returns 0",
			phase:                 SummarizePhaseFading,
			summarizeStart:        time.Now(),
			summarizeFadeEndAt:    time.Now().Add(4 * time.Second),
			wantProgressApprox:    0,
			wantProgressTolerance: 0.1,
		},
		{
			name:                  "halfway through returns 0.5",
			phase:                 SummarizePhaseFading,
			summarizeStart:        time.Now().Add(-2 * time.Second),
			summarizeFadeEndAt:    time.Now().Add(2 * time.Second),
			wantProgressApprox:    0.5,
			wantProgressTolerance: 0.1,
		},
		{
			name:                  "past end returns 1.0",
			phase:                 SummarizePhaseFading,
			summarizeStart:        time.Now().Add(-5 * time.Second),
			summarizeFadeEndAt:    time.Now().Add(-1 * time.Second),
			wantProgressApprox:    1.0,
			wantProgressTolerance: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &workstream.Workstream{}
			p := NewPaneModel(ws)
			p.summarizePhase = tt.phase
			p.summarizeStart = tt.summarizeStart
			p.summarizeFadeEndAt = tt.summarizeFadeEndAt

			got := p.SummarizeFadeProgress()

			diff := got - tt.wantProgressApprox
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.wantProgressTolerance {
				t.Errorf("SummarizeFadeProgress() = %v, want ~%v (tolerance %v)", got, tt.wantProgressApprox, tt.wantProgressTolerance)
			}
		})
	}
}

func TestShouldFinishFade(t *testing.T) {
	tests := []struct {
		name               string
		phase              SummarizePhase
		summarizeFadeEndAt time.Time
		want               bool
	}{
		{
			name:               "not in fading phase",
			phase:              SummarizePhasePrompt,
			summarizeFadeEndAt: time.Now().Add(-time.Hour),
			want:               false,
		},
		{
			name:               "fading but not finished",
			phase:              SummarizePhaseFading,
			summarizeFadeEndAt: time.Now().Add(time.Hour),
			want:               false,
		},
		{
			name:               "fading and finished",
			phase:              SummarizePhaseFading,
			summarizeFadeEndAt: time.Now().Add(-time.Second),
			want:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &workstream.Workstream{}
			p := NewPaneModel(ws)
			p.summarizePhase = tt.phase
			p.summarizeFadeEndAt = tt.summarizeFadeEndAt

			if got := p.ShouldFinishFade(); got != tt.want {
				t.Errorf("ShouldFinishFade() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTickSpinner(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	// Initial frame should be 0
	if p.spinnerFrame != 0 {
		t.Errorf("initial spinnerFrame = %d, want 0", p.spinnerFrame)
	}

	// Tick through all frames
	for i := 1; i <= 10; i++ {
		p.TickSpinner()
		expected := i % 4
		if p.spinnerFrame != expected {
			t.Errorf("after %d ticks, spinnerFrame = %d, want %d", i, p.spinnerFrame, expected)
		}
	}
}

func TestTickSpinner_ModuloWrap(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	// Set to frame 3
	p.spinnerFrame = 3
	p.TickSpinner()

	// Should wrap to 0
	if p.spinnerFrame != 0 {
		t.Errorf("spinnerFrame = %d, want 0 (should wrap)", p.spinnerFrame)
	}
}
