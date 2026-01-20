package tui

import (
	"testing"
)

func TestLayoutType_String(t *testing.T) {
	tests := []struct {
		layout   LayoutType
		expected string
	}{
		{LayoutGrid, "Grid"},
		{LayoutMainLeft, "Main+Stack"},
		{LayoutMainTop, "Main+Row"},
		{LayoutRows, "Rows"},
		{LayoutColumns, "Columns"},
		{LayoutType(99), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.layout.String(); got != tt.expected {
			t.Errorf("LayoutType(%d).String() = %q, want %q", tt.layout, got, tt.expected)
		}
	}
}

func TestLayoutType_Next(t *testing.T) {
	tests := []struct {
		layout   LayoutType
		expected LayoutType
	}{
		{LayoutGrid, LayoutMainLeft},
		{LayoutMainLeft, LayoutMainTop},
		{LayoutMainTop, LayoutRows},
		{LayoutRows, LayoutColumns},
		{LayoutColumns, LayoutGrid}, // wraps around
	}

	for _, tt := range tests {
		if got := tt.layout.Next(); got != tt.expected {
			t.Errorf("LayoutType(%d).Next() = %d, want %d", tt.layout, got, tt.expected)
		}
	}
}

func TestCalculateLayout_SinglePane(t *testing.T) {
	layouts := []LayoutType{LayoutGrid, LayoutMainLeft, LayoutMainTop, LayoutRows, LayoutColumns}

	for _, layout := range layouts {
		sizes := CalculateLayout(layout, 1, 100, 50)
		if len(sizes) != 1 {
			t.Errorf("CalculateLayout(%v, 1, ...) returned %d sizes, want 1", layout, len(sizes))
			continue
		}
		if sizes[0].Width != 100 || sizes[0].Height != 50 {
			t.Errorf("CalculateLayout(%v, 1, 100, 50) = {%d, %d}, want {100, 50}",
				layout, sizes[0].Width, sizes[0].Height)
		}
	}
}

func TestCalculateLayout_EmptyPanes(t *testing.T) {
	layouts := []LayoutType{LayoutGrid, LayoutMainLeft, LayoutMainTop, LayoutRows, LayoutColumns}

	for _, layout := range layouts {
		sizes := CalculateLayout(layout, 0, 100, 50)
		if sizes != nil {
			t.Errorf("CalculateLayout(%v, 0, ...) returned %v, want nil", layout, sizes)
		}
	}
}

func TestCalculateGridLayout(t *testing.T) {
	tests := []struct {
		name      string
		paneCount int
		width     int
		height    int
		wantCols  int
		wantRows  int
	}{
		{"1 pane", 1, 100, 50, 1, 1},
		{"2 panes", 2, 100, 50, 2, 1},
		{"3 panes", 3, 100, 50, 2, 2},
		{"4 panes", 4, 100, 50, 2, 2},
		{"5 panes", 5, 120, 60, 3, 2},
		{"6 panes", 6, 120, 60, 3, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sizes := CalculateLayout(LayoutGrid, tt.paneCount, tt.width, tt.height)
			if len(sizes) != tt.paneCount {
				t.Errorf("got %d sizes, want %d", len(sizes), tt.paneCount)
			}

			// Verify total width and height usage (sizes should fill the space)
			// The grid layout should use the full width for each row
			// and full height distributed across rows
		})
	}
}

func TestCalculateMainLeftLayout(t *testing.T) {
	sizes := CalculateLayout(LayoutMainLeft, 3, 100, 50)

	if len(sizes) != 3 {
		t.Fatalf("got %d sizes, want 3", len(sizes))
	}

	// Main pane should be 60% of width
	expectedMainWidth := 60
	if sizes[0].Width != expectedMainWidth {
		t.Errorf("main pane width = %d, want %d", sizes[0].Width, expectedMainWidth)
	}

	// Main pane should have full height
	if sizes[0].Height != 50 {
		t.Errorf("main pane height = %d, want 50", sizes[0].Height)
	}

	// Side panes should share the remaining width
	expectedSideWidth := 40
	if sizes[1].Width != expectedSideWidth || sizes[2].Width != expectedSideWidth {
		t.Errorf("side pane widths = %d, %d; want %d", sizes[1].Width, sizes[2].Width, expectedSideWidth)
	}
}

