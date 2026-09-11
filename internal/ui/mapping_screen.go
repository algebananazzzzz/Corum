package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"io"
)

func runMappingForm(form *huh.Form, sidebar string, in io.Reader, out io.Writer) error {
	screen := NewScreen("Course mapping", form)
	screen.sidebar = sidebar
	_, err := tea.NewProgram(screen, tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return promptError(err)
	}
	if form.State == huh.StateAborted {
		return ErrCancelled
	}
	return nil
}
