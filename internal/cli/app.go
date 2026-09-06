package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	corum "github.com/algebananazzzzz/Corum"
	"github.com/algebananazzzzz/Corum/internal/buildinfo"
	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/jira"
	"github.com/algebananazzzzz/Corum/internal/ui"
	"github.com/algebananazzzzz/Corum/internal/update"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

var (
	openJiraSession                                          = jira.Open
	makeJiraClient                                           = jira.NewJiraClient
	maybeUpdate                                              = update.Maybe
	syncVaultToolkits func(fs.FS, string) []vault.SyncResult = vault.SyncToolkits
)

// RunProcess performs invocation-time update and toolkit work before dispatching
// the requested command. argv must include the executable name.
func RunProcess(ctx context.Context, argv []string, in io.Reader, out, errOut io.Writer) int {
	if len(argv) == 0 {
		argv = []string{"corum"}
	}
	args := argv[1:]
	explicitUpdate := len(args) == 1 && args[0] == "update"
	if !explicitUpdate && os.Getenv(update.ReexecEnv) != "1" {
		outcome, err := maybeUpdate(ctx, update.Options{
			Version: buildinfo.Version,
			Args:    argv,
			Env:     os.Environ(),
		})
		if err != nil {
			fmt.Fprintf(errOut, "warning: automatic update check failed: %v\n", err)
		} else if outcome.Warning != "" {
			fmt.Fprintf(errOut, "warning: %s\n", outcome.Warning)
		}
	}
	if buildinfo.Version != "dev" {
		for _, result := range syncVaultToolkits(corum.Assets, buildinfo.Version) {
			if result.Err == nil {
				continue
			}
			if result.Root == "" {
				fmt.Fprintf(errOut, "warning: toolkit update failed: %v\n", result.Err)
			} else {
				fmt.Fprintf(errOut, "warning: toolkit update failed for %s: %v\n", result.Root, result.Err)
			}
		}
	}
	return Run(ctx, args, in, out, errOut)
}

// Run executes the intentionally small Task 1 command surface.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	_ = in
	if len(args) == 1 && args[0] == "update" {
		outcome, err := update.Run(ctx, update.Options{Version: buildinfo.Version})
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		switch {
		case outcome.Updated:
			fmt.Fprintf(out, "corum updated to %s\n", withV(outcome.Latest))
		case outcome.Skipped != "":
			fmt.Fprintln(out, outcome.Skipped)
		default:
			fmt.Fprintf(out, "corum %s is up to date\n", withV(buildinfo.Version))
		}
		return 0
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(out, buildinfo.Version)
		return 0
	}
	if (len(args) == 2 && args[0] == "init" && args[1] == "--help") || (len(args) == 3 && args[0] == "init" && args[1] == "--help") {
		fmt.Fprintln(out, "usage: corum init [PATH] | corum init --defaults PATH")
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
	if root, ok := interactiveInitRoot(args); ok {
		if !isTerminal(in) {
			fmt.Fprintln(errOut, ui.NonTTYGuidance("init"))
			return 2
		}
		deps := ui.DefaultInitDependencies(in, out, corum.Assets, buildinfo.Version)
		if err := ui.RunInit(ctx, root, deps); err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				fmt.Fprintln(errOut, "setup cancelled")
			} else {
				fmt.Fprintln(errOut, "could not initialize vault")
			}
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
	if len(args) == 3 && args[0] == "jira" && args[1] == "apply" && args[2] == "--help" {
		fmt.Fprintln(out, "usage: corum jira apply COURSE [--dry-run]")
		return 0
	}
	if code, handled := runJiraApply(ctx, args, in, out, errOut); handled {
		return code
	}
	if code, handled := runCanvasSync(ctx, args, out, errOut); handled {
		return code
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
	if len(args) == 3 && args[0] == "jira" && args[1] == "login" && args[2] == "--help" {
		fmt.Fprintln(out, "usage: corum jira login [PATH]")
		return 0
	}
	if len(args) == 3 && args[0] == "jira" && args[1] == "login" {
		if !isTerminal(in) {
			fmt.Fprintln(errOut, ui.NonTTYGuidance("jira login"))
			return 2
		}
		if err := ui.RunJiraLogin(ctx, args[2], ui.DefaultLoginDependencies(in, out)); err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				fmt.Fprintln(errOut, "Jira setup cancelled")
			} else {
				fmt.Fprintln(errOut, "could not configure Jira")
			}
			return 1
		}
		fmt.Fprintln(out, "Jira configured")
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
	fmt.Fprintln(errOut, "usage: corum init [PATH] | corum init --defaults PATH | corum doctor PATH | corum version | corum update | corum sync COURSE...|--all [--dry-run] [--json] | corum jira login [PATH]|status|logout | corum jira apply COURSE [--dry-run]")
	return 2
}

func withV(version string) string {
	if len(version) > 0 && version[0] == 'v' {
		return version
	}
	return "v" + version
}