func TestCalculateMainTopLayout(t *testing.T) {
	sizes := CalculateLayout(LayoutMainTop, 3, 100, 50)

	if len(sizes) != 3 {
		t.Fatalf("got %d sizes, want 3", len(sizes))
	}

	// Main pane should have full width
	if sizes[0].Width != 100 {
		t.Errorf("main pane width = %d, want 100", sizes[0].Width)
	}

	// Main pane should be 60% of height
	expectedMainHeight := 30
	if sizes[0].Height != expectedMainHeight {
		t.Errorf("main pane height = %d, want %d", sizes[0].Height, expectedMainHeight)
	}

	// Bottom panes should share the width
	expectedBottomHeight := 20
	if sizes[1].Height != expectedBottomHeight || sizes[2].Height != expectedBottomHeight {
		t.Errorf("bottom pane heights = %d, %d; want %d", sizes[1].Height, sizes[2].Height, expectedBottomHeight)
	}
}

func TestCalculateRowsLayout(t *testing.T) {
	sizes := CalculateLayout(LayoutRows, 3, 100, 60)

	if len(sizes) != 3 {
		t.Fatalf("got %d sizes, want 3", len(sizes))
	}

	// All panes should have full width
	for i, size := range sizes {
		if size.Width != 100 {
			t.Errorf("pane %d width = %d, want 100", i, size.Width)
		}
	}

	// Heights should be roughly equal (60/3 = 20 each)
	if sizes[0].Height != 20 || sizes[1].Height != 20 || sizes[2].Height != 20 {
		t.Errorf("pane heights = %d, %d, %d; want 20 each",
			sizes[0].Height, sizes[1].Height, sizes[2].Height)
	}
}

func TestCalculateColumnsLayout(t *testing.T) {
	sizes := CalculateLayout(LayoutColumns, 3, 90, 50)

	if len(sizes) != 3 {
		t.Fatalf("got %d sizes, want 3", len(sizes))
	}

	// All panes should have full height
	for i, size := range sizes {
		if size.Height != 50 {
			t.Errorf("pane %d height = %d, want 50", i, size.Height)
		}
	}

	// Widths should be roughly equal (90/3 = 30 each)
	if sizes[0].Width != 30 || sizes[1].Width != 30 || sizes[2].Width != 30 {
		t.Errorf("pane widths = %d, %d, %d; want 30 each",
			sizes[0].Width, sizes[1].Width, sizes[2].Width)
	}
}

func TestCalculateGridDimensions(t *testing.T) {
	tests := []struct {
		n        int
		wantCols int
		wantRows int
	}{
		{1, 1, 1},
		{2, 2, 1},
		{3, 2, 2},
		{4, 2, 2},
		{5, 3, 2},
		{6, 3, 2},
		{7, 3, 3},
		{8, 3, 3},
		{9, 3, 3},
		{10, 4, 3},
		{12, 4, 3},
	}

	for _, tt := range tests {
		cols, rows := calculateGridDimensions(tt.n)
		if cols != tt.wantCols || rows != tt.wantRows {
			t.Errorf("calculateGridDimensions(%d) = %d, %d; want %d, %d",
				tt.n, cols, rows, tt.wantCols, tt.wantRows)
		}
	}
}

// ============================================================================
// CalculatePaneBounds Tests - Critical for click-to-focus functionality
// ============================================================================

func TestCalculatePaneBounds_EmptyPanes(t *testing.T) {
	bounds := CalculatePaneBounds(LayoutGrid, 0, 100, 50, 1)
	if bounds != nil {
		t.Errorf("CalculatePaneBounds with 0 panes should return nil, got %v", bounds)
	}
}

func TestCalculatePaneBounds_SinglePane(t *testing.T) {
	layouts := []LayoutType{LayoutGrid, LayoutMainLeft, LayoutMainTop, LayoutRows, LayoutColumns}

	for _, layout := range layouts {
		t.Run(layout.String(), func(t *testing.T) {
			bounds := CalculatePaneBounds(layout, 1, 100, 50, 1)

			if len(bounds) != 1 {
				t.Fatalf("Expected 1 bound, got %d", len(bounds))
			}

			// Single pane should fill the available space
			b := bounds[0]
			if b.X != 0 {
				t.Errorf("X should be 0, got %d", b.X)
			}
			if b.Y != 1 { // startY is 1 (title bar height)
				t.Errorf("Y should be 1 (startY), got %d", b.Y)
			}
			if b.Width != 100 {
				t.Errorf("Width should be 100, got %d", b.Width)
			}
			if b.Height != 50 {
				t.Errorf("Height should be 50, got %d", b.Height)
			}
		})
	}
}

