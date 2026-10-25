package timeline

import (
	"reflect"
	"testing"
)

func TestSpanOverlaps(t *testing.T) {
	for _, tt := range []struct {
		name string
		a, b Span
		want bool
	}{
		{"intersection", Span{0, 1000}, Span{999, 2000}, true},
		{"contained", Span{0, 1000}, Span{100, 200}, true},
		{"touching", Span{0, 1000}, Span{1000, 2000}, false},
		{"disjoint", Span{0, 1000}, Span{1001, 2000}, false},
		{"empty", Span{0, 1000}, Span{500, 500}, false},
		{"reversed", Span{0, 1000}, Span{700, 500}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Overlaps(tt.b); got != tt.want || tt.b.Overlaps(tt.a) != tt.want {
				t.Fatalf("%v overlaps %v = %v, want %v in both directions", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCells(t *testing.T) {
	for _, tt := range []struct {
		duration, width, count int
		last                   Span
	}{
		{1, 1000, 1, Span{0, 1}},
		{2501, 1000, 3, Span{2000, 2501}},
		{89999, 1000, 90, Span{89000, 89999}},
		{90000, 2500, 36, Span{87500, 90000}},
		{90001, 2500, 37, Span{90000, 90001}},
	} {
		cells := Cells(tt.duration)
		if CellMs(tt.duration) != tt.width || len(cells) != tt.count || cells[len(cells)-1] != tt.last {
			t.Fatalf("duration %d: width=%d, cells=%v", tt.duration, CellMs(tt.duration), cells)
		}
		for i, cell := range cells {
			if cell.StartMs != i*tt.width || cell.EndMs <= cell.StartMs || cell.EndMs > tt.duration {
				t.Fatalf("duration %d: invalid cell %d: %v", tt.duration, i, cell)
			}
			if i > 0 && cells[i-1].EndMs != cell.StartMs {
				t.Fatalf("non-contiguous cells: %v", cells)
			}
		}
	}
	for _, duration := range []int{-1, 0} {
		if got := Cells(duration); len(got) != 0 {
			t.Fatalf("Cells(%d) = %v, want empty", duration, got)
		}
	}
}

func TestCellsOverlapping(t *testing.T) {
	for _, tt := range []struct {
		name     string
		duration int
		span     Span
		want     []int
	}{
		{"exact boundary", 3500, Span{1000, 2000}, []int{1}},
		{"cross boundary", 3500, Span{999, 1001}, []int{0, 1}},
		{"clip start", 3500, Span{-200, 1}, []int{0}},
		{"clip end", 3500, Span{3000, 9000}, []int{3}},
		{"clip both", 2501, Span{-1, 9000}, []int{0, 1, 2}},
		{"long grid", 90001, Span{87499, 90001}, []int{34, 35, 36}},
		{"empty", 3500, Span{1000, 1000}, nil},
		{"reversed", 3500, Span{2000, 1000}, nil},
		{"before video", 3500, Span{-100, 0}, nil},
		{"after video", 3500, Span{3500, 4000}, nil},
		{"zero duration", 0, Span{0, 1000}, nil},
		{"negative duration", -1, Span{-10, 1000}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := CellsOverlapping(tt.duration, tt.span); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergeSentences(t *testing.T) {
	for _, tt := range []struct {
		name     string
		in, want []Sentence
		min, max int
	}{
		{"empty", nil, nil, 8, 120},
		{"Unicode and forward preference", []Sentence{{Span{0, 10}, "你好"}, {Span{20, 30}, "世界"}, {Span{30, 40}, "再见"}}, []Sentence{{Span{0, 40}, "你好世界再见"}}, 5, 6},
		{"exact minimum stays separate", []Sentence{{Span{0, 10}, "你好"}, {Span{10, 20}, "世界"}}, []Sentence{{Span{0, 10}, "你好"}, {Span{10, 20}, "世界"}}, 2, 4},
		{"trailing short merges backward", []Sentence{{Span{0, 10}, "完整句子"}, {Span{20, 30}, "啊"}}, []Sentence{{Span{0, 30}, "完整句子啊"}}, 3, 5},
		{"next cannot fit so merge backward", []Sentence{{Span{0, 10}, "完整句子"}, {Span{10, 20}, "啊"}, {Span{20, 30}, "另一个完整句子"}}, []Sentence{{Span{0, 20}, "完整句子啊"}, {Span{20, 30}, "另一个完整句子"}}, 3, 5},
		{"max prevents merge", []Sentence{{Span{0, 10}, "你好"}, {Span{10, 20}, "世界"}}, []Sentence{{Span{0, 10}, "你好"}, {Span{10, 20}, "世界"}}, 3, 3},
		{"long sentence preserved", []Sentence{{Span{0, 10}, "很长很长的句子"}, {Span{10, 20}, "啊"}}, []Sentence{{Span{0, 10}, "很长很长的句子"}, {Span{10, 20}, "啊"}}, 3, 5},
		{"overlap preserved", []Sentence{{Span{0, 20}, "你"}, {Span{10, 30}, "好"}}, []Sentence{{Span{0, 20}, "你"}, {Span{10, 30}, "好"}}, 3, 10},
		{"invalid span preserved", []Sentence{{Span{10, 10}, "你"}, {Span{20, 30}, "好"}}, []Sentence{{Span{10, 10}, "你"}, {Span{20, 30}, "好"}}, 3, 10},
		{"out of order preserved", []Sentence{{Span{20, 30}, "你"}, {Span{0, 10}, "好"}}, []Sentence{{Span{20, 30}, "你"}, {Span{0, 10}, "好"}}, 3, 10},
		{"text bytes preserved", []Sentence{{Span{0, 10}, "hello "}, {Span{10, 20}, "世界！"}}, []Sentence{{Span{0, 20}, "hello 世界！"}}, 8, 9},
		{"disabled", []Sentence{{Span{0, 10}, "你"}, {Span{10, 20}, "好"}}, []Sentence{{Span{0, 10}, "你"}, {Span{10, 20}, "好"}}, 0, 10},
		{"invalid maximum", []Sentence{{Span{0, 10}, "你"}, {Span{10, 20}, "好"}}, []Sentence{{Span{0, 10}, "你"}, {Span{10, 20}, "好"}}, 3, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := append([]Sentence(nil), tt.in...)
			got := MergeSentences(tt.in, tt.min, tt.max)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.in, original) {
				t.Fatal("input was modified")
			}
			for i := 1; i < len(got); i++ {
				if tt.name != "overlap preserved" && got[i-1].Span.Overlaps(got[i].Span) {
					t.Fatalf("merged sentences overlap: %v", got)
				}
			}
		})
	}
}

func TestSpriteTile(t *testing.T) {
	layout := SpriteLayout{TileW: 160, TileH: 90, Cols: 3, Rows: 2, Count: 13}
	for _, tt := range []struct{ cell, sprite, x, y int }{
		{0, 0, 0, 0}, {2, 0, 320, 0}, {3, 0, 0, 90}, {5, 0, 320, 90},
		{6, 1, 0, 0}, {12, 2, 0, 0}, {-1, -1, -1, -1}, {13, -1, -1, -1},
	} {
		if sprite, x, y := layout.Tile(tt.cell); sprite != tt.sprite || x != tt.x || y != tt.y {
			t.Fatalf("Tile(%d) = (%d,%d,%d), want (%d,%d,%d)", tt.cell, sprite, x, y, tt.sprite, tt.x, tt.y)
		}
	}
	for _, layout := range []SpriteLayout{{}, {TileW: 160, TileH: 90, Cols: 0, Rows: 2, Count: 1}, {TileW: -1, TileH: 90, Cols: 3, Rows: 2, Count: 1}} {
		if sprite, x, y := layout.Tile(0); sprite != -1 || x != -1 || y != -1 {
			t.Fatalf("invalid layout %v returned (%d,%d,%d)", layout, sprite, x, y)
		}
	}
}
