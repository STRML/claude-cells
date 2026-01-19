package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// LayoutType represents the type of pane layout
type LayoutType int

const (
	// LayoutGrid arranges panes in a balanced grid pattern
	LayoutGrid LayoutType = iota
	// LayoutMainLeft has one large pane on left, stack on right
	LayoutMainLeft
	// LayoutMainTop has one large pane on top, row of panes below
	LayoutMainTop
	// LayoutRows stacks full-width panes vertically
	LayoutRows
	// LayoutColumns arranges full-height panes side by side
	LayoutColumns
)

// LayoutNames maps layout types to display names
var LayoutNames = map[LayoutType]string{
	LayoutGrid:     "Grid",
	LayoutMainLeft: "Main+Stack",
	LayoutMainTop:  "Main+Row",
	LayoutRows:     "Rows",
	LayoutColumns:  "Columns",
}

// NextLayout returns the next layout type in the cycle
func (l LayoutType) Next() LayoutType {
	return (l + 1) % 5
}

// String returns the display name for the layout
func (l LayoutType) String() string {
	if name, ok := LayoutNames[l]; ok {
		return name
	}
	return "Unknown"
}

// PaneSize holds the dimensions for a pane
type PaneSize struct {
	Width  int
	Height int
}

// PanePosition holds both size and position info for rendering
type PanePosition struct {
	Index  int
	Width  int
	Height int
	Row    int // Which row this pane is in (for rendering)
	Col    int // Which column this pane is in (for rendering)
}

// CalculateLayout computes pane sizes based on layout type and available space
func CalculateLayout(layout LayoutType, paneCount, totalWidth, totalHeight int) []PaneSize {
	if paneCount == 0 {
		return nil
	}

	switch layout {
	case LayoutGrid:
		return calculateGridLayout(paneCount, totalWidth, totalHeight)
	case LayoutMainLeft:
		return calculateMainLeftLayout(paneCount, totalWidth, totalHeight)
	case LayoutMainTop:
		return calculateMainTopLayout(paneCount, totalWidth, totalHeight)
	case LayoutRows:
		return calculateRowsLayout(paneCount, totalWidth, totalHeight)
	case LayoutColumns:
		return calculateColumnsLayout(paneCount, totalWidth, totalHeight)
	default:
		return calculateGridLayout(paneCount, totalWidth, totalHeight)
	}
}

// calculateGridLayout creates a balanced grid
// 1 pane: 1x1, 2 panes: 1x2, 3-4 panes: 2x2, 5-6 panes: 2x3, 7-9 panes: 3x3, etc.
func calculateGridLayout(paneCount, totalWidth, totalHeight int) []PaneSize {
	if paneCount == 1 {
		return []PaneSize{{totalWidth, totalHeight}}
	}

	// Calculate optimal grid dimensions
	cols, rows := calculateGridDimensions(paneCount)

	paneWidth := totalWidth / cols
	paneHeight := totalHeight / rows

	sizes := make([]PaneSize, paneCount)
	for i := 0; i < paneCount; i++ {
		row := i / cols
		col := i % cols

		// Handle last row which might have fewer panes
		panesInThisRow := cols
		if row == rows-1 {
			panesInThisRow = paneCount - (rows-1)*cols
		}

		w := paneWidth
		h := paneHeight

		// If this is the last row and has fewer panes, make them wider
		if row == rows-1 && panesInThisRow < cols {
			w = totalWidth / panesInThisRow
		}

		// Last column in a row gets any extra width
		if col == panesInThisRow-1 {
			w = totalWidth - (panesInThisRow-1)*(totalWidth/panesInThisRow)
		}

		// Last row gets any extra height
		if row == rows-1 {
			h = totalHeight - (rows-1)*paneHeight
		}

		sizes[i] = PaneSize{w, h}
	}

	return sizes
}

// calculateGridDimensions returns optimal cols x rows for n panes
func calculateGridDimensions(n int) (cols, rows int) {
	switch {
	case n <= 1:
		return 1, 1
	case n == 2:
		return 2, 1
	case n <= 4:
		return 2, 2
	case n <= 6:
		return 3, 2
	case n <= 9:
		return 3, 3
	case n <= 12:
		return 4, 3
	default:
		return 4, (n + 3) / 4
	}
}

