package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/algebananazzzzz/Corum/internal/config"
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
		huh.NewSelect[string]().Title("Academic term").Options(
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

// RunInitFullscreen runs the whole first-time setup and returns the vault
// path, which the user may have edited in the form. The vault only appears
// once setup succeeds, so a rejected token or a cancel leaves nothing behind.
func RunInitFullscreen(ctx context.Context, root string, assets fs.FS, in io.Reader, out io.Writer) (string, error) {
	screen, answers := newInitScreen(root)
	if err := RunScreen("Corum Setup", screen.form, in, out); err != nil {
		return "", err
	}
	if !answers.confirmed {
		return "", ErrCancelled
	}
	workspace := answers.workspace()
	if err := config.ValidateWorkspace(workspace); err != nil {
		return "", err
	}
	err := stageVault(answers.root, func(staging string) error {
		if err := initializeVault(staging, workspace, assets); err != nil {
			return err
		}
		return RunSetup(ctx, staging, in, out)
	})
	if err != nil {
		return "", err
	}
	return answers.root, nil
}

// stageVault builds a vault in a hidden sibling directory and renames it onto
// target only when build succeeds; otherwise the staging directory is removed.
// Vault files use relative paths, so the rename does not invalidate them.
func stageVault(target string, build func(staging string) error) error {
	target, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
		return fmt.Errorf("refusing to initialize non-empty target %q", target)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(target)+".corum-init-*")
	if err != nil {
		return err
	}
	if err := build(staging); err != nil {
		return errors.Join(err, os.RemoveAll(staging))
	}
	if err := os.Chmod(staging, 0o755); err != nil {
		return errors.Join(err, os.RemoveAll(staging))
	}
	// os.Rename will not replace a directory; os.Remove only deletes the
	// target if it is still empty.
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return errors.Join(err, os.RemoveAll(staging))
	}
	if err := os.Rename(staging, target); err != nil {
		return errors.Join(err, os.RemoveAll(staging))
	}
	return nil
}
