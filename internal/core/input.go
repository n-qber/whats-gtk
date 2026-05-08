package core

import "fmt"

// ShortcutAction is a function to be executed when a shortcut is triggered.
type ShortcutAction func()

// InputManager manages keyboard shortcuts and input events.
type InputManager struct {
	shortcuts map[string]ShortcutAction
}

// NewInputManager creates a new InputManager.
func NewInputManager() *InputManager {
	return &InputManager{
		shortcuts: make(map[string]ShortcutAction),
	}
}

// Register adds a new shortcut action.
func (im *InputManager) Register(keyCombo string, action ShortcutAction) {
	im.shortcuts[keyCombo] = action
}

// HandleKeyPressed processes a key press event.
func (im *InputManager) HandleKeyPressed(key string) bool {
	if action, ok := im.shortcuts[key]; ok {
		fmt.Printf("InputManager: Shortcut triggered: %s\n", key)
		action()
		return true
	}
	return false
}
