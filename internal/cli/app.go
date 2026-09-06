package cli

import (
	"context"
	"fmt"
	"io"

	corum "github.com/algebananazzzzz/Corum"
	"github.com/algebananazzzzz/Corum/internal/buildinfo"
	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/jira"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

// Run executes the intentionally small Task 1 command surface.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	_ = in
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(out, buildinfo.Version)
		return 0
	}
	if root, ok := defaultInitRoot(args); ok {
		if err := vault.Initialize(root, defaultWorkspace(), corum.Assets, buildinfo.Version); err != nil {
			fmt.Fprintln(errOut, "could not initialize vault")
			return 1
		}
		fmt.Fprintln(out, "vault initialized")
		return 0
	}
	if len(args) == 2 && args[0] == "doctor" {
		_, courses, err := vault.Validate(args[1])
		if err != nil {
			fmt.Fprintln(errOut, "vault validation failed")
			return 1
		}
		fmt.Fprintf(out, "doctor: %d courses\n", len(courses))
		return 0
	}
	if len(args) == 2 && args[0] == "jira" && args[1] == "logout" {
		removed, err := jira.ClearAuth()
		if err != nil {
			fmt.Fprintln(errOut, "could not clear Jira authentication")
			return 1
		}
		if removed {
			fmt.Fprintln(out, "Jira authentication cleared")
		} else {
			fmt.Fprintln(out, "Jira is already logged out")
		}
		return 0
	}
	if len(args) == 2 && args[0] == "jira" && (args[1] == "login" || args[1] == "status") {
		interactive := args[1] == "login"
		session, err := jira.Open(ctx, jira.OpenOptions{Interactive: interactive, Out: errOut})
		if err != nil {
			if err == jira.LoginRequired {
				fmt.Fprintln(errOut, "Jira session is missing or revoked; run corum jira login")
			} else {
				fmt.Fprintln(errOut, "could not connect to Atlassian")
			}
			return 1
		}
		defer session.Close()
		account, err := session.UserInfo(ctx)
		if err != nil {
			fmt.Fprintln(errOut, "could not read Atlassian account")
			return 1
		}
		label := account.DisplayName
		if label == "" {
			label = account.AccountID
		}
		if label == "" {
			fmt.Fprintln(errOut, "could not read Atlassian account")
			return 1
		}
		fmt.Fprintln(out, label)
		return 0
	}
	fmt.Fprintln(errOut, "usage: corum init --defaults PATH | corum doctor PATH | corum version | corum jira login|status|logout")
	return 2
}

func defaultInitRoot(args []string) (string, bool) {
	if len(args) != 3 || args[0] != "init" {
		return "", false
	}
	if args[1] == "--defaults" {
		return args[2], true
	}
	if args[2] == "--defaults" {
		return args[1], true
	}
	return "", false
}

func defaultWorkspace() config.Workspace {
	return config.Workspace{
		Version:   2,
		Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "AY2026/27 Semester 1"},
		Canvas:    &config.CanvasWorkspace{URL: "https://canvas.example.edu"},
		Wiki:      &config.WikiWorkspace{},
		Calendar:  config.Calendar{Timetable: "Timetable.md", Term: "Term_Calendar.md"},
	}
}