func TestCalculatePaneBounds_Grid_TwoPanes(t *testing.T) {
	// Two panes in grid: side by side (2 cols, 1 row)
	bounds := CalculatePaneBounds(LayoutGrid, 2, 100, 50, 1)

	if len(bounds) != 2 {
		t.Fatalf("Expected 2 bounds, got %d", len(bounds))
	}

	// First pane: left half
	if bounds[0].X != 0 || bounds[0].Y != 1 {
		t.Errorf("Pane 0 position: expected (0,1), got (%d,%d)", bounds[0].X, bounds[0].Y)
	}
	if bounds[0].Width != 50 {
		t.Errorf("Pane 0 width: expected 50, got %d", bounds[0].Width)
	}

	// Second pane: right half
	if bounds[1].X != 50 || bounds[1].Y != 1 {
		t.Errorf("Pane 1 position: expected (50,1), got (%d,%d)", bounds[1].X, bounds[1].Y)
	}
	if bounds[1].Width != 50 {
		t.Errorf("Pane 1 width: expected 50, got %d", bounds[1].Width)
	}
}

func TestCalculatePaneBounds_Grid_ThreePanes(t *testing.T) {
	// Three panes in grid: 2 cols, 2 rows
	// Row 0: pane 0, pane 1
	// Row 1: pane 2 (wider, spans unused space)
	bounds := CalculatePaneBounds(LayoutGrid, 3, 100, 40, 0)

	if len(bounds) != 3 {
		t.Fatalf("Expected 3 bounds, got %d", len(bounds))
	}

	// First row: panes 0 and 1
	if bounds[0].X != 0 || bounds[0].Y != 0 {
		t.Errorf("Pane 0 position: expected (0,0), got (%d,%d)", bounds[0].X, bounds[0].Y)
	}
	if bounds[1].X != 50 || bounds[1].Y != 0 {
		t.Errorf("Pane 1 position: expected (50,0), got (%d,%d)", bounds[1].X, bounds[1].Y)
	}

	// Second row: pane 2 at start of row
	if bounds[2].X != 0 {
		t.Errorf("Pane 2 X: expected 0, got %d", bounds[2].X)
	}
	// Y should be first row height
	if bounds[2].Y != bounds[0].Height {
		t.Errorf("Pane 2 Y: expected %d, got %d", bounds[0].Height, bounds[2].Y)
	}
}

func TestCalculatePaneBounds_Grid_FourPanes(t *testing.T) {
	// Four panes: 2x2 grid
	bounds := CalculatePaneBounds(LayoutGrid, 4, 100, 40, 0)

	if len(bounds) != 4 {
		t.Fatalf("Expected 4 bounds, got %d", len(bounds))
	}

	// Row 0
	if bounds[0].X != 0 || bounds[0].Y != 0 {
		t.Errorf("Pane 0: expected (0,0), got (%d,%d)", bounds[0].X, bounds[0].Y)
	}
	if bounds[1].X != 50 || bounds[1].Y != 0 {
		t.Errorf("Pane 1: expected (50,0), got (%d,%d)", bounds[1].X, bounds[1].Y)
	}

	// Row 1
	if bounds[2].X != 0 || bounds[2].Y != 20 {
		t.Errorf("Pane 2: expected (0,20), got (%d,%d)", bounds[2].X, bounds[2].Y)
	}
	if bounds[3].X != 50 || bounds[3].Y != 20 {
		t.Errorf("Pane 3: expected (50,20), got (%d,%d)", bounds[3].X, bounds[3].Y)
	}
}

func TestCalculatePaneBounds_MainLeft(t *testing.T) {
	// MainLeft: main pane on left (60%), side panes stacked on right (40%)
	bounds := CalculatePaneBounds(LayoutMainLeft, 3, 100, 60, 0)

	if len(bounds) != 3 {
		t.Fatalf("Expected 3 bounds, got %d", len(bounds))
	}

	// Main pane: full height, 60% width
	if bounds[0].X != 0 || bounds[0].Y != 0 {
		t.Errorf("Main pane position: expected (0,0), got (%d,%d)", bounds[0].X, bounds[0].Y)
	}
	if bounds[0].Width != 60 {
		t.Errorf("Main pane width: expected 60, got %d", bounds[0].Width)
	}
	if bounds[0].Height != 60 {
		t.Errorf("Main pane height: expected 60, got %d", bounds[0].Height)
	}

	// Side panes: stacked vertically on right
	if bounds[1].X != 60 || bounds[1].Y != 0 {
		t.Errorf("Side pane 1 position: expected (60,0), got (%d,%d)", bounds[1].X, bounds[1].Y)
	}
	if bounds[2].X != 60 {
		t.Errorf("Side pane 2 X: expected 60, got %d", bounds[2].X)
	}
	if bounds[2].Y <= bounds[1].Y {
		t.Errorf("Side pane 2 should be below side pane 1")
	}
}

