package timeline

const GridSeg = "grid@v1"

func CellMs(durationMs int) int {
	if durationMs >= 90000 {
		return 2500
	}
	return 1000
}

// Cells returns no cells for a nonpositive duration.
func Cells(durationMs int) []Span {
	if durationMs <= 0 {
		return nil
	}
	width := CellMs(durationMs)
	cells := make([]Span, (durationMs-1)/width+1)
	for i := range cells {
		start, end := i*width, durationMs
		if durationMs-start > width {
			end = start + width
		}
		cells[i] = Span{StartMs: start, EndMs: end}
	}
	return cells
}

// CellsOverlapping clips s to the video and returns no cells for an empty range.
func CellsOverlapping(durationMs int, s Span) []int {
	s.StartMs = max(s.StartMs, 0)
	s.EndMs = min(s.EndMs, durationMs)
	if s.StartMs >= s.EndMs {
		return nil
	}
	width := CellMs(durationMs)
	first, last := s.StartMs/width, (s.EndMs-1)/width
	cells := make([]int, last-first+1)
	for i := range cells {
		cells[i] = first + i
	}
	return cells
}
