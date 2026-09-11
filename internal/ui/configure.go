package ui

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/jira"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

type configureUI struct {
	root    string
	in      io.Reader
	out     io.Writer
	prompts Prompter
}

func activeTracker(ws config.Workspace) string {
	if ws.TaskTracker != "" {
		return ws.TaskTracker
	}
	if ws.Jira != nil {
		return "jira"
	}
	return "none"
}

func RunTaskTrackerSettings(ctx context.Context, root string, in io.Reader, out io.Writer) error {
	u := configureUI{root: root, in: in, out: out, prompts: NewHuhPrompter(in, out)}
	return u.tracker(ctx)
}

func configurationChoices(ws config.Workspace, courses []config.Course) []Choice {
	tracker := activeTracker(ws)
	name := map[string]string{"none": "None", "jira": "Jira", "google_tasks": "Google Tasks"}[tracker]
	choices := []Choice{{Label: "Canvas connection"}, {Label: "Tracked courses"}, {Label: "Task tracker · " + name}}
	if tracker != "none" {
		count, total := 0, 0
		for _, c := range courses {
			if c.Canvas == nil {
				continue
			}
			total++
			if tracker == "jira" && c.Jira != nil || tracker == "google_tasks" && c.GoogleTasks != nil {
				count++
			}
		}
		label := "Jira Epic mapping"
		if tracker == "google_tasks" {
			label = "Google Task List mapping"
		}
		choices = append(choices, Choice{Label: fmt.Sprintf("%s · %d/%d courses mapped", label, count, total)})
	}
	return append(choices, Choice{Label: "Done"})
}

