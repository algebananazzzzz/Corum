package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

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
	openJiraSession                                    = jira.Open
	makeJiraClient                                     = jira.NewJiraClient
	maybeUpdate                                        = update.Maybe
	syncVaultToolkit func(string, fs.FS, string) error = vault.SyncToolkit
	syncCanvas                                         = canvas.Sync
)

// RunProcess performs invocation-time update and toolkit work before dispatching
// the requested command. argv must include the executable name.
func RunProcess(ctx context.Context, argv []string, in io.Reader, out, errOut io.Writer) int {
	if len(argv) == 0 {
		argv = []string{"corum"}
	}
	args := argv[1:]
	fullscreen := isTerminal(in) && fullscreenCommand(args)
	explicitUpdate := len(args) == 1 && args[0] == "update"
	if !explicitUpdate && os.Getenv(update.ReexecEnv) != "1" {
		outcome, err := maybeUpdate(ctx, update.Options{
			Version: buildinfo.Version,
			APIBase: os.Getenv("_CORUM_TEST_UPDATE_API_BASE"),
			Args:    argv,
			Env:     os.Environ(),
		})
		if err != nil && !fullscreen {
			fmt.Fprintf(errOut, "warning: automatic update check failed: %v\n", err)
		} else if outcome.Warning != "" && !fullscreen {
			fmt.Fprintf(errOut, "warning: %s\n", outcome.Warning)
		}
	}
	if buildinfo.Version != "dev" {
		if path, ok := commandVaultPath(args); ok {
			root, err := filepath.Abs(path)
			if err == nil {
				if _, _, validateErr := vault.Validate(root); validateErr == nil {
					if syncErr := syncVaultToolkit(root, corum.Assets, buildinfo.Version); syncErr != nil && !fullscreen {
						fmt.Fprintf(errOut, "warning: toolkit update failed for %s: %v\n", root, syncErr)
					}
				}
			}
		}
	}
	return Run(ctx, args, in, out, errOut)
}

