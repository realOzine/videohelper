package timeline

// Span is a half-open interval in milliseconds. EndMs <= StartMs is empty.
type Span struct {
	StartMs, EndMs int
}

func (s Span) Overlaps(o Span) bool {
	return s.StartMs < s.EndMs && o.StartMs < o.EndMs && s.StartMs < o.EndMs && o.StartMs < s.EndMs
}
