package agentmcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/config"
)

var (
	jiraWorkspace  = config.Workspace{TaskTracker: "jira", Jira: &config.JiraWorkspace{Site: "https://study.atlassian.net", Project: "STUDY"}}
	kaneoWorkspace = config.Workspace{TaskTracker: "kaneo", Kaneo: &config.KaneoWorkspace{URL: "https://kaneo.example.test/", Project: "TOD"}}
)

func readClients(t *testing.T, root string) (string, string) {
	t.Helper()
	codex, err := os.ReadFile(codexConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	claude, err := os.ReadFile(claudeConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	return string(codex), string(claude)
}

func TestConfigureInstallsSelectedTrackerInBothClients(t *testing.T) {
	for _, tc := range []struct {
		workspace config.Workspace
		name, url string
	}{
		{jiraWorkspace, jiraServer, jiraMCPURL},
		{kaneoWorkspace, kaneoServer, "https://kaneo.example.test/api/mcp"},
	} {
		root := t.TempDir()
		if err := Configure(root, tc.workspace); err != nil {
			t.Fatal(err)
		}
		codex, claude := readClients(t, root)
		if !strings.Contains(codex, "[mcp_servers."+tc.name+"]") || !strings.Contains(codex, tc.url) {
			t.Fatalf("Codex config = %s", codex)
		}
		if !strings.Contains(claude, `"`+tc.name+`"`) || !strings.Contains(claude, tc.url) {
			t.Fatalf("Claude config = %s", claude)
		}
	}
}

func TestConfigureSwitchingTrackersReplacesOnlyManagedEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexConfigPath(root), []byte("model = \"gpt\"\n\n[mcp_servers.other]\nurl = \"https://example.test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeConfigPath(root), []byte(`{"enabled":true,"mcpServers":{"other":{"url":"https://example.test"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Configure(root, jiraWorkspace); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(codexConfigPath(root), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n[mcp_servers.atlassian-jira.http_headers]\nX-Custom = \"value\"\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Configure(root, kaneoWorkspace); err != nil {
		t.Fatal(err)
	}
	first, firstClaude := readClients(t, root)
	for _, content := range []string{first, firstClaude} {
		if strings.Contains(content, "atlassian") || !strings.Contains(content, "kaneo.example.test") || !strings.Contains(content, "https://example.test") {
			t.Fatalf("switch left wrong entries: %s", content)
		}
	}
	if !strings.Contains(firstClaude, `"enabled": true`) || !strings.Contains(first, `model = "gpt"`) {
		t.Fatal("unrelated settings were not preserved")
	}
	if err := Configure(root, kaneoWorkspace); err != nil {
		t.Fatal(err)
	}
	second, secondClaude := readClients(t, root)
	if first != second || firstClaude != secondClaude {
		t.Fatal("repeated configuration was not idempotent")
	}
}

func TestConfigureWithoutMCPTrackerRemovesManagedEntries(t *testing.T) {
	root := t.TempDir()
	if err := Configure(root, kaneoWorkspace); err != nil {
		t.Fatal(err)
	}
	for _, tracker := range []string{"google_tasks", "none"} {
		if err := Configure(root, config.Workspace{TaskTracker: tracker}); err != nil {
			t.Fatal(err)
		}
		codex, claude := readClients(t, root)
		if strings.Contains(codex, "kaneo") || strings.Contains(claude, "kaneo") {
			t.Fatalf("%s left Kaneo configured: %s %s", tracker, codex, claude)
		}
	}
	empty := t.TempDir()
	if err := Configure(empty, config.Workspace{TaskTracker: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(claudeConfigPath(empty)); !os.IsNotExist(err) {
		t.Fatalf("removal created a client config: %v", err)
	}
}
