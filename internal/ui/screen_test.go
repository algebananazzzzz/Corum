package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

func TestScreenRendersOdysseyStyleFullscreenFrame(t *testing.T) {
	value := ""
	model := NewScreen("Corum Setup", huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Workspace timezone").Value(&value),
	)))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := updated.(*Screen).View()

	if !view.AltScreen {
		t.Fatal("screen must use the alternate terminal buffer")
	}
	if got := lipgloss.Width(view.Content); got != 100 {
		t.Fatalf("frame width = %d, want 100\n%s", got, view.Content)
	}
	if got := lipgloss.Height(view.Content); got != 30 {
		t.Fatalf("frame height = %d, want 30\n%s", got, view.Content)
	}
	for _, want := range []string{"Corum Setup", "Workspace timezone", "////"} {
		if !strings.Contains(view.Content, want) {
			t.Fatalf("frame missing %q:\n%s", want, view.Content)
		}
	}
}

func TestInitScreenKeepsTheWizardInOneFullscreenForm(t *testing.T) {
	screen, _ := newInitScreen("/tmp/corum")
	_ = screen.Init()
	view := screen.View()

	for _, want := range []string{"Vault path", "Workspace timezone", "Asia/Singapore", "Academic term", "AY2026/27 Semester 1", "AY2026/27 Semester 2", "Canvas URL", "Enable wiki authoring?", "Create this vault?"} {
		if !strings.Contains(view.Content, want) {
			t.Fatalf("init form missing %q:\n%s", want, view.Content)
		}
	}
}

func TestInitScreenAlignsConfirmationButtons(t *testing.T) {
	screen, _ := newInitScreen("/tmp/corum")
	_ = screen.Init()
	view := screen.View().Content

	var positions []int
	for _, line := range strings.Split(view, "\n") {
		if index := strings.Index(line, "Yes"); index >= 0 {
			positions = append(positions, index)
		}
	}
	if len(positions) != 2 {
		t.Fatalf("confirmation rows = %d, want 2:\n%s", len(positions), view)
	}
	if positions[0] != positions[1] {
		t.Fatalf("confirmation buttons start at columns %d and %d, want alignment:\n%s", positions[0], positions[1], view)
	}
}

func TestErrorNoticeShowsTheCauseAndOneCloseButton(t *testing.T) {
	screen := NewScreen("Corum Setup", newNoticeForm("Could not initialize vault.", "permission denied", "Close"))
	_ = screen.Init()
	view := screen.View().Content

	for _, want := range []string{"Could not initialize vault.", "permission denied"} {
		if !strings.Contains(view, want) {
			t.Fatalf("notice missing %q:\n%s", want, view)
		}
	}
	if got := strings.Count(view, "Close"); got != 1 {
		t.Fatalf("Close buttons = %d, want 1:\n%s", got, view)
	}
}

func TestScreenUsesAvailableWidthForCourseChoices(t *testing.T) {
	value := ""
	model := NewScreen("Corum", huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Select current Canvas courses to track for the active academic term").Value(&value),
	)))
	_ = model.Init()
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := updated.(*Screen).View().Content

	if !strings.Contains(view, "Select current Canvas courses to track for the active academic term") {
		t.Fatalf("course choice title wrapped despite available width:\n%s", view)
	}
}

func TestSuccessNoticeHasOneDoneButton(t *testing.T) {
	screen := NewScreen("Corum Setup", newNoticeForm("Vault initialized.", "", "Done"))
	_ = screen.Init()
	view := screen.View().Content

	if got := strings.Count(view, "Done"); got != 1 {
		t.Fatalf("Done buttons = %d, want 1:\n%s", got, view)
	}
}

func TestLoadingScreenUsesTheFullscreenFrame(t *testing.T) {
	model := newLoadingScreen("Canvas Authentication", "Loading Canvas courses…", context.Background(), func(context.Context) error { return nil }, func() {})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := updated.(*loadingScreen).View()

	if !view.AltScreen || !strings.Contains(view.Content, "Loading Canvas courses…") {
		t.Fatalf("loading screen is not fullscreen:\n%s", view.Content)
	}
}
