package ui

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"sort"

	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
)

// CanvasAuthDependencies drives the interactive Canvas authentication flow.
type CanvasAuthDependencies struct {
	Prompts Prompter
	Output  io.Writer
	Root    string
	Load    func(config.Workspace, string) (*canvas.Client, error)
	Save    func(string) error
	Courses func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error)
}

func DefaultCanvasAuthDependencies(in io.Reader, out io.Writer, root string) CanvasAuthDependencies {
	return CanvasAuthDependencies{
		Prompts: NewHuhPrompter(in, out),
		Output:  out,
		Root:    root,
		Load: func(ws config.Workspace, root string) (*canvas.Client, error) {
			origin := ""
			if ws.Canvas != nil {
				origin = ws.Canvas.URL
			}
			return canvas.NewClientFromEnvironment(origin, root)
		},
		Save: func(token string) error { return canvas.SaveCredential(root, token) },
		Courses: func(ctx context.Context, client *canvas.Client) ([]canvas.CourseInfo, error) {
			return client.Courses(ctx)
		},
	}
}

// RunCanvasAuth stores a Canvas token (prompting only when none is stored) and
// proves it works by listing the accessible courses.
func RunCanvasAuth(ctx context.Context, deps CanvasAuthDependencies) (err error) {
	if deps.Prompts == nil || deps.Save == nil || deps.Courses == nil {
		return fmt.Errorf("interactive Canvas authentication is unavailable")
	}
	workspace, err := config.LoadWorkspace(deps.Root)
	if err != nil {
		return err
	}
	if workspace.Canvas == nil {
		return fmt.Errorf("Canvas is not configured in this vault")
	}
	token, _ := canvas.LoadCredential(deps.Root)
	if token == "" {
		token, err = deps.Prompts.Password("Canvas API token", "")
		if err != nil {
			return promptError(err)
		}
		if token == "" {
			return ErrCancelled
		}
		if err := deps.Save(token); err != nil {
			return err
		}
	}
	client, err := deps.Load(workspace, deps.Root)
	if err != nil {
		return err
	}
	courses, err := deps.Courses(ctx, client)
	if err != nil {
		return fmt.Errorf("Canvas rejected the stored token: %w", err)
	}
	if deps.Output != nil {
		printCourses(deps.Output, courses)
	}
	return nil
}

func printCourses(out io.Writer, courses []canvas.CourseInfo) {
	sort.Slice(courses, func(i, j int) bool {
		if courses[i].CourseCode != courses[j].CourseCode {
			return courses[i].CourseCode < courses[j].CourseCode
		}
		return courses[i].Name < courses[j].Name
	})
	if len(courses) == 0 {
		fmt.Fprintln(out, "Canvas is authenticated, but no courses are visible to this token.")
		return
	}
	fmt.Fprintln(out, "Authenticated courses:")
	for _, course := range courses {
		code := course.CourseCode
		if code == "" {
			code = "-"
		}
		fmt.Fprintf(out, "  %s\t%s\t(id %s)\n", code, course.Name, course.ID)
	}
	fmt.Fprintln(out, "Use the id when creating courses/<CODE>/course.yaml.")
}

// AuthDependencies drives the combined interactive authentication flow.
type AuthDependencies struct {
	Prompts Prompter
	Output  io.Writer
	Assets  fs.FS
	Toolkit string
	Canvas  CanvasAuthDependencies
	Jira    JiraAuthDependencies
	HasJira func(config.Workspace) bool
}

func DefaultAuthDependencies(in io.Reader, out io.Writer, root string, assets fs.FS, toolkit string) AuthDependencies {
	return AuthDependencies{
		Prompts: NewHuhPrompter(in, out),
		Output:  out,
		Assets:  assets,
		Toolkit: toolkit,
		Canvas:  DefaultCanvasAuthDependencies(in, out, root),
		Jira:    DefaultJiraAuthDependencies(in, out, root),
		HasJira: func(ws config.Workspace) bool { return ws.Jira != nil },
	}
}

// RunAuth authenticates every enabled service in the vault: Canvas first, then
// Jira when the workspace has a jira block. A failed or cancelled service
// stops the flow and restores that service's previous state.
func RunAuth(ctx context.Context, root string, deps AuthDependencies) (err error) {
	if deps.Prompts == nil || deps.Output == nil {
		return fmt.Errorf("interactive authentication is unavailable")
	}
	workspace, err := config.LoadWorkspace(root)
	if err != nil {
		return err
	}
	if workspace.Canvas != nil {
		if err := RunCanvasAuth(ctx, deps.Canvas); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(deps.Output, "Canvas is not configured; skipping.")
	}
	if deps.HasJira(workspace) {
		if err := RunJiraAuth(ctx, root, deps.Jira); err != nil {
			return err
		}
	} else {
		enable, err := deps.Prompts.Confirm("Enable Jira now?", false)
		if err != nil {
			return promptError(err)
		}
		if !enable {
			fmt.Fprintln(deps.Output, "Jira skipped; run corum auth jira later to enable it.")
		} else if err := RunJiraAuth(ctx, root, deps.Jira); err != nil {
			return err
		}
	}
	fmt.Fprintln(deps.Output, "Authentication complete.")
	return nil
}
