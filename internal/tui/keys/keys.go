// Package keys defines semantic keyboard navigation predicates shared across AIAI TUIs.
package keys

// IsUp reports whether key moves selection or scroll upward.
func IsUp(key string) bool {
	return key == "up" || key == "k"
}

// IsDown reports whether key moves selection or scroll downward.
func IsDown(key string) bool {
	return key == "down" || key == "j"
}

// IsLeft reports whether key adjusts a horizontal control or cursor leftward.
func IsLeft(key string) bool {
	return key == "left" || key == "h"
}

// IsRight reports whether key adjusts a horizontal control or cursor rightward.
func IsRight(key string) bool {
	return key == "right" || key == "l"
}

// IsNext reports whether key advances focus to the next field or pane.
func IsNext(key string) bool {
	return key == "tab"
}

// IsPrev reports whether key moves focus back to the previous field or pane.
func IsPrev(key string) bool {
	return key == "shift+tab"
}

// IsConfirm reports whether key opens an item or confirms the current action.
func IsConfirm(key string) bool {
	return key == "enter"
}

// IsToggle reports whether key toggles the currently focused option in place.
func IsToggle(key string) bool {
	return key == "space" || key == " "
}

// IsBack reports whether key navigates back or dismisses the current view.
// When allowQuit is true, q acts as a secondary back/quit shortcut outside text inputs.
func IsBack(key string, allowQuit bool) bool {
	return key == "esc" || (allowQuit && key == "q")
}