func runCanvasSync(ctx context.Context, args []string, out, errOut io.Writer) (int, bool) {
	if len(args) == 2 && args[0] == "sync" && args[1] == "--help" {
		fmt.Fprintln(out, "usage: corum sync COURSE...|--all [--dry-run] [--json]")
		return 0, true
	}
	if len(args) < 2 || args[0] != "sync" {
		return 0, false
	}
	dryRun, jsonOutput, all := false, false, false
	codes := []string{}
	for _, arg := range args[1:] {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--json":
			jsonOutput = true
		case "--all":
			all = true
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return 0, false
			}
			codes = append(codes, arg)
		}
	}
	if all == (len(codes) > 0) {
		return 0, false
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(errOut, "could not locate vault")
		return 1, true
	}
	workspace, err := config.LoadWorkspace(root)
	if err != nil {
		fmt.Fprintln(errOut, "could not load workspace configuration")
		return 1, true
	}
	var courses []config.Course
	if all {
		_, courses, err = vault.Validate(root)
		if err != nil {
			fmt.Fprintln(errOut, "vault validation failed")
			return 1, true
		}
	} else {
		seen := map[string]bool{}
		for _, code := range codes {
			if seen[code] {
				continue
			}
			seen[code] = true
			course, loadErr := config.LoadCourse(root, code)
			if loadErr != nil {
				fmt.Fprintln(errOut, "could not load course configuration")
				return 1, true
			}
			courses = append(courses, course)
		}
	}
	results := make([]canvas.StageResult, 0, len(courses))
	var client canvas.SourceClient
	for _, course := range courses {
		if !dryRun && config.Effective(workspace, course).Canvas {
			if client == nil {
				constructed, constructErr := canvas.NewClientFromEnvironment(workspace.Canvas.URL)
				if constructErr != nil {
					fmt.Fprintln(errOut, constructErr)
					return 1, true
				}
				client = constructed
			}
		}
		result, syncErr := canvas.Sync(ctx, root, workspace, course, client, dryRun)
		results = append(results, result)
		if syncErr != nil {
			fmt.Fprintln(errOut, syncErr)
			return 1, true
		}
	}
	if jsonOutput {
		if err := json.NewEncoder(out).Encode(results); err != nil {
			fmt.Fprintln(errOut, "could not write sync result")
			return 1, true
		}
	} else {
		for _, result := range results {
			fmt.Fprintf(out, "%s: %s\n", result.Course, result.Status)
		}
	}
	for _, result := range results {
		if result.Status == "failed" || result.Status == "partial" {
			return 1, true
		}
	}
	return 0, true
}

func runJiraApply(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) (int, bool) {
	if len(args) != 3 && len(args) != 4 {
		return 0, false
	}
	if args[0] != "jira" || args[1] != "apply" {
		return 0, false
	}
	dryRun := len(args) == 4 && args[3] == "--dry-run"
	if len(args) == 4 && !dryRun {
		return 0, false
	}
	if in == nil {
		fmt.Fprintln(errOut, "could not read Jira plan")
		return 1, true
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(errOut, "could not locate vault")
		return 1, true
	}
	workspace, err := config.LoadWorkspace(root)
	if err != nil {
		fmt.Fprintln(errOut, "could not load workspace configuration")
		return 1, true
	}
	course, err := config.LoadCourse(root, args[2])
	if err != nil {
		fmt.Fprintln(errOut, "could not load course configuration")
		return 1, true
	}
	plan, err := jira.DecodePlan(in)
	if err != nil {
		fmt.Fprintln(errOut, "invalid Jira plan")
		return 1, true
	}
	if err := jira.ValidatePlan(workspace, course, plan, !dryRun); err != nil {
		fmt.Fprintln(errOut, err)
		return 1, true
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if dryRun {
		if _, err := jira.Apply(ctx, root, workspace, course, plan, nil, true); err != nil {
			fmt.Fprintln(errOut, err)
			return 1, true
		}
		if err := encoder.Encode(plan); err != nil {
			fmt.Fprintln(errOut, "could not write Jira plan")
			return 1, true
		}
		return 0, true
	}
	session, err := openJiraSession(ctx, jira.OpenOptions{Interactive: false, Out: errOut})
	if err != nil {
		if errors.Is(err, jira.LoginRequired) {
			fmt.Fprintln(errOut, "Jira session is missing or revoked; run corum jira login")
		} else {
			fmt.Fprintln(errOut, "could not connect to Atlassian")
		}
		return 1, true
	}
	defer session.Close()
	client, err := makeJiraClient(session, workspace.Jira.CloudID)
	if err != nil {
		fmt.Fprintln(errOut, "invalid Jira configuration")
		return 1, true
	}
	result, applyErr := jira.Apply(ctx, root, workspace, course, plan, client, false)
	if result.Course != "" {
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintln(errOut, "could not write Jira result")
			return 1, true
		}
	}
	if applyErr != nil || result.Status == "partial" || result.Status == "failed" {
		if result.Course == "" {
			fmt.Fprintln(errOut, applyErr)
		}
		return 1, true
	}
	return 0, true
}

func interactiveInitRoot(args []string) (string, bool) {
	if len(args) == 1 && args[0] == "init" {
		return "corum-vault", true
	}
	if len(args) == 2 && args[0] == "init" && args[1] != "--defaults" {
		return args[1], true
	}
	return "", false
}

func isTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
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