// calculateMainLeftLayout has one large pane on left, stack on right
func calculateMainLeftLayout(paneCount, totalWidth, totalHeight int) []PaneSize {
	if paneCount == 1 {
		return []PaneSize{{totalWidth, totalHeight}}
	}

	// Main pane takes 60% of width
	mainWidth := totalWidth * 60 / 100
	sideWidth := totalWidth - mainWidth

	// Side panes share the height equally
	sideCount := paneCount - 1
	sideHeight := totalHeight / sideCount

	sizes := make([]PaneSize, paneCount)
	sizes[0] = PaneSize{mainWidth, totalHeight}

	for i := 1; i < paneCount; i++ {
		h := sideHeight
		// Last side pane gets any extra height
		if i == paneCount-1 {
			h = totalHeight - (sideCount-1)*sideHeight
		}
		sizes[i] = PaneSize{sideWidth, h}
	}

	return sizes
}

// calculateMainTopLayout has one large pane on top, row of panes below
func calculateMainTopLayout(paneCount, totalWidth, totalHeight int) []PaneSize {
	if paneCount == 1 {
		return []PaneSize{{totalWidth, totalHeight}}
	}

	// Main pane takes 60% of height
	mainHeight := totalHeight * 60 / 100
	bottomHeight := totalHeight - mainHeight

	// Bottom panes share the width equally
	bottomCount := paneCount - 1
	bottomWidth := totalWidth / bottomCount

	sizes := make([]PaneSize, paneCount)
	sizes[0] = PaneSize{totalWidth, mainHeight}

	for i := 1; i < paneCount; i++ {
		w := bottomWidth
		// Last bottom pane gets any extra width
		if i == paneCount-1 {
			w = totalWidth - (bottomCount-1)*bottomWidth
		}
		sizes[i] = PaneSize{w, bottomHeight}
	}

	return sizes
}

// calculateRowsLayout stacks full-width panes vertically
func calculateRowsLayout(paneCount, totalWidth, totalHeight int) []PaneSize {
	if paneCount == 0 {
		return nil
	}

	rowHeight := totalHeight / paneCount

	sizes := make([]PaneSize, paneCount)
	for i := 0; i < paneCount; i++ {
		h := rowHeight
		// Last row gets any extra height
		if i == paneCount-1 {
			h = totalHeight - (paneCount-1)*rowHeight
		}
		sizes[i] = PaneSize{totalWidth, h}
	}

	return sizes
}

// calculateColumnsLayout arranges full-height panes side by side
func calculateColumnsLayout(paneCount, totalWidth, totalHeight int) []PaneSize {
	if paneCount == 0 {
		return nil
	}

	colWidth := totalWidth / paneCount

	sizes := make([]PaneSize, paneCount)
	for i := 0; i < paneCount; i++ {
		w := colWidth
		// Last column gets any extra width
		if i == paneCount-1 {
			w = totalWidth - (paneCount-1)*colWidth
		}
		sizes[i] = PaneSize{w, totalHeight}
	}

	return sizes
}

// RenderPanesWithLayout renders panes according to the specified layout
func RenderPanesWithLayout(panes []PaneModel, layout LayoutType, totalWidth, totalHeight int) string {
	if len(panes) == 0 {
		return ""
	}

	switch layout {
	case LayoutGrid:
		return renderGridLayout(panes, totalWidth, totalHeight)
	case LayoutMainLeft:
		return renderMainLeftLayout(panes)
	case LayoutMainTop:
		return renderMainTopLayout(panes)
	case LayoutRows:
		return renderRowsLayout(panes)
	case LayoutColumns:
		return renderColumnsLayout(panes)
	default:
		return renderGridLayout(panes, totalWidth, totalHeight)
	}
}

// renderGridLayout renders panes in a grid
func renderGridLayout(panes []PaneModel, totalWidth, totalHeight int) string {
	if len(panes) == 1 {
		return panes[0].View()
	}

	cols, rows := calculateGridDimensions(len(panes))

	var rowViews []string
	for row := 0; row < rows; row++ {
		var rowPanes []string
		for col := 0; col < cols; col++ {
			idx := row*cols + col
			if idx >= len(panes) {
				break
			}
			rowPanes = append(rowPanes, panes[idx].View())
		}
		if len(rowPanes) > 0 {
			rowViews = append(rowViews, lipgloss.JoinHorizontal(lipgloss.Top, rowPanes...))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, rowViews...)
}

// renderMainLeftLayout renders main pane on left, stack on right
func renderMainLeftLayout(panes []PaneModel) string {
	if len(panes) == 1 {
		return panes[0].View()
	}

	main := panes[0].View()

	var sidePanes []string
	for i := 1; i < len(panes); i++ {
		sidePanes = append(sidePanes, panes[i].View())
	}
	side := lipgloss.JoinVertical(lipgloss.Left, sidePanes...)

	return lipgloss.JoinHorizontal(lipgloss.Top, main, side)
}

// renderMainTopLayout renders main pane on top, row below
func renderMainTopLayout(panes []PaneModel) string {
	if len(panes) == 1 {
		return panes[0].View()
	}

	main := panes[0].View()

	var bottomPanes []string
	for i := 1; i < len(panes); i++ {
		bottomPanes = append(bottomPanes, panes[i].View())
	}
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, bottomPanes...)

	return lipgloss.JoinVertical(lipgloss.Left, main, bottom)
}

