package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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

func TestMappingPaneFitsWideAndNarrowTerminals(t *testing.T) {
	for _, width := range []int{110, 65} {
		value := ""
		s := NewScreen("Course mapping", huh.NewForm(huh.NewGroup(huh.NewInput().Title("Destination name").Value(&value))))
		s.sidebar = "CS101 → Existing\nCS102 → Unmapped"
		s.Init()
		s.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		view := ansi.Strip(s.View().Content)
		if !strings.Contains(view, "Destination name") {
			t.Fatal(view)
		}
		if strings.Contains(view, "CS101") != (width >= 85) {
			t.Fatalf("width %d: %s", width, view)
		}
		if lipgloss.Width(view) > width {
			t.Fatalf("overflow at %d: %d", width, lipgloss.Width(view))
		}
	}
}

func TestInitScreenKeepsTheWizardInOneFullscreenForm(t *testing.T) {
	screen, _ := newInitScreen("/tmp/corum")
	_ = screen.Init()
	view := screen.View()

	for _, want := range []string{"Vault path", "Workspace timezone", "Asia/Singapore", "Academic term", "AY2026/27 Semester 1", "AY2026/27 Semester 2", "Canvas URL", "https://canvas.nus.edu.sg", "Create this vault?"} {
		if !strings.Contains(view.Content, want) {
			t.Fatalf("init form missing %q:\n%s", want, view.Content)
		}
	}
	if strings.Contains(view.Content, "Enable wiki authoring?") {
		t.Fatalf("init form still configures wiki:\n%s", view.Content)
	}
}

func TestInitScreenAlignsConfirmationButtons(t *testing.T) {
	screen, _ := newInitScreen("/tmp/corum")
	_ = screen.Init()
	view := ansi.Strip(screen.View().Content)

	buttonStart := -1
	fieldStart := -1
	for _, line := range strings.Split(view, "\n") {
		if index := strings.Index(line, "Yes"); index >= 0 {
			if buttonStart >= 0 {
				t.Fatalf("more than one confirmation row:\n%s", view)
			}
			buttonStart = index
		}
		if index := strings.Index(line, "> https://canvas.nus.edu.sg"); index >= 0 {
			fieldStart = index
		}
	}
	if buttonStart < 0 || fieldStart < 0 {
		t.Fatalf("could not locate confirmation button and Canvas field:\n%s", view)
	}
	// The first two columns of a Huh button are its visual padding. Its outer
	// edge must align with ordinary field controls rather than being centered.
	if buttonStart != fieldStart+2 {
		t.Fatalf("confirmation button text starts at column %d, want %d for a left-aligned button:\n%s", buttonStart, fieldStart+2, view)
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
