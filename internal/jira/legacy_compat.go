package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/algebananazzzzz/Corum/internal/config"
)

// Client is retained only so downstream code can compile while migrating to
// SyncClient. It exposes read operations exclusively.
type Client = SyncClient

type EpicResult struct {
	Key     string
	Created bool
}

func (*JiraClient) EnsureEpic(context.Context, string, string) (EpicResult, error) {
	return EpicResult{}, fmt.Errorf("Jira epic creation is agent-owned through Jira MCP")
}
func (*JiraClient) ReconcileEpic(context.Context, string, string) (EpicResult, error) {
	return EpicResult{}, fmt.Errorf("Jira epic reconciliation uses sync-epic")
}

func DecodePlan(reader io.Reader) (Plan, error) {
	var plan Plan
	err := json.NewDecoder(reader).Decode(&plan)
	return plan, err
}
func ValidatePlan(config.Workspace, config.Course, Plan, bool) error { return nil }
func Apply(context.Context, string, config.Workspace, config.Course, Plan, Client, bool) (ApplyResult, error) {
	return ApplyResult{}, fmt.Errorf("Jira mutations are agent-owned through Jira MCP")
}
