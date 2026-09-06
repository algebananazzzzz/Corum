package jira

import "fmt"

// WriteState records what is known about a failed remote mutation.
type WriteState string

const (
	WriteNotApplied WriteState = "not_applied"
	WriteApplied    WriteState = "applied"
	WriteUnknown    WriteState = "unknown"
)

type IssueFields struct {
	Type        string    `json:"type"`
	Parent      string    `json:"parent"`
	Summary     string    `json:"summary"`
	Description *string   `json:"description,omitempty"`
	Due         *string   `json:"due,omitempty"`
	Labels      *[]string `json:"labels,omitempty"`
}

type Action struct {
	Action     string         `json:"action"`
	Issue      *IssueFields   `json:"issue,omitempty"`
	Key        string         `json:"key,omitempty"`
	Set        map[string]any `json:"set,omitempty"`
	Transition string         `json:"transition,omitempty"`
}

type Plan struct {
	Version int      `json:"version"`
	Course  string   `json:"course"`
	Epic    string   `json:"epic"`
	Actions []Action `json:"actions"`
}

type AppliedAction struct {
	ID          string `json:"id"`
	ActionIndex int    `json:"action_index"`
	Action      string `json:"action"`
	Key         string `json:"key"`
}

type ActionFailure struct {
	ID          string     `json:"id"`
	ActionIndex int        `json:"action_index"`
	Action      string     `json:"action"`
	Key         string     `json:"key,omitempty"`
	Phase       string     `json:"phase"`
	Error       string     `json:"error"`
	WriteState  WriteState `json:"write_state"`
	RetrySafe   bool       `json:"retry_safe"`
	ExactAction *Action    `json:"exact_action,omitempty"`
}

type ApplyResult struct {
	Course                 string          `json:"course"`
	Epic                   string          `json:"epic"`
	DryRun                 bool            `json:"dry_run"`
	Status                 string          `json:"status"`
	Applied                []AppliedAction `json:"applied"`
	Failures               []ActionFailure `json:"failures"`
	ReconciliationRequired bool            `json:"reconciliation_required"`
	RetrySafe              bool            `json:"retry_safe"`
	Reconciled             bool            `json:"reconciled"`
}

type MutationError struct {
	Message string
	State   WriteState
}

func (e *MutationError) Error() string {
	if e.Message == "" {
		return "Jira mutation failed"
	}
	return e.Message
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var ErrReconciliationRequired = fmt.Errorf("Jira reconciliation is required before retry")
