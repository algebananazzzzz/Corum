package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/algebananazzzzz/Corum/internal/agentmcp"
	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

type configureUI struct {
	root    string
	in      io.Reader
	out     io.Writer
	prompts Prompter
}

var trackerNames = map[string]string{"none": "None", "jira": "Jira", "kaneo": "Kaneo", "google_tasks": "Google Tasks"}

func activeTracker(ws config.Workspace) string {
	if ws.TaskTracker == "" {
		return "none"
	}
	return ws.TaskTracker
}

func RunTaskTrackerSettings(ctx context.Context, root string, in io.Reader, out io.Writer) error {
	u := configureUI{root: root, in: in, out: out, prompts: NewHuhPrompter(in, out)}
	return u.tracker()
}

func configurationChoices(ws config.Workspace) []Choice {
	return []Choice{{Label: "Canvas connection"}, {Label: "Tracked courses"}, {Label: "Task tracker · " + trackerNames[activeTracker(ws)]}, {Label: "Done"}}
}

// RunSetup connects Canvas, tracks courses and selects the tracker for a new
// vault. It captures nothing: the agent's first sync then reports every Canvas
// item as new, so its first plan covers the whole course.
func RunSetup(ctx context.Context, root string, in io.Reader, out io.Writer) error {
	ws, err := config.LoadWorkspace(root)
	if err != nil {
		return err
	}
	if ws.Canvas != nil {
		if err := RunCanvasAuth(ctx, DefaultCanvasAuthDependencies(in, out, root)); err != nil {
			return err
		}
	}
	u := configureUI{root: root, in: in, out: out, prompts: NewHuhPrompter(in, out)}
	return u.tracker()
}

