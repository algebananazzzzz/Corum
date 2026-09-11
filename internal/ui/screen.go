package ui

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

const screenMaxWidth = 120

type LoadingRunner func(context.Context, string, func(context.Context) error) error

type Screen struct {
	sidebar string
	title   string
	form    *huh.Form
	width   int
	height  int
	done    bool
	style   screenStyle
}

type screenStyle struct {
	base   lipgloss.Style
	header lipgloss.Style
	red    lipgloss.Style
}

func NewScreen(title string, form *huh.Form) *Screen {
	screen := newScreen(title)
	screen.form = form.WithShowHelp(false).WithShowErrors(false).WithWidth(screen.formWidth())
	return screen
}

func newScreen(title string) *Screen {
	style := screenStyle{
		base:   lipgloss.NewStyle().Padding(1, 4, 0, 1),
		header: lipgloss.NewStyle().Foreground(lipgloss.Color("#7571F9")).Bold(true).Padding(0, 1, 0, 2),
	}
	style.red = style.header.Foreground(lipgloss.Color("#FE5F86"))
	return &Screen{title: title, width: screenMaxWidth - style.base.GetHorizontalFrameSize(), style: style}
}

func RunScreen(title string, form *huh.Form, in io.Reader, out io.Writer) error {
	program := tea.NewProgram(NewScreen(title, form), tea.WithInput(in), tea.WithOutput(out))
	_, err := program.Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) || errors.Is(err, context.Canceled) {
			return ErrCancelled
		}
		return err
	}
	if form.State == huh.StateAborted {
		return ErrCancelled
	}
	return nil
}

func ShowNotice(title, message string, in io.Reader, out io.Writer) error {
	return RunScreen(title, newNoticeForm(message, "", "Done"), in, out)
}

func ShowError(title, message string, err error, in io.Reader, out io.Writer) error {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return RunScreen(title, newNoticeForm(message, detail, "Close"), in, out)
}

func newNoticeForm(message, detail, action string) *huh.Form {
	note := huh.NewNote().Title(message).Next(true).NextLabel(action)
	if detail != "" {
		note.Description(detail)
	}
	return huh.NewForm(huh.NewGroup(note))
}

type loadingResult struct{ err error }

type loadingTick struct{}

type loadingScreen struct {
	*Screen
	message string
	task    func(context.Context) error
	ctx     context.Context
	cancel  context.CancelFunc
	step    int
	err     error
}

func runLoading(ctx context.Context, title, message string, in io.Reader, out io.Writer, task func(context.Context) error) error {
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	program := tea.NewProgram(newLoadingScreen(title, message, taskCtx, task, cancel), tea.WithInput(in), tea.WithOutput(out))
	model, err := program.Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) || errors.Is(err, context.Canceled) {
			return ErrCancelled
		}
		return err
	}
	result, ok := model.(*loadingScreen)
	if !ok {
		return errors.New("loading screen returned an unexpected model")
	}
	if errors.Is(result.err, context.Canceled) {
		return ErrCancelled
	}
	return result.err
}

func runLoadingTask(runner LoadingRunner, ctx context.Context, message string, task func(context.Context) error) error {
	if runner == nil {
		return task(ctx)
	}
	return runner(ctx, message, task)
}

func newLoadingScreen(title, message string, ctx context.Context, task func(context.Context) error, cancel context.CancelFunc) *loadingScreen {
	return &loadingScreen{Screen: newScreen(title), message: message, ctx: ctx, task: task, cancel: cancel}
}

func (s *loadingScreen) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return loadingResult{err: s.task(s.ctx)} },
		loadingTickCmd(),
	)
}

func (s *loadingScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = min(msg.Width, screenMaxWidth) - s.style.base.GetHorizontalFrameSize()
		s.height = msg.Height
	case loadingTick:
		s.step++
		return s, loadingTickCmd()
	case loadingResult:
		s.err = msg.err
		return s, tea.Quit
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			s.cancel()
			s.err = ErrCancelled
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s *loadingScreen) View() tea.View {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	body := "\n" + s.style.header.Render(frames[s.step%len(frames)]+" "+s.message)
	view := tea.NewView(s.frame(s.boundary(s.title, false), body, s.boundary("ctrl+c cancel", false)))
	view.AltScreen = true
	view.WindowTitle = s.title
	return view
}

func loadingTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return loadingTick{} })
}

func (s *Screen) Init() tea.Cmd {
	return s.form.Init()
}

func (s *Screen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		s.width = min(size.Width, screenMaxWidth) - s.style.base.GetHorizontalFrameSize()
		s.height = size.Height
		s.form = s.form.WithWidth(s.formWidth()).WithHeight(max(s.height-s.style.base.GetVerticalFrameSize()-5, 5))
	}
	next, cmd := s.form.Update(msg)
	if form, ok := next.(*huh.Form); ok {
		s.form = form
	}
	if s.form.State == huh.StateCompleted || s.form.State == huh.StateAborted {
		s.done = true
		return s, tea.Quit
	}
	return s, cmd
}

func (s *Screen) formWidth() int {
	if s.sidebar != "" && s.width >= 80 {
		return s.width - 35
	}
	return s.width
}

func (s *Screen) View() tea.View {
	content := ""
	if !s.done {
		form := strings.TrimSuffix(s.form.View(), "\n\n")
		header := s.boundary(s.title, false)
		footer := s.boundary(s.form.Help().ShortHelpView(s.form.KeyBinds()), false)
		if errors := s.form.Errors(); len(errors) > 0 {
			header = s.boundary(errors[0].Error(), true)
			footer = s.boundary("", true)
		}
		content = s.frame(header, form, footer)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = s.title
	return view
}

func (s *Screen) boundary(text string, failed bool) string {
	style := s.style.header
	if failed {
		style = s.style.red
	}
	return lipgloss.PlaceHorizontal(s.width, lipgloss.Left, style.Render(text), lipgloss.WithWhitespaceChars("/"))
}

func (s *Screen) frame(header, form, footer string) string {
	if s.sidebar != "" && s.width >= 80 {
		left := lipgloss.NewStyle().Width(32).MaxWidth(32).Render(s.sidebar)
		form = lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", form)
	}
	content := header + "\n\n" + form
	gap := 2
	if s.height > 0 {
		if available := s.height - s.style.base.GetVerticalFrameSize() - lipgloss.Height(content) - lipgloss.Height(footer) + 1; available > gap {
			gap = available
		}
	}
	return s.style.base.Render(content + strings.Repeat("\n", gap) + footer)
}
