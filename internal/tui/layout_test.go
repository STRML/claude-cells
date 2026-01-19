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
