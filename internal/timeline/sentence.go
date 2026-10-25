package timeline

import "unicode/utf8"

const SentenceSeg = "sentence@v1"

type Sentence struct {
	Span Span
	Text string
}

// MergeSentences counts Unicode code points and concatenates text unchanged.
// Short sentences prefer the next neighbor, then the previous one if it fits.
// Invalid or out-of-order spans stay separate; nonpositive limits disable merging.
func MergeSentences(in []Sentence, minChars, maxChars int) []Sentence {
	if len(in) == 0 {
		return nil
	}
	out := make([]Sentence, 0, len(in))
	for i := 0; i < len(in); i++ {
		first := i
		current := in[i]
		chars := utf8.RuneCountInString(current.Text)
		if minChars > 0 && maxChars > 0 {
			for chars < minChars && i+1 < len(in) && in[i].Span.StartMs <= in[i+1].Span.StartMs && canMerge(current, in[i+1], maxChars) {
				i++
				current.Span.StartMs = min(current.Span.StartMs, in[i].Span.StartMs)
				current.Span.EndMs = max(current.Span.EndMs, in[i].Span.EndMs)
				current.Text += in[i].Text
				chars = utf8.RuneCountInString(current.Text)
			}
			if chars < minChars && len(out) > 0 && in[first-1].Span.StartMs <= in[first].Span.StartMs && canMerge(out[len(out)-1], current, maxChars) {
				out[len(out)-1].Span.StartMs = min(out[len(out)-1].Span.StartMs, current.Span.StartMs)
				out[len(out)-1].Span.EndMs = max(out[len(out)-1].Span.EndMs, current.Span.EndMs)
				out[len(out)-1].Text += current.Text
				continue
			}
		}
		out = append(out, current)
	}
	return out
}

func canMerge(a, b Sentence, maxChars int) bool {
	return a.Span.StartMs < a.Span.EndMs && b.Span.StartMs < b.Span.EndMs &&
		a.Span.StartMs <= b.Span.StartMs &&
		utf8.RuneCountInString(a.Text)+utf8.RuneCountInString(b.Text) <= maxChars
}
