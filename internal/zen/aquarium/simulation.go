package aquarium

import (
	"math"
	"math/rand"
)

const (
	MaxFish    = 18
	MaxFood    = 24
	MaxBubbles = 48
)

type Fish struct {
	X, Y      float64
	DX        float64
	Size      int
	Color     int
	TurnAfter int
}

type Particle struct{ X, Y float64 }

type Scene struct {
	Width, Height int
	Fish          []Fish
	Food          []Particle
	Bubbles       []Particle
	Paused        bool
	Speed         float64
	frame         int
	rng           *rand.Rand
}

func New(width, height int, seed int64) *Scene {
	s := &Scene{rng: rand.New(rand.NewSource(seed)), Speed: 1}
	s.Resize(width, height)
	for range 6 {
		s.AddFish()
	}
	return s
}

func (s *Scene) Resize(width, height int) {
	s.Width, s.Height = max(12, width), max(6, height)
	for i := range s.Fish {
		s.Fish[i].X = clamp(s.Fish[i].X, 2, float64(s.Width-3))
		s.Fish[i].Y = clamp(s.Fish[i].Y, 1, float64(s.Height-3))
	}
}

func (s *Scene) AddFish() {
	if len(s.Fish) >= MaxFish {
		return
	}
	direction := 0.45 + s.rng.Float64()*0.55
	if s.rng.Intn(2) == 0 {
		direction = -direction
	}
	s.Fish = append(s.Fish, Fish{X: 3 + s.rng.Float64()*float64(max(1, s.Width-7)), Y: 1 + s.rng.Float64()*float64(max(1, s.Height-5)), DX: direction, Size: 1 + s.rng.Intn(2), Color: s.rng.Intn(4), TurnAfter: 45 + s.rng.Intn(120)})
}

func (s *Scene) RemoveFish() {
	if len(s.Fish) > 1 {
		s.Fish = s.Fish[:len(s.Fish)-1]
	}
}

func (s *Scene) DropFood() {
	if len(s.Food) < MaxFood {
		s.Food = append(s.Food, Particle{X: 2 + s.rng.Float64()*float64(max(1, s.Width-4)), Y: 1})
	}
}

func (s *Scene) Step() {
	if s.Paused {
		return
	}
	s.frame++
	for i := range s.Fish {
		fish := &s.Fish[i]
		fish.TurnAfter--
		if fish.TurnAfter <= 0 {
			if s.rng.Intn(3) == 0 {
				fish.DX = -fish.DX
			}
			fish.TurnAfter = 45 + s.rng.Intn(120)
		}
		if target, ok := nearestFood(*fish, s.Food); ok {
			if target.X < fish.X {
				fish.DX = -math.Abs(fish.DX)
			} else {
				fish.DX = math.Abs(fish.DX)
			}
			fish.Y += clamp(target.Y-fish.Y, -0.22, 0.22) * s.Speed
		}
		fish.X += fish.DX * s.Speed
		if fish.X < 2 {
			fish.X, fish.DX = 2, math.Abs(fish.DX)
		}
		if fish.X > float64(s.Width-3) {
			fish.X, fish.DX = float64(s.Width-3), -math.Abs(fish.DX)
		}
		fish.Y = clamp(fish.Y, 1, float64(s.Height-3))
	}
	for i := range s.Food {
		s.Food[i].Y += 0.16 * s.Speed
	}
	s.consumeFood()
	if s.frame%8 == 0 && len(s.Bubbles) < MaxBubbles {
		s.Bubbles = append(s.Bubbles, Particle{X: 1 + s.rng.Float64()*float64(s.Width-2), Y: float64(s.Height - 2)})
	}
	keptBubbles := s.Bubbles[:0]
	for _, bubble := range s.Bubbles {
		bubble.Y -= 0.28 * s.Speed
		if bubble.Y >= 0 {
			keptBubbles = append(keptBubbles, bubble)
		}
	}
	s.Bubbles = keptBubbles
}

func (s *Scene) consumeFood() {
	kept := s.Food[:0]
	for _, food := range s.Food {
		eaten := food.Y >= float64(s.Height-2)
		for _, fish := range s.Fish {
			if math.Hypot(fish.X-food.X, fish.Y-food.Y) < 1.8 {
				eaten = true
			}
		}
		if !eaten {
			kept = append(kept, food)
		}
	}
	s.Food = kept
}

func nearestFood(fish Fish, food []Particle) (Particle, bool) {
	var best Particle
	distance := math.MaxFloat64
	for _, item := range food {
		if d := math.Hypot(fish.X-item.X, fish.Y-item.Y); d < distance {
			best, distance = item, d
		}
	}
	return best, distance < math.MaxFloat64
}

func clamp(value, low, high float64) float64 { return math.Max(low, math.Min(high, value)) }
