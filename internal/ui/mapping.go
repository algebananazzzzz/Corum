package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/algebananazzzzz/Corum/internal/config"
	google "github.com/algebananazzzzz/Corum/internal/google"
	"github.com/algebananazzzzz/Corum/internal/jira"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

type destination struct{ ID, Name string }
type destinationStore struct {
	List   func(context.Context) ([]destination, error)
	Create func(context.Context, string) (destination, error)
}

func (u configureUI) mapping(ctx context.Context) error {
	ws, _, err := vault.Validate(u.root)
	if err != nil {
		return err
	}
	provider := activeTracker(ws)
	if provider == "none" {
		return nil
	}
	store := destinationStore{}
	if provider == "google_tasks" {
		store.List = func(ctx context.Context) ([]destination, error) {
			lists, err := google.Lists(ctx)
			result := []destination{}
			for _, l := range lists {
				result = append(result, destination{l.ID, l.Title})
			}
			return result, err
		}
		store.Create = func(ctx context.Context, name string) (destination, error) {
			list, err := google.CreateList(ctx, name)
			return destination{list.ID, list.Title}, err
		}
	} else {
		if ws.Jira == nil {
			return fmt.Errorf("configure a Jira site and project first")
		}
		session, err := u.openJira(ctx, false)
		if err != nil {
			return err
		}
		defer session.Close()
		store.List = func(ctx context.Context) ([]destination, error) {
			epics, err := session.Epics(ctx, ws.Jira.CloudID, ws.Jira.Project)
			result := []destination{}
			for _, e := range epics {
				result = append(result, destination{e.Key, e.Title})
			}
			return result, err
		}
		store.Create = func(ctx context.Context, name string) (destination, error) {
			epic, err := session.CreateEpic(ctx, ws.Jira.CloudID, ws.Jira.Project, name)
			return destination{epic.Key, epic.Title}, err
		}
	}
	for {
		_, all, err := vault.Validate(u.root)
		if err != nil {
			return err
		}
		courses := []config.Course{}
		for _, c := range all {
			if c.Canvas != nil {
				courses = append(courses, c)
			}
		}
		if len(courses) == 0 {
			return ShowNotice("Course mapping", "Select tracked courses first.", u.in, u.out)
		}
		var destinations []destination
		err = runLoading(ctx, "Course mapping", "Loading destinations…", u.in, u.out, func(ctx context.Context) error {
			var e error
			destinations, e = store.List(ctx)
			return e
		})
		if err != nil {
			return err
		}
		choices := []Choice{}
		for _, c := range courses {
			choices = append(choices, Choice{Label: c.Code + " → " + mappingLabel(c, provider, destinations)})
		}
		choices = append(choices, Choice{Label: "Back"})
		index, err := u.prompts.Select("Select a course to map", choices)
		if err != nil {
			return err
		}
		if index == len(courses) {
			return nil
		}
		if index < 0 || index >= len(courses) {
			return fmt.Errorf("invalid course")
		}
		p := u.prompts
		if hp, ok := p.(HuhPrompter); ok {
			lines := []string{}
			for _, choice := range choices[:len(courses)] {
				lines = append(lines, choice.Label)
			}
			hp.Sidebar = strings.Join(lines, "\n")
			p = hp
		}
		err = mapCourse(ctx, u.root, courses[index], provider, destinations, store, p)
		u.report(err)
	}
}

func mappingID(c config.Course, provider string) string {
	if provider == "jira" && c.Jira != nil {
		return c.Jira.Epic
	}
	if provider == "google_tasks" && c.GoogleTasks != nil {
		return c.GoogleTasks.ListID
	}
	return ""
}
func mappingLabel(c config.Course, provider string, items []destination) string {
	id := mappingID(c, provider)
	if id == "" {
		return "Unmapped"
	}
	for _, d := range items {
		if d.ID == id {
			return d.Name
		}
	}
	return "Needs attention (" + id + ")"
}

func mapCourse(ctx context.Context, root string, course config.Course, provider string, items []destination, store destinationStore, p Prompter) error {
	action, err := p.Select(course.Code+" destination — changes save immediately", []Choice{{Label: "Select existing"}, {Label: "Create new"}, {Label: "Leave unmapped / remove mapping"}, {Label: "Back"}})
	if err != nil {
		return err
	}
	if action == 3 {
		return nil
	}
	id := ""
	switch action {
	case 0:
		if len(items) == 0 {
			return fmt.Errorf("no destinations found; choose Create new")
		}
		choices := []Choice{}
		for _, d := range items {
			choices = append(choices, Choice{Label: d.Name + " (" + d.ID + ")"})
		}
		index, err := p.Select("Existing destinations (/ to search)", choices)
		if err != nil {
			return err
		}
		if index < 0 || index >= len(items) {
			return fmt.Errorf("invalid destination")
		}
		id = items[index].ID
	case 1:
		name := course.Code
		if course.Canvas != nil && course.Canvas.Name != "" {
			name += " — " + course.Canvas.Name
		}
		name, err = p.Input("Name for new destination", name)
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("name is required")
		}
		d, err := store.Create(ctx, name)
		if err != nil {
			return err
		}
		id = d.ID
		if id == "" {
			return fmt.Errorf("provider returned no destination ID; inspect destinations before retrying")
		}
	case 2:
	default:
		return fmt.Errorf("invalid mapping action")
	}
	if provider == "jira" {
		course.Jira = nil
		if id != "" {
			course.Jira = &config.JiraCourse{Epic: id}
		}
	} else {
		course.GoogleTasks = nil
		if id != "" {
			course.GoogleTasks = &config.GoogleTasksCourse{ListID: id}
		}
	}
	if err := vault.WriteCourse(root, course); err != nil {
		return fmt.Errorf("could not save mapping to %q: %w", id, err)
	}
	return nil
}

// Refresh mapped tasks after initial setup; successful course caches are independent.
func (u configureUI) syncMapped(ctx context.Context) {
	ws, courses, err := vault.Validate(u.root)
	if err != nil {
		u.report(err)
		return
	}
	if activeTracker(ws) == "google_tasks" {
		for _, c := range courses {
			if c.Canvas != nil && c.GoogleTasks != nil {
				_, err := google.SyncTasks(ctx, u.root, c.Code, c.GoogleTasks.ListID, google.SyncOptions{Client: google.DefaultCommandClient()})
				u.report(err)
			}
		}
		return
	}
	if activeTracker(ws) != "jira" || ws.Jira == nil {
		return
	}
	session, err := u.openJira(ctx, false)
	if err != nil {
		u.report(err)
		return
	}
	defer session.Close()
	client, err := jira.NewJiraClient(session, ws.Jira.CloudID)
	if err != nil {
		u.report(err)
		return
	}
	for _, c := range courses {
		if c.Canvas != nil && c.Jira != nil {
			_, err := jira.SyncEpic(ctx, u.root, c.Code, c.Jira.Epic, jira.SyncOptions{Client: client, Project: ws.Jira.Project})
			u.report(err)
		}
	}
}