func TestCalculatePaneBounds_MainTop(t *testing.T) {
	// MainTop: main pane on top (60% height), bottom panes in a row (40% height)
	bounds := CalculatePaneBounds(LayoutMainTop, 3, 100, 50, 0)

	if len(bounds) != 3 {
		t.Fatalf("Expected 3 bounds, got %d", len(bounds))
	}

	// Main pane: full width, 60% height
	if bounds[0].X != 0 || bounds[0].Y != 0 {
		t.Errorf("Main pane position: expected (0,0), got (%d,%d)", bounds[0].X, bounds[0].Y)
	}
	if bounds[0].Width != 100 {
		t.Errorf("Main pane width: expected 100, got %d", bounds[0].Width)
	}
	if bounds[0].Height != 30 { // 60% of 50
		t.Errorf("Main pane height: expected 30, got %d", bounds[0].Height)
	}

	// Bottom panes: side by side below main
	if bounds[1].X != 0 || bounds[1].Y != 30 {
		t.Errorf("Bottom pane 1 position: expected (0,30), got (%d,%d)", bounds[1].X, bounds[1].Y)
	}
	if bounds[2].Y != 30 {
		t.Errorf("Bottom pane 2 Y: expected 30, got %d", bounds[2].Y)
	}
	if bounds[2].X <= bounds[1].X {
		t.Errorf("Bottom pane 2 should be to the right of bottom pane 1")
	}
}

func TestCalculatePaneBounds_Rows(t *testing.T) {
	// Rows: full-width panes stacked vertically
	bounds := CalculatePaneBounds(LayoutRows, 3, 100, 60, 0)

	if len(bounds) != 3 {
		t.Fatalf("Expected 3 bounds, got %d", len(bounds))
	}

	// All panes should have full width
	for i, b := range bounds {
		if b.Width != 100 {
			t.Errorf("Pane %d width: expected 100, got %d", i, b.Width)
		}
		if b.X != 0 {
			t.Errorf("Pane %d X: expected 0, got %d", i, b.X)
		}
	}

	// Panes should be stacked (increasing Y)
	if bounds[1].Y <= bounds[0].Y {
		t.Error("Pane 1 should be below pane 0")
	}
	if bounds[2].Y <= bounds[1].Y {
		t.Error("Pane 2 should be below pane 1")
	}

	// Heights should be equal (60/3 = 20 each)
	if bounds[0].Height != 20 {
		t.Errorf("Pane 0 height: expected 20, got %d", bounds[0].Height)
	}
}

func TestCalculatePaneBounds_Columns(t *testing.T) {
	// Columns: full-height panes side by side
	bounds := CalculatePaneBounds(LayoutColumns, 3, 90, 50, 0)

	if len(bounds) != 3 {
		t.Fatalf("Expected 3 bounds, got %d", len(bounds))
	}

	// All panes should have full height and Y=0
	for i, b := range bounds {
		if b.Height != 50 {
			t.Errorf("Pane %d height: expected 50, got %d", i, b.Height)
		}
		if b.Y != 0 {
			t.Errorf("Pane %d Y: expected 0, got %d", i, b.Y)
		}
	}

	// Panes should be side by side (increasing X)
	if bounds[1].X <= bounds[0].X {
		t.Error("Pane 1 should be to the right of pane 0")
	}
	if bounds[2].X <= bounds[1].X {
		t.Error("Pane 2 should be to the right of pane 1")
	}

	// Widths should be equal (90/3 = 30 each)
	if bounds[0].Width != 30 {
		t.Errorf("Pane 0 width: expected 30, got %d", bounds[0].Width)
	}
}

