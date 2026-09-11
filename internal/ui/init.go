package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/jira"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

var initializeVault = vault.Initialize

type initAnswers struct {
	root      string
	timezone  string
	term      string
	canvasURL string
	confirmed bool
}

func (a initAnswers) workspace() config.Workspace {
	workspace := config.Workspace{
		Workspace: config.WorkspaceDetails{Timezone: a.timezone, Term: a.term},
		Canvas:    &config.CanvasWorkspace{URL: a.canvasURL},
	}
	return workspace
}

func newInitScreen(root string) (*Screen, *initAnswers) {
	answers := &initAnswers{
		root: root, timezone: "Asia/Singapore", term: "AY2026/27 Semester 1",
		canvasURL: "https://canvas.nus.edu.sg", confirmed: true,
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Vault path").Description("Folder where Corum will store your courses and notes.").Validate(nonblank).Value(&answers.root),
		huh.NewSelect[string]().Title("Workspace timezone").Options(huh.NewOption("Asia/Singapore", "Asia/Singapore")).Value(&answers.timezone),
		huh.NewSelect[string]().Title("Academic term").Description("Choose the semester you’re organizing.").Options(
			huh.NewOption("AY2026/27 Semester 1", "AY2026/27 Semester 1"),
			huh.NewOption("AY2026/27 Semester 2", "AY2026/27 Semester 2"),
		).Value(&answers.term),
		huh.NewInput().Title("Canvas URL").Description("Your university’s Canvas address.").Value(&answers.canvasURL),
		huh.NewConfirm().Title("Create this vault?").Validate(func(confirmed bool) error {
			if !confirmed {
				return errors.New("confirmation is required")
			}
			return config.ValidateWorkspace(answers.workspace())
		}).WithButtonAlignment(lipgloss.Left).Value(&answers.confirmed),
	))
	return NewScreen("Corum Setup", form), answers
}

func nonblank(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("required")
	}
	return nil
}

func RunInitFullscreen(root string, assets fs.FS, in io.Reader, out io.Writer) error {
	screen, answers := newInitScreen(root)
	if err := RunScreen("Corum Setup", screen.form, in, out); err != nil {
		return err
	}
	if !answers.confirmed {
		return ErrCancelled
	}
	workspace := answers.workspace()
	if err := config.ValidateWorkspace(workspace); err != nil {
		return err
	}
	return initializeVault(answers.root, workspace, assets)
}

// JiraAuthDependencies drives the interactive Jira authentication flow.
type JiraAuthDependencies struct {
	Prompts           Prompter
	OpenJira          func(context.Context) (JiraSession, error)
	SnapshotAuth      func() (func() error, error)
	Write             func(string, config.Workspace) error
	Loading           LoadingRunner
	AuthorizationURLs <-chan string
}

func DefaultJiraAuthDependencies(in io.Reader, out io.Writer, root string) JiraAuthDependencies {
	urls := make(chan string, 1)
	return JiraAuthDependencies{
		Prompts: NewHuhPrompter(in, out),
		OpenJira: func(ctx context.Context) (JiraSession, error) {
			session, err := jira.Open(ctx, jira.OpenOptions{Interactive: true, ForceReauth: true, CachePath: jiraProjectCachePath(root), Out: io.Discard, AuthorizationURL: func(url string) {
				select {
				case urls <- url:
				default:
				}
			}})
			if err != nil {
				return nil, err
			}
			return rovoSelectionSession{session}, nil
		},
		SnapshotAuth: func() (func() error, error) {
			snapshot, err := jira.SnapshotAuthFor(root)
			if err != nil {
				return nil, err
			}
			return snapshot.Restore, nil
		},
		Write: vault.WriteWorkspace,
		Loading: func(ctx context.Context, message string, task func(context.Context) error) error {
			return runLoading(ctx, "Jira Authentication", message, in, out, task)
		},
		AuthorizationURLs: urls,
	}
}

func jiraProjectCachePath(root string) string {
	path, _ := jira.AuthCachePathFor(root)
	return path
}

// RunJiraAuth starts fresh OAuth and confirms a Jira project before saving configuration.
// The previous credential cache is restored on failure.
func RunJiraAuth(ctx context.Context, root string, deps JiraAuthDependencies) (err error) {
	if deps.Prompts == nil || deps.OpenJira == nil || deps.SnapshotAuth == nil || deps.Write == nil {
		return fmt.Errorf("interactive Jira authentication is unavailable")
	}
	workspace, err := config.LoadWorkspace(root)
	if err != nil {
		return err
	}
	restore, err := deps.SnapshotAuth()
	if err != nil {
		return err
	}
	keepAuth := false
	defer func() {
		if !keepAuth {
			restoreErr := restore()
			if err != nil && restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("restore previous Jira authentication: %w", restoreErr))
			} else if err == nil {
				err = restoreErr
			}
		}
	}()
	var session JiraSession
	err = runLoadingTask(deps.Loading, ctx, "Complete Jira authentication in your browser…", func(ctx context.Context) error {
		var openErr error
		session, openErr = deps.OpenJira(ctx)
		return openErr
	})
	if err != nil {
		return promptError(err)
	}
	defer session.Close()
	selection, err := selectJiraWithLoading(ctx, session, deps.Prompts, deps.Loading)
	if err != nil {
		return promptError(err)
	}
	workspace.Jira = &selection
	if err := config.ValidateWorkspace(workspace); err != nil {
		return err
	}
	confirmed, err := deps.Prompts.Confirm("Jira project "+selection.Project+" selected.\n\nSave this configuration?", true)
	if err != nil {
		return promptError(err)
	}
	if !confirmed {
		return ErrCancelled
	}
	if err := deps.Write(root, workspace); err != nil {
		return err
	}
	keepAuth = true
	return nil
}
