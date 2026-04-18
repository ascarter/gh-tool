package cmd

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
)

// promptKeyMap returns a huh KeyMap customized so that tab / shift+tab move
// the cursor in select and multi-select prompts, and toggle yes/no in confirm
// prompts. Default keys (arrows, j/k, space) still work — this is purely
// additive so users without arrow keys (e.g. HHKB) can navigate with tab.
func promptKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()

	km.Select.Down = key.NewBinding(key.WithKeys("down", "j", "ctrl+j", "ctrl+n", "tab"), key.WithHelp("↓/tab", "down"))
	km.Select.Up = key.NewBinding(key.WithKeys("up", "k", "ctrl+k", "ctrl+p", "shift+tab"), key.WithHelp("↑/shift+tab", "up"))
	km.Select.Next = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select"))

	km.MultiSelect.Down = key.NewBinding(key.WithKeys("down", "j", "ctrl+n", "tab"), key.WithHelp("↓/tab", "down"))
	km.MultiSelect.Up = key.NewBinding(key.WithKeys("up", "k", "ctrl+p", "shift+tab"), key.WithHelp("↑/shift+tab", "up"))
	km.MultiSelect.Next = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm"))

	km.Confirm.Toggle = key.NewBinding(key.WithKeys("h", "l", "left", "right", "tab"), key.WithHelp("←/→/tab", "toggle"))

	return km
}

// runField wraps a single huh field in a Form so we can apply our custom
// keymap. (huh.Run / field.Run reset every field's keymap to the default
// inside NewForm, so per-field WithKeyMap gets clobbered.)
func runField(field huh.Field) error {
	return huh.NewForm(huh.NewGroup(field)).
		WithKeyMap(promptKeyMap()).
		WithShowHelp(false).
		Run()
}

// promptSelect runs a single huh select prompt with the project's keymap.
func promptSelect(title string, options []string, value *string) error {
	opts := make([]huh.Option[string], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, o)
	}
	return runField(huh.NewSelect[string]().
		Title(title).
		Options(opts...).
		Value(value))
}

// promptMultiSelect runs a multi-select with options and an optional set of
// pre-selected values, validating that at least one item is chosen.
func promptMultiSelect(title string, options, defaults []string, value *[]string) error {
	return runMultiSelect(title, options, defaults, value, false)
}

// promptMultiSelectOptional runs a multi-select that permits an empty
// selection — used when the entire group (e.g. completions) is opt-in.
func promptMultiSelectOptional(title string, options, defaults []string, value *[]string) error {
	return runMultiSelect(title, options, defaults, value, true)
}

func runMultiSelect(title string, options, defaults []string, value *[]string, allowEmpty bool) error {
	defSet := make(map[string]bool, len(defaults))
	for _, d := range defaults {
		defSet[d] = true
	}
	opts := make([]huh.Option[string], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, o).Selected(defSet[o])
	}
	field := huh.NewMultiSelect[string]().
		Title(title).
		Options(opts...).
		Value(value)
	if !allowEmpty {
		field = field.Validate(func(v []string) error {
			if len(v) == 0 {
				return errAtLeastOne
			}
			return nil
		})
	}
	return runField(field)
}

// promptConfirm runs a yes/no prompt with the given default and project keymap.
func promptConfirm(title string, defaultYes bool, value *bool) error {
	*value = defaultYes
	return runField(huh.NewConfirm().
		Title(title).
		Value(value))
}

type promptError string

func (e promptError) Error() string { return string(e) }

const errAtLeastOne = promptError("select at least one")