// Run dispatches the complete local Corum command surface.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 1 && args[0] == "--help" {
		fmt.Fprintln(out, "usage: corum init [PATH] | corum init --defaults PATH | corum doctor [PATH] | corum version | corum update | corum auth [PATH]|jira [PATH]|canvas [PATH] | corum sync COURSE...|--all [--dry-run] [--json] | corum jira status [PATH] | corum jira logout [PATH] | corum jira apply COURSE [--dry-run]")
		return 0
	}
	if len(args) == 1 && args[0] == "update" {
		outcome, err := update.Run(ctx, update.Options{
			Version: buildinfo.Version,
			APIBase: os.Getenv("_CORUM_TEST_UPDATE_API_BASE"),
		})
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
	if len(args) == 1 && args[0] == update.ContinuationArg {
		// The updated binary re-executed itself after an explicit update.
		// Report the result without another metadata request.
		fmt.Fprintf(out, "corum updated to %s\n", withV(buildinfo.Version))
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
		if err := ui.RunInitFullscreen(root, corum.Assets, buildinfo.Version, in, out); err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				_ = ui.ShowNotice("Corum Setup", "Setup cancelled.", in, out)
				return 1
			}
			_ = ui.ShowError("Corum Setup", "Could not initialize vault.", err, in, out)
			return 1
		}
		_ = ui.ShowNotice("Corum Setup", "Vault initialized.", in, out)
		return 0
	}
	if code, handled := runAuth(ctx, args, in, out, errOut); handled {
		return code
	}
	if len(args) == 1 || len(args) == 2 {
		if args[0] == "doctor" {
			path := "."
			if len(args) == 2 {
				path = args[1]
			}
			if _, courses, err := vault.Validate(path); err != nil {
				fmt.Fprintln(errOut, "vault validation failed")
				return 1
			} else {
				fmt.Fprintf(out, "doctor: %d courses\n", len(courses))
				return 0
			}
		}
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
	if (len(args) == 2 || len(args) == 3) && args[0] == "jira" && args[1] == "logout" {
		path := "."
		if len(args) == 3 {
			path = args[2]
		}
		root, err := openVault(path)
		if err != nil {
			fmt.Fprintln(errOut, "vault validation failed")
			return 1
		}
		removed, err := jira.ClearAuthFor(root)
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
	if (len(args) == 2 || len(args) == 3) && args[0] == "jira" && args[1] == "status" {
		path := "."
		if len(args) == 3 {
			path = args[2]
		}
		root, err := openVault(path)
		if err != nil {
			fmt.Fprintln(errOut, "vault validation failed")
			return 1
		}
		session, err := jira.Open(ctx, jira.OpenOptions{Interactive: false, CachePath: jiraCacheFor(root), Out: errOut})
		if err != nil {
			if err == jira.LoginRequired {
				fmt.Fprintln(errOut, "Jira session is missing or revoked; run corum auth jira")
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
	fmt.Fprintln(errOut, "usage: corum init [PATH] | corum init --defaults PATH | corum doctor [PATH] | corum version | corum update | corum auth [PATH]|jira [PATH]|canvas [PATH] | corum sync COURSE...|--all [--dry-run] [--json] | corum jira status [PATH] | corum jira logout [PATH] | corum jira apply COURSE [--dry-run]")
	return 2
}

// jiraCacheFor resolves the project-local cache path for a validated vault.
func jiraCacheFor(root string) string {
	path, _ := jira.AuthCachePathFor(root)
	return path
}

// runAuth handles corum auth [PATH] and the special corum auth jira/canvas
// flows.
func runAuth(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) (int, bool) {
	if len(args) < 1 || args[0] != "auth" {
		return 0, false
	}
	if len(args) == 2 && args[1] == "--help" {
		fmt.Fprintln(out, "usage: corum auth [PATH] | corum auth jira [PATH] | corum auth canvas [PATH]")
		return 0, true
	}
	path := "."
	target := "all"
	if len(args) == 2 {
		switch args[1] {
		case "jira", "canvas":
			target = args[1]
		default:
			path = args[1]
		}
	} else if len(args) == 3 {
		switch args[1] {
		case "jira", "canvas":
			target = args[1]
			path = args[2]
		default:
			return 0, false
		}
	} else if len(args) > 3 {
		return 0, false
	}
	root, err := openVault(path)
	if err != nil {
		fmt.Fprintln(errOut, "vault validation failed")
		return 1, true
	}
	if !isTerminal(in) {
		fmt.Fprintln(errOut, "interactive authentication requires a terminal")
		return 2, true
	}
	fail := func(title, message string, err error) (int, bool) {
		if err == nil {
			_ = ui.ShowNotice(title, message, in, out)
		} else {
			_ = ui.ShowError(title, message, err, in, out)
		}
		return 1, true
	}
	switch target {
	case "canvas":
		deps := ui.DefaultCanvasAuthDependencies(in, out, root)
		if err := ui.RunCanvasAuth(ctx, deps); err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return fail("Canvas Authentication", "Canvas authentication cancelled.", nil)
			}
			return fail("Canvas Authentication", "Canvas authentication failed.", err)
		}
		_ = ui.ShowNotice("Canvas Authentication", "Canvas authenticated.", in, out)
		return 0, true
	case "jira":
		deps := ui.DefaultJiraAuthDependencies(in, out, root)
		if err := ui.RunJiraAuthFullscreen(ctx, root, deps, in, out); err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return fail("Jira Authentication", "Jira authentication cancelled.", nil)
			}
			return fail("Jira Authentication", "Could not configure Jira.", err)
		}
		_ = ui.ShowNotice("Jira Authentication", "Jira configured.", in, out)
		return 0, true
	default:
		deps := ui.DefaultAuthDependencies(in, out, root)
		if err := ui.RunAuth(ctx, root, deps); err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return fail("Corum Authentication", "Authentication cancelled.", nil)
			}
			return fail("Corum Authentication", "Authentication failed.", err)
		}
		_ = ui.ShowNotice("Corum Authentication", "Authentication complete.", in, out)
		return 0, true
	}
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
	root, err := openVault(".")
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
	failures := []string{}
	for _, course := range courses {
		if !dryRun && config.Effective(workspace, course).Canvas {
			if client == nil {
				constructed, constructErr := canvas.NewClientFromEnvironment(workspace.Canvas.URL, root)
				if constructErr != nil {
					fmt.Fprintln(errOut, constructErr)
					if flushErr := flushSyncResults(jsonOutput, out, workspace, courses, results); flushErr != nil {
						fmt.Fprintln(errOut, "could not write sync result")
						return 1, true
					}
					return 1, true
				}
				client = constructed
			}
		}
		result, syncErr := syncCanvas(ctx, root, workspace, course, client, dryRun)
		results = append(results, result)
		if syncErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", course.Code, syncErr))
		}
	}
	if err := flushSyncResults(jsonOutput, out, workspace, courses, results); err != nil {
		fmt.Fprintln(errOut, "could not write sync result")
		return 1, true
	}
	for _, message := range failures {
		fmt.Fprintln(errOut, message)
	}
	if len(failures) > 0 {
		return 1, true
	}
	for _, result := range results {
		if result.Status == "failed" || result.Status == "partial" {
			return 1, true
		}
	}
	return 0, true
}

