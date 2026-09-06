// Package ui contains Corum's terminal-only interaction boundary.
package ui

import (
	"context"
	"errors"
	"io"
	"os"

	"charm.land/huh/v2"
)

// ErrCancelled is returned for an explicit cancellation, Ctrl-C, or EOF.
var ErrCancelled = errors.New("interactive setup cancelled")

// Choice is a display label paired with a stable selection index.
type Choice struct{ Label string }

// Prompter is deliberately small so orchestration tests do not render a terminal.
type Prompter interface {
	Input(label, defaultValue string) (string, error)
	Password(label, defaultValue string) (string, error)
	Confirm(label string, defaultValue bool) (bool, error)
	Select(label string, choices []Choice) (int, error)
}

// HuhPrompter implements Prompter with Huh. All Huh imports stay in this file.
type HuhPrompter struct {
	In         io.Reader
	Out        io.Writer
	Accessible bool
}

func NewHuhPrompter(in io.Reader, out io.Writer) HuhPrompter {
	return HuhPrompter{In: in, Out: out, Accessible: os.Getenv("ACCESSIBLE") != "" || os.Getenv("CORUM_ACCESSIBLE") != ""}
}

func (p HuhPrompter) Input(label, defaultValue string) (string, error) {
	value := defaultValue
	form := p.form(huh.NewInput().Title(label).Value(&value))
	if err := form.Run(); err != nil {
		return "", promptError(err)
	}
	return value, nil
}

func (p HuhPrompter) Password(label, defaultValue string) (string, error) {
	value := defaultValue
	form := p.form(huh.NewInput().Title(label).Value(&value).EchoMode(huh.EchoModePassword))
	if err := form.Run(); err != nil {
		return "", promptError(err)
	}
	return value, nil
}

func (p HuhPrompter) Confirm(label string, defaultValue bool) (bool, error) {
	value := defaultValue
	form := p.form(huh.NewConfirm().Title(label).Value(&value))
	if err := form.Run(); err != nil {
		return false, promptError(err)
	}
	return value, nil
}

func (p HuhPrompter) Select(label string, choices []Choice) (int, error) {
	if len(choices) == 0 {
		return 0, errors.New("no choices available")
	}
	value := 0
	options := make([]huh.Option[int], len(choices))
	for i, choice := range choices {
		options[i] = huh.NewOption(choice.Label, i)
	}
	form := p.form(huh.NewSelect[int]().Title(label).Options(options...).Value(&value))
	if err := form.Run(); err != nil {
		return 0, promptError(err)
	}
	return value, nil
}

func (p HuhPrompter) form(field huh.Field) *huh.Form {
	form := huh.NewForm(huh.NewGroup(field)).WithAccessible(p.Accessible)
	if p.In != nil {
		form = form.WithInput(p.In)
	}
	if p.Out != nil {
		form = form.WithOutput(p.Out)
	}
	return form
}

func promptError(err error) error {
	if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return ErrCancelled
	}
	return err
}

// NonTTYGuidance is stable so automation receives actionable, noninteractive help.
func NonTTYGuidance(command string) string {
	if command == "init" {
		return "interactive init requires a terminal; use corum init --defaults PATH"
	}
	return "interactive Jira login requires a terminal"
}
