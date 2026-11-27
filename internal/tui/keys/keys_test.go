package keys

import "testing"

func TestSemanticKeyPredicates(t *testing.T) {
	if !IsUp("up") || !IsUp("k") || IsUp("down") {
		t.Fatal("IsUp mismatch")
	}
	if !IsDown("down") || !IsDown("j") || IsDown("up") {
		t.Fatal("IsDown mismatch")
	}
	if !IsLeft("left") || !IsLeft("h") || IsLeft("right") {
		t.Fatal("IsLeft mismatch")
	}
	if !IsRight("right") || !IsRight("l") || IsRight("left") {
		t.Fatal("IsRight mismatch")
	}
	if !IsNext("tab") || IsNext("shift+tab") || !IsPrev("shift+tab") || IsPrev("tab") {
		t.Fatal("focus predicates mismatch")
	}
	if !IsConfirm("enter") || IsConfirm("right") {
		t.Fatal("IsConfirm mismatch")
	}
	if !IsToggle("space") || !IsToggle(" ") || IsToggle("enter") {
		t.Fatal("IsToggle mismatch")
	}
	if !IsBack("esc", false) || IsBack("q", false) || !IsBack("q", true) || IsBack("left", true) {
		t.Fatal("IsBack mismatch")
	}
}
