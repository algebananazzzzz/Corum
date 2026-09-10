package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	corum "github.com/algebananazzzzz/Corum"
	"github.com/algebananazzzzz/Corum/internal/buildinfo"
	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/jira"
	"github.com/algebananazzzzz/Corum/internal/ui"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

var (
	openJiraSession = jira.Open
	makeJiraClient  = jira.NewJiraClient
	syncCanvas      = canvas.Sync
)

// Run dispatches the complete local Corum command surface.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 1 && args[0] == "--help" {
		fmt.Fprintln(out, "usage: corum init [PATH] | corum init --defaults PATH | corum doctor [PATH] | corum version | corum toolkit update [PATH] | corum configure [PATH]|jira [PATH]|canvas [PATH] | corum sync COURSE...|--all [--dry-run] [--json] | corum jira sync-epic COURSE")
		return 0
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(out, buildinfo.Version)
		return 0
	}
	if (len(args) == 2 || len(args) == 3) && args[0] == "toolkit" && args[1] == "update" {
		path := "."
		if len(args) == 3 {
			path = args[2]
		}
		root, err := openVault(path)
		if err != nil {
			fmt.Fprintln(errOut, "vault validation failed")
			return 1
		}
		if err := vault.RefreshToolkit(root, corum.Assets); err != nil {
			fmt.Fprintln(errOut, "toolkit update failed")
			return 1
		}
		fmt.Fprintln(out, "toolkit updated")
		return 0
	}
	if (len(args) == 2 && args[0] == "init" && args[1] == "--help") || (len(args) == 3 && args[0] == "init" && args[1] == "--help") {
		fmt.Fprintln(out, "usage: corum init [PATH] | corum init --defaults PATH")
		return 0
	}
	if root, ok := defaultInitRoot(args); ok {
		if err := vault.Initialize(root, defaultWorkspace(), corum.Assets); err != nil {
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
		if err := ui.RunInitFullscreen(root, corum.Assets, in, out); err != nil {
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
	if code, handled := runConfigure(ctx, args, in, out, errOut); handled {
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
	if len(args) == 3 && args[0] == "jira" && args[1] == "sync-epic" && args[2] == "--help" {
		fmt.Fprintln(out, "usage: corum jira sync-epic COURSE")
		return 0
	}
	if code, handled := runJiraSyncEpic(ctx, args, out, errOut); handled {
		return code
	}
	if code, handled := runCanvasSync(ctx, args, out, errOut); handled {
		return code
	}
	fmt.Fprintln(errOut, "usage: corum init [PATH] | corum init --defaults PATH | corum doctor [PATH] | corum version | corum toolkit update [PATH] | corum configure [PATH]|jira [PATH]|canvas [PATH] | corum sync COURSE...|--all [--dry-run] [--json] | corum jira sync-epic COURSE")
	return 2
}

// jiraCacheFor resolves the project-local cache path for a validated vault.
func jiraCacheFor(root string) string {
	path, _ := jira.AuthCachePathFor(root)
	return path
}

// runConfigure handles corum configure [PATH] and the special corum configure jira/canvas
// flows.
func runConfigure(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) (int, bool) {
	if len(args) < 1 || args[0] != "configure" {
		return 0, false
	}
	if len(args) == 2 && args[1] == "--help" {
		fmt.Fprintln(out, "usage: corum configure [PATH] | corum configure jira [PATH] | corum configure canvas [PATH]")
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
	fail := func(title, message string, err error) (int, bool) {
		if err == nil {
			_ = ui.ShowNotice(title, message, in, out)
		} else {
			_ = ui.ShowError(title, message, err, in, out)
		}
		return 1, true
	}
	if target == "jira" {
		if err := jira.ConfigureProjectMCP(root); err != nil {
			return fail("Jira MCP Configuration", "Could not configure project-local Jira MCP clients.", err)
		}
		fmt.Fprintln(out, "Jira MCP configured for Codex and Claude. Authenticate from either client before use.")
		return 0, true
	}
	if !isTerminal(in) {
		fmt.Fprintln(errOut, "interactive authentication requires a terminal")
		return 2, true
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
	default:
		deps := ui.DefaultAuthDependencies(in, out, root)
		if err := ui.RunAuth(ctx, root, deps); err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return fail("Corum Configuration", "Configuration cancelled.", nil)
			}
			return fail("Corum Configuration", "Configuration failed.", err)
		}
		_ = ui.ShowNotice("Corum Configuration", "Configuration complete.", in, out)
		return 0, true
	}
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
		if !dryRun && (workspace.Canvas != nil && course.Canvas != nil) {
			if client == nil {
				constructed, constructErr := canvas.NewClientFromEnvironment(workspace.Canvas.URL, root)
				if constructErr != nil {
					fmt.Fprintln(errOut, constructErr)
					if flushErr := flushSyncResults(jsonOutput, out, results); flushErr != nil {
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
	if err := flushSyncResults(jsonOutput, out, results); err != nil {
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
func flushSyncResults(jsonOutput bool, out io.Writer, results []canvas.StageResult) error {
	if jsonOutput {
		return json.NewEncoder(out).Encode(results)
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

func runJiraSyncEpic(ctx context.Context, args []string, out, errOut io.Writer) (int, bool) {
	if len(args) != 3 || args[0] != "jira" || args[1] != "sync-epic" {
		return 0, false
	}
	root, err := openVault(".")
	if err != nil {
		fmt.Fprintln(errOut, "could not locate vault")
		return 1, true
	}
	workspace, err := config.LoadWorkspace(root)
	if err != nil || workspace.Jira == nil {
		fmt.Fprintln(errOut, "Jira is disabled or misconfigured")
		return 1, true
	}
	course, err := config.LoadCourse(root, args[2])
	if err != nil || course.Jira == nil {
		fmt.Fprintln(errOut, "course has no configured Jira epic")
		return 1, true
	}
	session, err := openJiraSession(ctx, jira.OpenOptions{Interactive: false, CachePath: jiraCacheFor(root), Out: errOut})
	if err != nil {
		fmt.Fprintln(errOut, "could not connect to Atlassian; run corum configure")
		return 1, true
	}
	defer session.Close()
	client, err := makeJiraClient(session, workspace.Jira.CloudID)
	if err != nil {
		fmt.Fprintln(errOut, "invalid Jira configuration")
		return 1, true
	}
	result, err := jira.SyncEpic(ctx, root, course.Code, course.Jira.Epic, jira.SyncOptions{Client: client, Project: workspace.Jira.Project})
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1, true
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(errOut, "could not write Jira sync result")
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
		Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "AY2026/27 Semester 1"},
		Canvas:    &config.CanvasWorkspace{URL: "https://canvas.nus.edu.sg"},
	}
}
