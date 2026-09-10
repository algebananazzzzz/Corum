// Package ui contains Corum's terminal-only interaction boundary.
package ui

import (
	"context"
	"errors"
	"io"
	"os"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// ErrCancelled is returned for an explicit cancellation, Ctrl-C, or EOF.
var ErrCancelled = errors.New("interactive setup cancelled")

// Choice is a display label paired with a stable selection index.
type Choice struct {
	Label    string
	Selected bool
}

// Prompter is deliberately small so orchestration tests do not render a terminal.
type Prompter interface {
	Input(label, defaultValue string) (string, error)
	Password(label, defaultValue string) (string, error)
	Confirm(label string, defaultValue bool) (bool, error)
	Select(label string, choices []Choice) (int, error)
	MultiSelect(label string, choices []Choice) ([]int, error)
	ConfigureServices(current ServiceSettings) (ServiceSettings, error)
}

// HuhPrompter implements Prompter with Huh.
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
	if err := p.run(huh.NewInput().Title(label).Value(&value)); err != nil {
		return "", promptError(err)
	}
	return value, nil
}

func (p HuhPrompter) Password(label, defaultValue string) (string, error) {
	value := defaultValue
	if err := p.run(huh.NewInput().Title(label).Value(&value).EchoMode(huh.EchoModePassword)); err != nil {
		return "", promptError(err)
	}
	return value, nil
}

func (p HuhPrompter) Confirm(label string, defaultValue bool) (bool, error) {
	value := defaultValue
	if err := p.run(huh.NewConfirm().Title(label).WithButtonAlignment(lipgloss.Left).Value(&value)); err != nil {
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
	if err := p.run(huh.NewSelect[int]().Title(label).Options(options...).Value(&value)); err != nil {
		return 0, promptError(err)
	}
	return value, nil
}

func (p HuhPrompter) MultiSelect(label string, choices []Choice) ([]int, error) {
	if len(choices) == 0 {
		return []int{}, nil
	}
	value := make([]int, 0, len(choices))
	options := make([]huh.Option[int], len(choices))
	for i, choice := range choices {
		options[i] = huh.NewOption(choice.Label, i).Selected(choice.Selected)
		if choice.Selected {
			value = append(value, i)
		}
	}
	field := huh.NewMultiSelect[int]().Title(label).Options(options...).Value(&value).Filterable(true)
	if err := p.run(field); err != nil {
		return nil, promptError(err)
	}
	return value, nil
}

func (p HuhPrompter) run(field huh.Field) error {
	form := huh.NewForm(huh.NewGroup(field)).WithAccessible(p.Accessible)
	return RunScreen("Corum", form, p.In, p.Out)
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

// ServiceSettings holds the optional features selected on the configuration page.
type ServiceSettings struct {
	Integration string
}

func newServicesForm(settings *ServiceSettings) *huh.Form {
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Jira Integration").Options(
			huh.NewOption("None", "none"),
			huh.NewOption("Jira", "jira"),
		).Value(&settings.Integration),
	))
}

func (p HuhPrompter) ConfigureServices(current ServiceSettings) (ServiceSettings, error) {
	form := newServicesForm(&current).WithAccessible(p.Accessible)
	if err := RunScreen("Corum Configuration", form, p.In, p.Out); err != nil {
		return ServiceSettings{}, promptError(err)
	}
	return current, nil
}
