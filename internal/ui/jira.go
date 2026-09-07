package ui

import (
	"context"
	"fmt"

	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/jira"
)

type Resource struct {
	CloudID string
	Name    string
}

type Project struct {
	Key  string
	Name string
}

// JiraSession is the read-only selection surface used by setup.
type JiraSession interface {
	Resources(context.Context) ([]Resource, error)
	Projects(context.Context, string) ([]Project, error)
	Close() error
}

type rovoSelectionSession struct{ session *jira.RovoSession }

func (s rovoSelectionSession) Resources(ctx context.Context) ([]Resource, error) {
	resources, err := s.session.Resources(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		cloudID := resource.CloudID
		if cloudID == "" {
			cloudID = resource.ID
		}
		result = append(result, Resource{CloudID: cloudID, Name: resource.Name})
	}
	return result, nil
}

func (s rovoSelectionSession) Projects(ctx context.Context, cloudID string) ([]Project, error) {
	projects, err := s.session.Projects(ctx, cloudID)
	if err != nil {
		return nil, err
	}
	result := make([]Project, len(projects))
	for i, project := range projects {
		result[i] = Project{Key: project.Key, Name: project.Name}
	}
	return result, nil
}

func (s rovoSelectionSession) Close() error { return s.session.Close() }

// SelectJira selects an accessible Jira resource and project from a Rovo session.
func SelectJira(ctx context.Context, session *jira.RovoSession, prompts Prompter) (config.JiraWorkspace, error) {
	if session == nil {
		return config.JiraWorkspace{}, fmt.Errorf("Jira session is unavailable")
	}
	return selectJira(ctx, rovoSelectionSession{session}, prompts)
}

func selectJira(ctx context.Context, session JiraSession, prompts Prompter) (config.JiraWorkspace, error) {
	return selectJiraWithLoading(ctx, session, prompts, nil)
}

func selectJiraWithLoading(ctx context.Context, session JiraSession, prompts Prompter, loading LoadingRunner) (config.JiraWorkspace, error) {
	var resources []Resource
	err := runLoadingTask(loading, ctx, "Loading Jira sites…", func(ctx context.Context) error {
		var loadErr error
		resources, loadErr = session.Resources(ctx)
		return loadErr
	})
	if err != nil {
		return config.JiraWorkspace{}, err
	}
	if len(resources) == 0 {
		return config.JiraWorkspace{}, fmt.Errorf("no accessible Jira sites")
	}
	resource, err := selectValue(prompts, "Atlassian site", resources, func(value Resource) string {
		if value.Name != "" {
			return value.Name
		}
		return "Jira site"
	})
	if err != nil {
		return config.JiraWorkspace{}, err
	}
	if resource.CloudID == "" {
		return config.JiraWorkspace{}, fmt.Errorf("selected Jira site has no cloud ID")
	}
	var projects []Project
	err = runLoadingTask(loading, ctx, "Loading Jira projects…", func(ctx context.Context) error {
		var loadErr error
		projects, loadErr = session.Projects(ctx, resource.CloudID)
		return loadErr
	})
	if err != nil {
		return config.JiraWorkspace{}, err
	}
	if len(projects) == 0 {
		return config.JiraWorkspace{}, fmt.Errorf("selected Jira site has no projects")
	}
	project, err := selectValue(prompts, "Jira project", projects, func(value Project) string {
		if value.Name == "" {
			return value.Key
		}
		return value.Name + " (" + value.Key + ")"
	})
	if err != nil {
		return config.JiraWorkspace{}, err
	}
	return config.JiraWorkspace{CloudID: resource.CloudID, Project: project.Key}, nil
}

func selectValue[T any](prompts Prompter, label string, values []T, text func(T) string) (T, error) {
	var zero T
	choices := make([]Choice, len(values))
	for i, value := range values {
		choices[i] = Choice{Label: text(value)}
	}
	index, err := prompts.Select(label, choices)
	if err != nil {
		return zero, promptError(err)
	}
	if index < 0 || index >= len(values) {
		return zero, fmt.Errorf("invalid %s selection", label)
	}
	return values[index], nil
}