func TestCalculatePaneBounds_StartYOffset(t *testing.T) {
	// Verify that startY offset is correctly applied
	bounds := CalculatePaneBounds(LayoutGrid, 2, 100, 50, 5)

	for i, b := range bounds {
		if b.Y < 5 {
			t.Errorf("Pane %d Y (%d) should be >= startY (5)", i, b.Y)
		}
	}
}

// ============================================================================
// FindPaneAtPosition Tests - Critical for click detection
// ============================================================================

func TestFindPaneAtPosition_Empty(t *testing.T) {
	idx := FindPaneAtPosition(nil, 10, 10)
	if idx != -1 {
		t.Errorf("Expected -1 for nil bounds, got %d", idx)
	}

	idx = FindPaneAtPosition([]PaneBounds{}, 10, 10)
	if idx != -1 {
		t.Errorf("Expected -1 for empty bounds, got %d", idx)
	}
}

func TestFindPaneAtPosition_SinglePane(t *testing.T) {
	bounds := []PaneBounds{
		{X: 0, Y: 0, Width: 100, Height: 50},
	}

	tests := []struct {
		name     string
		x, y     int
		expected int
	}{
		{"top-left corner", 0, 0, 0},
		{"center", 50, 25, 0},
		{"bottom-right inside", 99, 49, 0},
		{"right edge (exclusive)", 100, 25, -1},
		{"bottom edge (exclusive)", 50, 50, -1},
		{"outside right", 150, 25, -1},
		{"outside bottom", 50, 75, -1},
		{"negative X", -1, 25, -1},
		{"negative Y", 50, -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := FindPaneAtPosition(bounds, tt.x, tt.y)
			if idx != tt.expected {
				t.Errorf("FindPaneAtPosition at (%d,%d): expected %d, got %d", tt.x, tt.y, tt.expected, idx)
			}
		})
	}
}

func TestFindPaneAtPosition_TwoPanesHorizontal(t *testing.T) {
	// Two panes side by side
	bounds := []PaneBounds{
		{X: 0, Y: 0, Width: 50, Height: 40},
		{X: 50, Y: 0, Width: 50, Height: 40},
	}

	tests := []struct {
		name     string
		x, y     int
		expected int
	}{
		{"left pane center", 25, 20, 0},
		{"left pane right edge", 49, 20, 0},
		{"right pane left edge", 50, 20, 1},
		{"right pane center", 75, 20, 1},
		{"right pane right edge", 99, 20, 1},
		{"boundary exact", 50, 20, 1}, // Boundary belongs to right pane
		{"outside both", 100, 20, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := FindPaneAtPosition(bounds, tt.x, tt.y)
			if idx != tt.expected {
				t.Errorf("FindPaneAtPosition at (%d,%d): expected %d, got %d", tt.x, tt.y, tt.expected, idx)
			}
		})
	}
}

func TestFindPaneAtPosition_TwoPanesVertical(t *testing.T) {
	// Two panes stacked vertically
	bounds := []PaneBounds{
		{X: 0, Y: 0, Width: 100, Height: 25},
		{X: 0, Y: 25, Width: 100, Height: 25},
	}

	tests := []struct {
		name     string
		x, y     int
		expected int
	}{
		{"top pane center", 50, 12, 0},
		{"top pane bottom edge", 50, 24, 0},
		{"bottom pane top edge", 50, 25, 1},
		{"bottom pane center", 50, 37, 1},
		{"boundary exact", 50, 25, 1}, // Boundary belongs to bottom pane
		{"outside both bottom", 50, 50, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := FindPaneAtPosition(bounds, tt.x, tt.y)
			if idx != tt.expected {
				t.Errorf("FindPaneAtPosition at (%d,%d): expected %d, got %d", tt.x, tt.y, tt.expected, idx)
			}
		})
	}
}

func TestFindPaneAtPosition_Grid2x2(t *testing.T) {
	// 2x2 grid
	bounds := []PaneBounds{
		{X: 0, Y: 0, Width: 50, Height: 25},   // Top-left
		{X: 50, Y: 0, Width: 50, Height: 25},  // Top-right
		{X: 0, Y: 25, Width: 50, Height: 25},  // Bottom-left
		{X: 50, Y: 25, Width: 50, Height: 25}, // Bottom-right
	}

	tests := []struct {
		name     string
		x, y     int
		expected int
	}{
		{"top-left pane", 25, 12, 0},
		{"top-right pane", 75, 12, 1},
		{"bottom-left pane", 25, 37, 2},
		{"bottom-right pane", 75, 37, 3},
		{"center intersection", 50, 25, 3}, // Belongs to bottom-right
		{"center-left intersection", 49, 25, 2},
		{"center-top intersection", 50, 24, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := FindPaneAtPosition(bounds, tt.x, tt.y)
			if idx != tt.expected {
				t.Errorf("FindPaneAtPosition at (%d,%d): expected %d, got %d", tt.x, tt.y, tt.expected, idx)
			}
		})
	}
}

