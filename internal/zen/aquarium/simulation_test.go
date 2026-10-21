package aquarium

import "testing"

func TestFishMovementBoundariesPauseAndResize(t *testing.T) {
	scene := New(30, 10, 1)
	scene.Fish = []Fish{{X: 28, Y: 4, DX: 1, TurnAfter: 100}}
	scene.Step()
	if scene.Fish[0].X > 27 || scene.Fish[0].DX >= 0 {
		t.Fatalf("boundary failed: %+v", scene.Fish[0])
	}
	before := scene.Fish[0].X
	scene.Paused = true
	scene.Step()
	if scene.Fish[0].X != before {
		t.Fatal("paused scene moved")
	}
	scene.Resize(12, 6)
	if scene.Fish[0].X > 9 || len(scene.Render(true)) != 6 {
		t.Fatal("resize did not clamp or render")
	}
}

func TestFoodAttractsFishAndParticlesAreBounded(t *testing.T) {
	scene := New(30, 10, 2)
	scene.Fish = []Fish{{X: 10, Y: 3, DX: -1, TurnAfter: 100}}
	scene.Food = []Particle{{X: 20, Y: 4}}
	scene.Step()
	if scene.Fish[0].DX <= 0 {
		t.Fatal("fish did not turn toward food")
	}
	for range MaxFood + 10 {
		scene.DropFood()
	}
	for range MaxFish + 10 {
		scene.AddFish()
	}
	if len(scene.Food) > MaxFood || len(scene.Fish) > MaxFish {
		t.Fatal("scene limits exceeded")
	}
}
