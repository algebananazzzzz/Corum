package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/algebananazzzzz/Corum/internal/buildinfo"
	"github.com/algebananazzzzz/Corum/internal/jira"
)

// Run executes the intentionally small Task 1 command surface.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	_ = in
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(out, buildinfo.Version)
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
	fmt.Fprintln(errOut, "usage: corum version | corum jira login|status|logout")
	return 2
}