func TestFindPaneAtPosition_WithStartYOffset(t *testing.T) {
	// Panes start at Y=1 (title bar offset)
	bounds := []PaneBounds{
		{X: 0, Y: 1, Width: 100, Height: 48},
	}

	tests := []struct {
		name     string
		x, y     int
		expected int
	}{
		{"on title bar", 50, 0, -1},
		{"just inside pane", 50, 1, 0},
		{"inside pane", 50, 25, 0},
		{"bottom edge", 50, 48, 0},
		{"outside bottom", 50, 49, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := FindPaneAtPosition(bounds, tt.x, tt.y)
			if idx != tt.expected {
				t.Errorf("FindPaneAtPosition at (%d,%d): expected %d, got %d", tt.x, tt.y, tt.expected, idx)
			}
		})
	}
}

func TestFindPaneAtPosition_MainLeftLayout(t *testing.T) {
	// Main pane on left (60% width), side panes stacked on right
	bounds := CalculatePaneBounds(LayoutMainLeft, 3, 100, 60, 0)

	tests := []struct {
		name     string
		x, y     int
		expected int
	}{
		{"main pane top-left", 10, 10, 0},
		{"main pane center", 30, 30, 0},
		{"main pane right edge", 59, 30, 0},
		{"side pane 1 left edge", 60, 10, 1},
		{"side pane 1 center", 80, 10, 1},
		{"side pane 2", 80, 45, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := FindPaneAtPosition(bounds, tt.x, tt.y)
			if idx != tt.expected {
				t.Errorf("FindPaneAtPosition at (%d,%d): expected %d, got %d", tt.x, tt.y, tt.expected, idx)
			}
		})
	}
}

// ============================================================================
// Integration Tests - CalculatePaneBounds + FindPaneAtPosition
// ============================================================================

func TestPaneBoundsAndClick_AllLayouts(t *testing.T) {
	layouts := []LayoutType{LayoutGrid, LayoutMainLeft, LayoutMainTop, LayoutRows, LayoutColumns}
	paneCounts := []int{1, 2, 3, 4, 5, 6}

	for _, layout := range layouts {
		for _, count := range paneCounts {
			t.Run(layout.String()+"/"+string(rune('0'+count)), func(t *testing.T) {
				bounds := CalculatePaneBounds(layout, count, 120, 60, 1)

				if len(bounds) != count {
					t.Fatalf("Expected %d bounds, got %d", count, len(bounds))
				}

				// Every pane should be clickable at its center
				for i, b := range bounds {
					centerX := b.X + b.Width/2
					centerY := b.Y + b.Height/2
					idx := FindPaneAtPosition(bounds, centerX, centerY)
					if idx != i {
						t.Errorf("Center of pane %d (%d,%d) returned index %d", i, centerX, centerY, idx)
					}
				}

				// Bounds should not overlap (each point belongs to at most one pane)
				// Test boundary points
				for i, b := range bounds {
					// Check corners that should belong to this pane
					topLeft := FindPaneAtPosition(bounds, b.X, b.Y)
					if topLeft != i {
						t.Errorf("Top-left of pane %d (%d,%d) returned %d", i, b.X, b.Y, topLeft)
					}
				}
			})
		}
	}
}

func TestPaneBoundsNonNegativeDimensions(t *testing.T) {
	layouts := []LayoutType{LayoutGrid, LayoutMainLeft, LayoutMainTop, LayoutRows, LayoutColumns}

	for _, layout := range layouts {
		for count := 1; count <= 9; count++ {
			bounds := CalculatePaneBounds(layout, count, 100, 50, 0)

			for i, b := range bounds {
				if b.X < 0 {
					t.Errorf("%s with %d panes: pane %d has negative X: %d", layout, count, i, b.X)
				}
				if b.Y < 0 {
					t.Errorf("%s with %d panes: pane %d has negative Y: %d", layout, count, i, b.Y)
				}
				if b.Width <= 0 {
					t.Errorf("%s with %d panes: pane %d has non-positive width: %d", layout, count, i, b.Width)
				}
				if b.Height <= 0 {
					t.Errorf("%s with %d panes: pane %d has non-positive height: %d", layout, count, i, b.Height)
				}
			}
		}
	}
}

