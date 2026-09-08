package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/algebananazzzzz/Corum/internal/config"
)

type jiraOption struct {
	label string
	value config.JiraWorkspace
}

type jiraOptionsResult struct {
	options []jiraOption
	err     error
}

type jiraLinkResult struct{ url string }

type jiraAuthScreen struct {
	*Screen
	ctx       context.Context
	cancel    context.CancelFunc
	load      func(context.Context) ([]jiraOption, error)
	links     <-chan string
	save      func(config.JiraWorkspace) error
	options   []jiraOption
	selected  int
	confirmed bool
	link      string
	step      int
	err       error
	saved     bool
}

func RunJiraAuthFullscreen(ctx context.Context, root string, deps JiraAuthDependencies, in io.Reader, out io.Writer) (err error) {
	if deps.OpenJira == nil || deps.SnapshotAuth == nil || deps.Write == nil {
		return errors.New("interactive Jira authentication is unavailable")
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
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	screen := &jiraAuthScreen{
		Screen: newScreen("Jira Authentication"), ctx: taskCtx, cancel: cancel, links: deps.AuthorizationURLs,
		load: func(ctx context.Context) ([]jiraOption, error) {
			session, err := deps.OpenJira(ctx)
			if err != nil {
				return nil, err
			}
			defer session.Close()
			return jiraOptions(ctx, session)
		},
		save: func(selection config.JiraWorkspace) error {
			workspace.Jira = &selection
			if err := config.ValidateWorkspace(workspace); err != nil {
				return err
			}
			return deps.Write(root, workspace)
		},
	}
	model, runErr := tea.NewProgram(screen, tea.WithInput(in), tea.WithOutput(out)).Run()
	if runErr != nil {
		return runErr
	}
	result, ok := model.(*jiraAuthScreen)
	if !ok {
		return errors.New("Jira authentication returned an unexpected model")
	}
	if result.err != nil {
		return result.err
	}
	if !result.saved {
		return ErrCancelled
	}
	keepAuth = true
	return nil
}

func jiraOptions(ctx context.Context, session JiraSession) ([]jiraOption, error) {
	resources, err := session.Resources(ctx)
	if err != nil {
		return nil, err
	}
	options := []jiraOption{}
	for _, resource := range resources {
		if resource.CloudID == "" {
			continue
		}
		projects, err := session.Projects(ctx, resource.CloudID)
		if err != nil {
			return nil, err
		}
		for _, project := range projects {
			if project.Key == "" {
				continue
			}
			site := resource.Name
			if site == "" {
				site = "Jira site"
			}
			name := project.Name
			if name == "" {
				name = project.Key
			}
			options = append(options, jiraOption{label: site + " / " + name + " (" + project.Key + ")", value: config.JiraWorkspace{CloudID: resource.CloudID, Project: project.Key}})
		}
	}
	if len(options) == 0 {
		return nil, errors.New("no accessible Jira projects")
	}
	return options, nil
}

func (s *jiraAuthScreen) Init() tea.Cmd {
	cmds := []tea.Cmd{func() tea.Msg {
		options, err := s.load(s.ctx)
		return jiraOptionsResult{options: options, err: err}
	}, jiraLoadingTick()}
	if s.links != nil {
		cmds = append(cmds, s.waitForLink())
	}
	return tea.Batch(cmds...)
}

func (s *jiraAuthScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		s.width = min(size.Width, screenMaxWidth) - s.style.base.GetHorizontalFrameSize()
		s.height = size.Height
		if s.form != nil {
			s.form = s.form.WithWidth(s.formWidth()).WithHeight(max(s.height-s.style.base.GetVerticalFrameSize()-5, 5))
		}
	}
	switch msg := msg.(type) {
	case jiraOptionsResult:
		if msg.err != nil {
			s.err = msg.err
			return s, tea.Quit
		}
		s.options = msg.options
		s.form = s.selectionForm()
		if s.height > 0 {
			s.form = s.form.WithHeight(max(s.height-s.style.base.GetVerticalFrameSize()-5, 5))
		}
		return s, s.form.Init()
	case jiraLinkResult:
		if msg.url != "" {
			s.link = msg.url
			return s, s.waitForLink()
		}
	case jiraLoadingTickMsg:
		if s.form == nil {
			s.step++
			return s, jiraLoadingTick()
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			s.cancel()
			s.err = ErrCancelled
			return s, tea.Quit
		}
	}
	if s.form == nil {
		return s, nil
	}
	next, cmd := s.form.Update(msg)
	if form, ok := next.(*huh.Form); ok {
		s.form = form
	}
	if s.form.State == huh.StateCompleted {
		if !s.confirmed {
			s.err = ErrCancelled
			return s, tea.Quit
		}
		if s.selected < 0 || s.selected >= len(s.options) {
			s.err = errors.New("invalid Jira project selection")
			return s, tea.Quit
		}
		if err := s.save(s.options[s.selected].value); err != nil {
			s.err = err
			return s, tea.Quit
		}
		s.saved = true
		return s, tea.Quit
	}
	return s, cmd
}

func (s *jiraAuthScreen) View() tea.View {
	if s.form != nil {
		return s.Screen.View()
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	body := "\n" + s.style.header.Render(frames[s.step%len(frames)]+" Complete Jira authentication in your browser…")
	if s.link != "" {
		body += "\n\nOpen this link if the browser did not launch:\n" + s.link
	}
	view := tea.NewView(s.frame(s.boundary(s.title, false), body, s.boundary("ctrl+c cancel", false)))
	view.AltScreen = true
	view.WindowTitle = s.title
	return view
}

func (s *jiraAuthScreen) selectionForm() *huh.Form {
	choices := make([]huh.Option[int], len(s.options))
	for index, option := range s.options {
		choices[index] = huh.NewOption(option.label, index)
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[int]().Title("Jira project").Options(choices...).Value(&s.selected),
		huh.NewConfirm().Title("Save this Jira configuration?").WithButtonAlignment(lipgloss.Left).Value(&s.confirmed),
	)).WithShowHelp(false).WithShowErrors(false).WithWidth(s.formWidth())
}

func (s *jiraAuthScreen) waitForLink() tea.Cmd {
	return func() tea.Msg {
		select {
		case url := <-s.links:
			return jiraLinkResult{url: url}
		case <-s.ctx.Done():
			return jiraLinkResult{}
		}
	}
}

type jiraLoadingTickMsg struct{}

func jiraLoadingTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return jiraLoadingTickMsg{} })
}