// renderRowsLayout renders full-width panes stacked vertically
func renderRowsLayout(panes []PaneModel) string {
	var views []string
	for _, p := range panes {
		views = append(views, p.View())
	}
	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

// renderColumnsLayout renders full-height panes side by side
func renderColumnsLayout(panes []PaneModel) string {
	var views []string
	for _, p := range panes {
		views = append(views, p.View())
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, views...)
}

// GridPosition represents a pane's position in the grid
type GridPosition struct {
	Row int
	Col int
}

// CalculateGridPositions returns the grid row/col for each pane index
func CalculateGridPositions(layout LayoutType, paneCount int) []GridPosition {
	if paneCount == 0 {
		return nil
	}

	positions := make([]GridPosition, paneCount)

	switch layout {
	case LayoutGrid:
		cols, _ := calculateGridDimensions(paneCount)
		for i := 0; i < paneCount; i++ {
			positions[i] = GridPosition{
				Row: i / cols,
				Col: i % cols,
			}
		}

	case LayoutMainLeft:
		// Main pane at (0,0), side panes stacked in column 1
		positions[0] = GridPosition{Row: 0, Col: 0}
		for i := 1; i < paneCount; i++ {
			positions[i] = GridPosition{Row: i - 1, Col: 1}
		}

	case LayoutMainTop:
		// Main pane at (0,0), bottom panes in row 1
		positions[0] = GridPosition{Row: 0, Col: 0}
		for i := 1; i < paneCount; i++ {
			positions[i] = GridPosition{Row: 1, Col: i - 1}
		}

	case LayoutRows:
		// All panes stacked vertically (column 0)
		for i := 0; i < paneCount; i++ {
			positions[i] = GridPosition{Row: i, Col: 0}
		}

	case LayoutColumns:
		// All panes side by side (row 0)
		for i := 0; i < paneCount; i++ {
			positions[i] = GridPosition{Row: 0, Col: i}
		}

	default:
		// Fall back to grid
		cols, _ := calculateGridDimensions(paneCount)
		for i := 0; i < paneCount; i++ {
			positions[i] = GridPosition{
				Row: i / cols,
				Col: i % cols,
			}
		}
	}

	return positions
}

// Direction represents a navigation direction
type Direction int

const (
	DirUp Direction = iota
	DirDown
	DirLeft
	DirRight
)

// FindNeighbor finds the pane index in the given direction from currentIndex
// Returns -1 if no neighbor exists in that direction
func FindNeighbor(layout LayoutType, paneCount, currentIndex int, dir Direction) int {
	if paneCount <= 1 || currentIndex < 0 || currentIndex >= paneCount {
		return -1
	}

	positions := CalculateGridPositions(layout, paneCount)
	current := positions[currentIndex]

	// Find target row/col
	targetRow, targetCol := current.Row, current.Col
	switch dir {
	case DirUp:
		targetRow--
	case DirDown:
		targetRow++
	case DirLeft:
		targetCol--
	case DirRight:
		targetCol++
	}

	// Find pane at target position (exact match first)
	for i, pos := range positions {
		if pos.Row == targetRow && pos.Col == targetCol {
			return i
		}
	}

	// For grid layouts with uneven last row, try to find closest match
	if layout == LayoutGrid && (dir == DirDown || dir == DirUp) {
		// Find any pane in the target row, preferring same column or closest
		bestMatch := -1
		bestDist := 999
		for i, pos := range positions {
			if pos.Row == targetRow {
				dist := abs(pos.Col - current.Col)
				if dist < bestDist {
					bestDist = dist
					bestMatch = i
				}
			}
		}
		if bestMatch >= 0 {
			return bestMatch
		}
	}

	// Wrap around behavior for rows/columns layouts
	switch layout {
	case LayoutRows:
		if dir == DirUp && targetRow < 0 {
			return paneCount - 1
		}
		if dir == DirDown && targetRow >= paneCount {
			return 0
		}
	case LayoutColumns:
		if dir == DirLeft && targetCol < 0 {
			return paneCount - 1
		}
		if dir == DirRight && targetCol >= paneCount {
			return 0
		}
	}

	return -1
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
