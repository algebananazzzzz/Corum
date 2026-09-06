package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/jira"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

type InitDependencies struct {
	Prompts        Prompter
	Assets         fs.FS
	ToolkitVersion string
	Output         io.Writer
	Initialize     func(string, config.Workspace, fs.FS, string) error
	OpenJira       func(context.Context) (JiraSession, error)
	SnapshotAuth   func() (func() error, error)
}

var initializeVault = vault.Initialize

func DefaultInitDependencies(in io.Reader, out io.Writer, assets fs.FS, toolkitVersion string) InitDependencies {
	return InitDependencies{
		Prompts:        NewHuhPrompter(in, out),
		Assets:         assets,
		ToolkitVersion: toolkitVersion,
		Output:         out,
		Initialize:     initializeVault,
		OpenJira: func(ctx context.Context) (JiraSession, error) {
			session, err := jira.Open(ctx, jira.OpenOptions{Interactive: true, Out: out})
			if err != nil {
				return nil, err
			}
			return rovoSelectionSession{session}, nil
		},
		SnapshotAuth: func() (func() error, error) {
			snapshot, err := jira.SnapshotAuth()
			if err != nil {
				return nil, err
			}
			return snapshot.Restore, nil
		},
	}
}

// RunInit collects all setup fields, validates them, and writes only after the
// final confirmation.
func RunInit(ctx context.Context, proposedRoot string, deps InitDependencies) (err error) {
	if deps.Prompts == nil || deps.Initialize == nil {
		return fmt.Errorf("interactive initialization is unavailable")
	}
	root, err := deps.Prompts.Input("Vault path", proposedRoot)
	if err != nil {
		return promptError(err)
	}
	timezone, err := deps.Prompts.Input("Workspace timezone", "Asia/Singapore")
	if err != nil {
		return promptError(err)
	}
	term, err := deps.Prompts.Input("Academic term", "AY2026/27 Semester 1")
	if err != nil {
		return promptError(err)
	}
	canvasURL, err := deps.Prompts.Input("Canvas URL", "https://canvas.example.edu")
	if err != nil {
		return promptError(err)
	}
	wikiEnabled, err := deps.Prompts.Confirm("Enable wiki authoring?", true)
	if err != nil {
		return promptError(err)
	}
	jiraEnabled, err := deps.Prompts.Confirm("Connect Jira?", false)
	if err != nil {
		return promptError(err)
	}
	workspace := config.Workspace{
		Version:   2,
		Workspace: config.WorkspaceDetails{Timezone: timezone, Term: term},
		Canvas:    &config.CanvasWorkspace{URL: canvasURL},
		Calendar:  config.Calendar{Timetable: "Timetable.md", Term: "Term_Calendar.md"},
	}
	if wikiEnabled {
		workspace.Wiki = &config.WikiWorkspace{}
	}
	var restore func() error
	keepAuth := false
	defer func() {
		if restore != nil && !keepAuth {
			if restoreErr := restore(); err == nil && restoreErr != nil {
				err = restoreErr
			}
		}
	}()
	if jiraEnabled {
		if deps.SnapshotAuth == nil || deps.OpenJira == nil {
			return fmt.Errorf("Jira setup is unavailable")
		}
		restore, err = deps.SnapshotAuth()
		if err != nil {
			return err
		}
		session, openErr := deps.OpenJira(ctx)
		if openErr != nil {
			if errors.Is(openErr, ErrCancelled) {
				return ErrCancelled
			}
			return openErr
		}
		defer session.Close()
		selection, selectErr := selectJira(ctx, session, deps.Prompts)
		if selectErr != nil {
			return promptError(selectErr)
		}
		workspace.Jira = &selection
	}
	if err := config.ValidateWorkspace(workspace); err != nil {
		return err
	}
	if deps.Output != nil {
		fmt.Fprintln(deps.Output, initSummary(workspace))
	}
	confirmed, err := deps.Prompts.Confirm("Create this vault?", true)
	if err != nil {
		return promptError(err)
	}
	if !confirmed {
		return ErrCancelled
	}
	if err := deps.Initialize(root, workspace, deps.Assets, deps.ToolkitVersion); err != nil {
		return err
	}
	keepAuth = true
	return nil
}

func initSummary(workspace config.Workspace) string {
	if workspace.Jira != nil {
		return "Configuration ready: Canvas configured; Jira configured; wiki configured as selected."
	}
	return "Configuration ready: Canvas configured; Jira disabled; wiki configured as selected."
}

type LoginDependencies struct {
	Prompts      Prompter
	Output       io.Writer
	OpenJira     func(context.Context) (JiraSession, error)
	SnapshotAuth func() (func() error, error)
	Write        func(string, config.Workspace) error
}

func DefaultLoginDependencies(in io.Reader, out io.Writer) LoginDependencies {
	init := DefaultInitDependencies(in, out, nil, "")
	return LoginDependencies{Prompts: init.Prompts, Output: out, OpenJira: init.OpenJira, SnapshotAuth: init.SnapshotAuth, Write: vault.WriteWorkspace}
}

// RunJiraLogin authenticates and selects a Jira project, then changes only
// corum.yaml after confirmation. The preceding cache is restored on failure.
func RunJiraLogin(ctx context.Context, root string, deps LoginDependencies) (err error) {
	if deps.Prompts == nil || deps.OpenJira == nil || deps.SnapshotAuth == nil || deps.Write == nil {
		return fmt.Errorf("interactive Jira login is unavailable")
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
			if restoreErr := restore(); err == nil && restoreErr != nil {
				err = restoreErr
			}
		}
	}()
	session, err := deps.OpenJira(ctx)
	if err != nil {
		return promptError(err)
	}
	defer session.Close()
	selection, err := selectJira(ctx, session, deps.Prompts)
	if err != nil {
		return promptError(err)
	}
	workspace.Jira = &selection
	if err := config.ValidateWorkspace(workspace); err != nil {
		return err
	}
	if deps.Output != nil {
		fmt.Fprintln(deps.Output, initSummary(workspace))
	}
	confirmed, err := deps.Prompts.Confirm("Save Jira configuration?", true)
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
