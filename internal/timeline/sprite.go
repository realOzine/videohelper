package timeline

type SpriteLayout struct {
	TileW int `json:"tile_w"`
	TileH int `json:"tile_h"`
	Cols  int `json:"cols"`
	Rows  int `json:"rows"`
	Count int `json:"count"`
}

// Tile returns zero-based sprite and pixel coordinates, or (-1, -1, -1) if invalid.
func (l SpriteLayout) Tile(cell int) (sprite, x, y int) {
	if cell < 0 || cell >= l.Count || l.TileW <= 0 || l.TileH <= 0 || l.Cols <= 0 || l.Rows <= 0 {
		return -1, -1, -1
	}
	row := cell / l.Cols
	return row / l.Rows, cell % l.Cols * l.TileW, row % l.Rows * l.TileH
}