// RunConfigure infers setup from saved settings; there is no wizard ledger.
func RunConfigure(ctx context.Context, root string, in io.Reader, out io.Writer) error {
	u := configureUI{root: root, in: in, out: out, prompts: NewHuhPrompter(in, out)}
	ws, _, err := vault.Validate(root)
	if err != nil {
		return err
	}
	if ws.TaskTracker == "" && ws.Jira == nil {
		if _, e := canvas.LoadCredential(root); errors.Is(e, canvas.ErrNoCredential) && ws.Canvas != nil {
			if e := ShowNotice("Connect Canvas", "Open "+ws.Canvas.URL+"/profile/settings → Approved Integrations → New Access Token. Create a token and paste it on the next screen. Corum will verify it and load your courses.", in, out); e != nil {
				return e
			}
		}
		deps := DefaultCanvasAuthDependencies(in, out, root)
		deps.ReuseCredential = true
		if ws.Canvas != nil {
			u.report(RunCanvasAuth(ctx, deps))
		}
		if err := u.chooseTracker(ctx); err != nil {
			u.report(err)
		} else {
			u.report(u.mapping(ctx))
			u.initialSync(ctx)
		}
	}
	for {
		ws, courses, err := vault.Validate(root)
		if err != nil {
			return err
		}
		choices := configurationChoices(ws, courses)
		index, err := withDescription(u.prompts, "Manage your connections, tracked courses, and task destinations.").Select("Configure Corum", choices)
		if errors.Is(err, ErrCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
		if index == len(choices)-1 {
			return nil
		}
		switch index {
		case 0:
			err = u.canvasConnection(ctx)
		case 1:
			deps := DefaultCanvasAuthDependencies(in, out, root)
			deps.ReuseCredential = true
			err = RunCanvasAuth(ctx, deps)
		case 2:
			err = u.tracker(ctx)
		case 3:
			err = u.mapping(ctx)
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
	index, err := withDescription(u.prompts, "Check Canvas access or update the token used to sync courses.").Select("Canvas connection", []Choice{{Label: "Check connection"}, {Label: "Replace API token"}, {Label: "Back"}})
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
		_ = ShowNotice("Canvas connection", "Open "+ws.Canvas.URL+"/profile/settings and create an access token under Approved Integrations. Paste it into the next field.", u.in, u.out)
		token, err := withDescription(u.prompts, "Paste a token from Canvas Settings → Approved Integrations.").Password("Canvas API token", "")
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

func (u configureUI) tracker(ctx context.Context) error {
	ws, err := config.LoadWorkspace(u.root)
	if err != nil {
		return err
	}
	provider := activeTracker(ws)
	if provider == "none" {
		return u.chooseTracker(ctx)
	}
	choices := []Choice{{Label: "Check connection"}, {Label: "Change task tracker"}, {Label: "Sign in again / switch account"}, {Label: "Disconnect task tracker"}}
	if provider == "jira" {
		choices = append(choices, Choice{Label: "Change Jira site / project"})
	}
	choices = append(choices, Choice{Label: "Back"})
	index, err := withDescription(u.prompts, "Manage the account and service used for course tasks.").Select("Task tracker: "+provider, choices)
	if err != nil {
		return err
	}
	switch index {
	case 0:
		if provider == "google_tasks" {
			err = u.checkGoogle(ctx)
		} else {
			var s *jira.RovoSession
			s, err = u.openJira(ctx, false)
			if s != nil {
				defer s.Close()
			}
		}
		if err != nil {
			return err
		}
		return ShowNotice("Task tracker", "Connected. Agent access may require separate authentication in your agent client.", u.in, u.out)
	case 1:
		return u.chooseTracker(ctx)
	case 2:
		if provider == "google_tasks" {
			return u.connectGoogle(ctx, true)
		}
		return u.connectJira(ctx, true)
	case 3:
		ws.TaskTracker = "none"
		if err := vault.WriteWorkspace(u.root, ws); err != nil {
			return err
		}
		return jira.RemoveProjectMCP(u.root)
	case 4:
		if provider != "jira" {
			return nil
		}
		s, err := u.openJira(ctx, false)
		if err != nil {
			return err
		}
		defer s.Close()
		selected, err := selectJiraWithLoading(ctx, rovoSelectionSession{session: s}, u.prompts, nil)
		if err != nil {
			return err
		}
		ws.Jira = &selected
		return vault.WriteWorkspace(u.root, ws)
	}
	return nil
}

func (u configureUI) chooseTracker(ctx context.Context) error {
	index, err := withDescription(u.prompts, "Choose a service now, or connect one later in configuration.").Select("Where would you like to track course tasks?", []Choice{{Label: "Google Tasks — simple lists, visible on Calendar"}, {Label: "Jira — epics and issue workflows"}, {Label: "None / set up later"}, {Label: "Back"}})
	if err != nil {
		return err
	}
	if index == 3 {
		return ErrCancelled
	}
	provider := "none"
	switch index {
	case 0:
		provider = "google_tasks"
		err = u.connectGoogle(ctx, false)
	case 1:
		provider = "jira"
		err = u.connectJira(ctx, false)
	}
	if err != nil {
		return err
	}
	ws, err := config.LoadWorkspace(u.root)
	if err != nil {
		return err
	}
	ws.TaskTracker = provider
	if err := vault.WriteWorkspace(u.root, ws); err != nil {
		return err
	}
	if provider != "jira" {
		return jira.RemoveProjectMCP(u.root)
	}
	return nil
}

func (u configureUI) openJira(ctx context.Context, login bool) (*jira.RovoSession, error) {
	path, err := jira.AuthCachePathFor(u.root)
	if err != nil {
		return nil, err
	}
	return jira.Open(ctx, jira.OpenOptions{CachePath: path, Interactive: login, ForceReauth: login, Out: u.out})
}

func (u configureUI) connectJira(ctx context.Context, force bool) error {
	if !force {
		ws, err := config.LoadWorkspace(u.root)
		if err != nil {
			return err
		}
		if ws.Jira != nil {
			s, err := u.openJira(ctx, false)
			if err == nil {
				s.Close()
				return jira.ConfigureProjectMCP(u.root)
			}
			index, e := withDescription(u.prompts, "Sign in through your browser to reconnect Jira.").Select("Jira needs reconnection", []Choice{{Label: "Sign in again"}, {Label: "Back"}})
			if e != nil {
				return e
			}
			if index != 0 {
				return ErrCancelled
			}
		}
	}
	deps := DefaultJiraAuthDependencies(u.in, u.out, u.root)
	if err := RunJiraAuthFullscreen(ctx, u.root, deps, u.in, u.out); err != nil {
		return err
	}
	return jira.ConfigureProjectMCP(u.root)
}

func (u configureUI) initialSync(ctx context.Context) {
	defer func() {
		u.syncMapped(ctx)
		_ = ShowNotice("Initial sync", "Initial sync finished. Successful captures are saved; any failures were reported. Re-run course sync after correcting a connection or mapping.", u.in, u.out)
	}()
	ws, courses, err := vault.Validate(u.root)
	if err != nil {
		u.report(err)
		return
	}
	for _, course := range courses {
		if course.Canvas == nil {
			continue
		}
		token, err := canvas.LoadCredential(u.root)
		if err == nil && ws.Canvas != nil {
			client, e := canvas.NewClient(ws.Canvas.URL, token)
			err = e
			if e == nil {
				err = runLoading(ctx, "Initial sync", "Capturing "+course.Code+"…", u.in, u.out, func(ctx context.Context) error { _, e := canvas.Sync(ctx, u.root, ws, course, client, false); return e })
			}
		}
		u.report(err)
	}
}