// RunConfigure is the settings menu for changing a vault after init.
func RunConfigure(ctx context.Context, root string, in io.Reader, out io.Writer) error {
	u := configureUI{root: root, in: in, out: out, prompts: NewHuhPrompter(in, out)}
	if _, _, err := vault.Validate(root); err != nil {
		return err
	}
	for {
		ws, _, err := vault.Validate(root)
		if err != nil {
			return err
		}
		choices := configurationChoices(ws)
		index, err := u.prompts.Select("Configure Corum", choices)
		if errors.Is(err, ErrCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
		switch index {
		case 0:
			err = u.canvasConnection(ctx)
		case 1:
			deps := DefaultCanvasAuthDependencies(in, out, root)
			deps.ReuseCredential = true
			err = RunCanvasAuth(ctx, deps)
		case 2:
			err = u.tracker()
		case 3:
			return nil
		default:
			err = fmt.Errorf("invalid configuration selection")
		}
		u.report(err)
	}
}

func (u configureUI) report(err error) {
	if err != nil && !errors.Is(err, ErrCancelled) {
		_ = ShowError("Corum Configuration", "Could not complete this action", err, u.in, u.out)
	}
}

func (u configureUI) canvasConnection(ctx context.Context) error {
	index, err := u.prompts.Select("Canvas connection", []Choice{{Label: "Check connection"}, {Label: "Replace API token"}, {Label: "Back"}})
	if err != nil {
		return err
	}
	if index == 2 {
		return nil
	}
	if index == 1 {
		ws, err := config.LoadWorkspace(u.root)
		if err != nil {
			return err
		}
		if ws.Canvas == nil {
			return fmt.Errorf("Canvas is not configured")
		}
		token, err := withDescription(u.prompts, canvasTokenGuidance(ws.Canvas.URL)).Password("Connect Canvas", "")
		if err != nil {
			return err
		}
		client, err := canvas.NewClient(ws.Canvas.URL, token)
		if err != nil {
			return err
		}
		if _, err = client.Courses(ctx); err != nil {
			return err
		}
		return canvas.SaveCredential(u.root, token)
	}
	ws, err := config.LoadWorkspace(u.root)
	if err != nil {
		return err
	}
	if ws.Canvas == nil {
		return fmt.Errorf("Canvas is not configured")
	}
	token, err := canvas.LoadCredential(u.root)
	if err != nil {
		return err
	}
	client, err := canvas.NewClient(ws.Canvas.URL, token)
	if err != nil {
		return err
	}
	if _, err = client.Courses(ctx); err != nil {
		return err
	}
	return ShowNotice("Canvas connection", "Connected", u.in, u.out)
}

func (u configureUI) tracker() error {
	notice, err := configureTracker(u.root, u.prompts)
	if err != nil || notice == "" {
		return err
	}
	return ShowNotice("Task tracker", notice, u.in, u.out)
}

// configureTracker saves the selected tracker and where it lives, returning
// the follow-up guidance. Corum does not sign in; agents reach the tracker
// through their own MCP or CLI login, and each course code names its epic,
// label or task list.
func configureTracker(root string, prompts Prompter) (string, error) {
	ws, err := config.LoadWorkspace(root)
	if err != nil {
		return "", err
	}
	providers := []string{"kaneo", "jira", "google_tasks", "none"}
	choices := []Choice{
		{Label: "Kaneo"},
		{Label: "Jira"},
		{Label: "Google Tasks"},
		{Label: "None / set up later"},
		{Label: "Back"},
	}
	index, err := withDescription(prompts, "Current: "+trackerNames[activeTracker(ws)]).Select("Which task tracker would you like to use?", choices)
	if err != nil {
		return "", err
	}
	if index < 0 || index > len(providers) {
		return "", fmt.Errorf("invalid task tracker selection")
	}
	if index == len(providers) {
		return "", ErrCancelled
	}
	provider := providers[index]
	notice := ""
	switch provider {
	case "jira":
		current := ""
		if ws.Jira != nil {
			current = ws.Jira.Site
		}
		site, err := trackerURL(prompts, "Jira site URL", "The URL to your Jira site", current)
		if err != nil {
			return "", err
		}
		if ws.Jira == nil || ws.Jira.Site != site {
			ws.Jira = &config.JiraWorkspace{Site: site}
		}
		notice = mcpSetupNotice("Jira", "atlassian-jira")
	case "kaneo":
		current := ""
		if ws.Kaneo != nil {
			current = ws.Kaneo.URL
		}
		url, err := trackerURL(prompts, "Kaneo URL", "The URL to your Kaneo site", current)
		if err != nil {
			return "", err
		}
		if ws.Kaneo == nil || ws.Kaneo.URL != url {
			ws.Kaneo = &config.KaneoWorkspace{URL: url}
		}
		notice = mcpSetupNotice("Kaneo", "kaneo")
	case "google_tasks":
		notice = "To connect Google Tasks:\n" +
			"1. Install the Google Workspace CLI: npm install -g @googleworkspace/cli\n" +
			"2. Create the Google Cloud project and OAuth client (needs the gcloud CLI): gws auth setup\n" +
			"3. Sign in and approve Tasks access in your browser: gws auth login -s tasks\n" +
			"4. Confirm the connection: gws auth status"
	}
	ws.TaskTracker = provider
	if provider != "jira" {
		ws.Jira = nil
	}
	if provider != "kaneo" {
		ws.Kaneo = nil
	}
	if err := config.ValidateWorkspace(ws); err != nil {
		return "", err
	}
	if err := vault.WriteWorkspace(root, ws); err != nil {
		return "", err
	}
	if err := agentmcp.Configure(root, ws); err != nil {
		return "", err
	}
	return notice, nil
}

// trackerURL asks for a tracker origin, keeping the saved one as the default.
// The agent's saved project belongs to that origin, so callers drop it when
// the returned URL differs.
func trackerURL(prompts Prompter, label, description, current string) (string, error) {
	if current == "" {
		current = "https://"
	}
	url, err := withDescription(prompts, description).Input(label, current)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSpace(url), "/"), nil
}

func mcpSetupNotice(name, server string) string {
	return "To connect " + name + ":\n" +
		"1. Start Claude Code or Codex in this vault\n" +
		"2. Approve the " + server + " MCP server when prompted\n" +
		"3. Sign in: run /mcp in Claude Code, or codex mcp login " + server + " in Codex"
}
