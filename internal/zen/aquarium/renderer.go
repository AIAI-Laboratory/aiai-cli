package aquarium

import "strings"

type Cell struct {
	Rune  rune
	Color int
}

func (s *Scene) Render(unicode bool) [][]Cell {
	grid := make([][]Cell, s.Height)
	for y := range grid {
		grid[y] = make([]Cell, s.Width)
		for x := range grid[y] {
			grid[y][x].Rune = ' '
		}
	}
	for _, bubble := range s.Bubbles {
		put(grid, int(bubble.X), int(bubble.Y), choose(unicode, '·', '.'), 4)
	}
	for _, food := range s.Food {
		put(grid, int(food.X), int(food.Y), choose(unicode, '•', '*'), 5)
	}
	for _, fish := range s.Fish {
		shape := "><>"
		if fish.Size > 1 {
			shape = "><(((°>"
		}
		if fish.DX < 0 {
			shape = "<°)))><"
			if fish.Size == 1 {
				shape = "<><"
			}
		}
		if !unicode {
			shape = strings.ReplaceAll(shape, "°", "o")
		}
		start := int(fish.X) - len([]rune(shape))/2
		for offset, r := range []rune(shape) {
			put(grid, start+offset, int(fish.Y), r, fish.Color)
		}
	}
	// A rare, short-lived glint keeps the water feeling alive without adding noise.
	if s.frame%311 < 4 {
		put(grid, max(1, s.Width/3), max(1, s.Height/3), choose(unicode, '✦', '+'), 5)
	}
	bottom := s.Height - 1
	for x := range s.Width {
		floor := choose(unicode, '≈', '~')
		if (x+s.frame/4)%9 == 0 {
			floor = choose(unicode, '▴', '^')
		}
		put(grid, x, bottom, floor, 6)
		if bottom > 0 && (x*7+s.frame/12)%23 == 0 {
			put(grid, x, bottom-1, choose(unicode, '╿', '|'), 3)
		}
	}
	return grid
}

func put(grid [][]Cell, x, y int, r rune, color int) {
	if y >= 0 && y < len(grid) && x >= 0 && x < len(grid[y]) {
		grid[y][x] = Cell{Rune: r, Color: color}
	}
}

func choose(unicode bool, rich, fallback rune) rune {
	if unicode {
		return rich
	}
	return fallback
}