// flushSyncResults preserves completed course results even when a later course fails.
func flushSyncResults(jsonOutput bool, out io.Writer, workspace config.Workspace, courses []config.Course, results []canvas.StageResult) error {
	if jsonOutput {
		manifests := make([]any, 0, len(results))
		for index, result := range results {
			manifests = append(manifests, result.ManifestOf(workspace, courses[index]))
		}
		return json.NewEncoder(out).Encode(manifests)
	}
	for _, result := range results {
		fmt.Fprintf(out, "%s: %s\n", result.Course, result.Status)
	}
	return nil
}

// openVault centralizes validated vault opening. It resolves an optional path
// (defaulting to the working directory) and validates the vault.
func openVault(path string) (string, error) {
	if path == "" {
		path = "."
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, _, err := vault.Validate(root); err != nil {
		return "", err
	}
	return root, nil
}

// commandVaultPath identifies commands that operate on one project so process
// startup can refresh only that project's toolkit without a global registry.
func commandVaultPath(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	switch args[0] {
	case "doctor":
		if len(args) == 1 {
			return ".", true
		}
		if len(args) == 2 {
			return args[1], true
		}
	case "auth":
		if len(args) == 1 {
			return ".", true
		}
		if len(args) == 2 && args[1] != "--help" {
			if args[1] == "jira" || args[1] == "canvas" {
				return ".", true
			}
			return args[1], true
		}
		if len(args) == 3 && (args[1] == "jira" || args[1] == "canvas") {
			return args[2], true
		}
	case "sync":
		return ".", true
	case "jira":
		if len(args) >= 2 && (args[1] == "apply" || args[1] == "status" || args[1] == "logout") {
			if (args[1] == "status" || args[1] == "logout") && len(args) == 3 {
				return args[2], true
			}
			return ".", true
		}
	}
	return "", false
}

func fullscreenCommand(args []string) bool {
	if _, ok := interactiveInitRoot(args); ok {
		return true
	}
	if len(args) == 0 || args[0] != "auth" {
		return false
	}
	if len(args) == 1 {
		return true
	}
	if len(args) == 2 {
		return args[1] != "--help"
	}
	return len(args) == 3 && (args[1] == "jira" || args[1] == "canvas")
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
	root, err := openVault(".")
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
	session, err := openJiraSession(ctx, jira.OpenOptions{Interactive: false, CachePath: jiraCacheFor(root), Out: errOut})
	if err != nil {
		if errors.Is(err, jira.LoginRequired) {
			fmt.Fprintln(errOut, "Jira session is missing or revoked; run corum auth jira")
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