// ============================================================================
// Uneven Width/Height Distribution Tests
// ============================================================================

func TestCalculateLayout_UnevenWidthDistribution(t *testing.T) {
	// Test that total widths always sum to totalWidth (no pixels lost)
	testCases := []struct {
		name      string
		layout    LayoutType
		paneCount int
		width     int
		height    int
	}{
		// Columns layout - width should be fully distributed
		{"Columns 2 panes odd width", LayoutColumns, 2, 101, 50},
		{"Columns 3 panes odd width", LayoutColumns, 3, 100, 50},
		{"Columns 4 panes odd width", LayoutColumns, 4, 123, 50},

		// Grid layout - width should be fully distributed per row
		{"Grid 2 panes odd width", LayoutGrid, 2, 101, 50},
		{"Grid 3 panes odd width", LayoutGrid, 3, 101, 50},
		{"Grid 4 panes odd width", LayoutGrid, 4, 101, 50},
		{"Grid 5 panes odd width", LayoutGrid, 5, 101, 60},
		{"Grid 6 panes odd width", LayoutGrid, 6, 101, 60},

		// MainTop layout - bottom panes should fill width
		{"MainTop 3 panes odd width", LayoutMainTop, 3, 101, 50},
		{"MainTop 4 panes odd width", LayoutMainTop, 4, 100, 50},

		// MainLeft layout - side width should be remainder
		{"MainLeft 3 panes odd width", LayoutMainLeft, 3, 101, 50},

		// Rows layout - all panes should have full width
		{"Rows 2 panes", LayoutRows, 2, 101, 50},
		{"Rows 3 panes", LayoutRows, 3, 101, 60},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sizes := CalculateLayout(tc.layout, tc.paneCount, tc.width, tc.height)

			if len(sizes) != tc.paneCount {
				t.Fatalf("Expected %d sizes, got %d", tc.paneCount, len(sizes))
			}

			// For non-grid layouts that arrange horizontally, sum widths
			switch tc.layout {
			case LayoutColumns:
				totalWidth := 0
				for _, s := range sizes {
					totalWidth += s.Width
				}
				if totalWidth != tc.width {
					t.Errorf("Total width %d != expected %d (sizes: %v)", totalWidth, tc.width, sizesToWidths(sizes))
				}

			case LayoutRows:
				// All rows should have full width
				for i, s := range sizes {
					if s.Width != tc.width {
						t.Errorf("Pane %d width %d != expected %d", i, s.Width, tc.width)
					}
				}

			case LayoutMainTop:
				// Main pane (first) should have full width
				if sizes[0].Width != tc.width {
					t.Errorf("Main pane width %d != expected %d", sizes[0].Width, tc.width)
				}
				// Bottom panes should sum to full width
				bottomWidth := 0
				for i := 1; i < len(sizes); i++ {
					bottomWidth += sizes[i].Width
				}
				if bottomWidth != tc.width {
					t.Errorf("Bottom panes total width %d != expected %d", bottomWidth, tc.width)
				}

			case LayoutMainLeft:
				// Main + side should equal total width
				mainWidth := sizes[0].Width
				sideWidth := sizes[1].Width // All side panes have same width
				if mainWidth+sideWidth != tc.width {
					t.Errorf("Main (%d) + Side (%d) = %d != expected %d",
						mainWidth, sideWidth, mainWidth+sideWidth, tc.width)
				}

			case LayoutGrid:
				// Verify each row sums to total width using bounds
				bounds := CalculatePaneBounds(tc.layout, tc.paneCount, tc.width, tc.height, 0)
				cols, rows := calculateGridDimensions(tc.paneCount)
				for row := 0; row < rows; row++ {
					rowWidth := 0
					for i, b := range bounds {
						if i/cols == row {
							rowWidth += b.Width
						}
					}
					if rowWidth != tc.width {
						t.Errorf("Row %d total width %d != expected %d", row, rowWidth, tc.width)
					}
				}
			}
		})
	}
}

func sizesToWidths(sizes []PaneSize) []int {
	widths := make([]int, len(sizes))
	for i, s := range sizes {
		widths[i] = s.Width
	}
	return widths
}
