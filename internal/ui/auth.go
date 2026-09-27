package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

// CanvasAuthDependencies drives the interactive Canvas authentication flow.
type CanvasAuthDependencies struct {
	ReuseCredential bool
	Prompts         Prompter
	Root            string
	Load            func(config.Workspace, string) (*canvas.Client, error)
	Save            func(string) error
	Clear           func() error
	Courses         func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error)
	Loading         LoadingRunner
}

func DefaultCanvasAuthDependencies(in io.Reader, out io.Writer, root string) CanvasAuthDependencies {
	return CanvasAuthDependencies{
		Prompts: NewHuhPrompter(in, out),
		Root:    root,
		Load: func(ws config.Workspace, token string) (*canvas.Client, error) {
			origin := ""
			if ws.Canvas != nil {
				origin = ws.Canvas.URL
			}
			return canvas.NewClient(origin, token)
		},
		Save:  func(token string) error { return canvas.SaveCredential(root, token) },
		Clear: func() error { _, err := canvas.ClearCredential(root); return err },
		Courses: func(ctx context.Context, client *canvas.Client) ([]canvas.CourseInfo, error) {
			return client.Courses(ctx)
		},
		Loading: func(ctx context.Context, message string, task func(context.Context) error) error {
			return runLoading(ctx, "Canvas Authentication", message, in, out, task)
		},
	}
}

// RunCanvasAuth stores a Canvas token (prompting only when none is stored),
// validates it, and reconciles the selected current courses for this project.
func RunCanvasAuth(ctx context.Context, deps CanvasAuthDependencies) (err error) {
	if deps.Prompts == nil || deps.Load == nil || deps.Save == nil || deps.Clear == nil || deps.Courses == nil {
		return fmt.Errorf("interactive Canvas authentication is unavailable")
	}
	workspace, err := config.LoadWorkspace(deps.Root)
	if err != nil {
		return err
	}
	if workspace.Canvas == nil {
		return fmt.Errorf("Canvas is not configured in this vault")
	}
	storedToken, credentialErr := canvas.LoadCredential(deps.Root)
	if credentialErr != nil && !errors.Is(credentialErr, canvas.ErrNoCredential) {
		return credentialErr
	}
	token := ""
	if deps.ReuseCredential {
		token = storedToken
	}
	// Setup must not continue without a working token, so a rejected or blank
	// token asks again, with the reason, until Canvas accepts one or the user
	// cancels.
	guidance := "Paste a token from Canvas Settings → Approved Integrations."
	var client *canvas.Client
	var courses []canvas.CourseInfo
	for {
		if token == "" {
			token, err = withDescription(deps.Prompts, guidance).Password("Canvas API token", "")
			if err != nil {
				return promptError(err)
			}
			token = strings.TrimSpace(token)
			if token == "" {
				guidance = "A token is required. Paste one from Canvas Settings → Approved Integrations."
				continue
			}
		}
		client, err = deps.Load(workspace, token)
		if err != nil {
			return err
		}
		err = runLoadingTask(deps.Loading, ctx, "Loading Canvas courses…", func(ctx context.Context) error {
			var loadErr error
			courses, loadErr = deps.Courses(ctx, client)
			return loadErr
		})
		if err == nil {
			break
		}
		var httpErr *canvas.HTTPError
		if !errors.As(err, &httpErr) || !httpErr.Unauthorized() {
			return err
		}
		guidance = fmt.Sprintf("Canvas rejected that token (HTTP %d). Corum received %s. Paste a new one from Canvas Settings → Approved Integrations.", httpErr.Status, tokenFingerprint(token))
		token = ""
	}
	newToken := token != storedToken
	current := make([]canvas.CourseInfo, 0, len(courses))
	for _, course := range courses {
		if course.Current && course.ID != "" && (course.CourseCode != "" || course.Name != "") {
			current = append(current, course)
		}
	}
	sortCourses(current)
	if len(current) == 0 {
		if newToken {
			if err := deps.Save(token); err != nil {
				return err
			}
		}
		return nil
	}
	tracked, err := vault.TrackedCanvasIDs(deps.Root)
	if err != nil {
		return err
	}
	choices := courseChoices(current, tracked)
	selected, err := withDescription(deps.Prompts, "Choose the courses to sync into this vault.").MultiSelect("Select current Canvas courses to track", choices)
	if err != nil {
		return promptError(err)
	}
	selectedIDs := make([]string, 0, len(selected))
	for _, index := range selected {
		if index < 0 || index >= len(current) {
			return fmt.Errorf("invalid Canvas course selection")
		}
		selectedIDs = append(selectedIDs, current[index].ID)
	}
	vaultChoices := make([]vault.CanvasCourseChoice, 0, len(current))
	for _, course := range current {
		vaultChoices = append(vaultChoices, vault.CanvasCourseChoice{ID: course.ID, Code: course.CourseCode, Name: course.Name})
	}
	if newToken {
		if err := deps.Save(token); err != nil {
			return err
		}
	}
	_, err = vault.ConfigureCanvasCourses(deps.Root, vaultChoices, selectedIDs)
	if err != nil {
		if newToken {
			return errors.Join(err, deps.Clear())
		}
		return err
	}
	return nil
}

func sortCourses(courses []canvas.CourseInfo) {
	sort.Slice(courses, func(i, j int) bool {
		if courses[i].CourseCode != courses[j].CourseCode {
			return courses[i].CourseCode < courses[j].CourseCode
		}
		return courses[i].Name < courses[j].Name
	})
}

func courseChoices(courses []canvas.CourseInfo, tracked map[string]bool) []Choice {
	width := 0
	for _, course := range courses {
		if len(course.CourseCode) > width {
			width = len(course.CourseCode)
		}
	}
	choices := make([]Choice, 0, len(courses))
	for _, course := range courses {
		code := course.CourseCode
		if code == "" {
			code = "Course"
		}
		label := fmt.Sprintf("%-*s  %s  · ID %s", width, code, course.Name, course.ID)
		choices = append(choices, Choice{Label: strings.TrimSpace(label), Selected: tracked[course.ID]})
	}
	return choices
}

// tokenFingerprint describes a pasted token without revealing it, so a paste
// that arrived truncated or altered is visible to the user.
func tokenFingerprint(token string) string {
	if len(token) <= 12 {
		return fmt.Sprintf("%d characters", len(token))
	}
	return fmt.Sprintf("%d characters, %s…%s", len(token), token[:6], token[len(token)-4:])
}
