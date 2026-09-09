package jira

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectMCPConfigCreatesBothClientFiles(t *testing.T) {
	root := t.TempDir()
	if err := writeProjectMCPConfig(root); err != nil {
		t.Fatal(err)
	}
	codex, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	claude, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(codex), "mcp_servers.atlassian-jira") || !strings.Contains(string(codex), rovoMCPURL) {
		t.Fatalf("Codex config = %s", codex)
	}
	if !strings.Contains(string(claude), `"atlassian-jira"`) || !strings.Contains(string(claude), rovoMCPURL) {
		t.Fatalf("Claude config = %s", claude)
	}
}

func TestProjectMCPConfigPreservesUnrelatedEntriesAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex", "config.toml"), []byte("model = \"gpt\"\n\n[mcp_servers.other]\nurl = \"https://example.test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mcp.json"), []byte(`{"mcpServers":{"other":{"url":"https://example.test"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectMCPConfig(root); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	firstClaude, _ := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err := writeProjectMCPConfig(root); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	secondClaude, _ := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if string(first) != string(second) || string(firstClaude) != string(secondClaude) {
		t.Fatal("repeated config write was not idempotent")
	}
	if !strings.Contains(string(second), "mcp_servers.other") || !strings.Contains(string(secondClaude), "https://example.test") {
		t.Fatal("unrelated MCP entry was not preserved")
	}
	if strings.Contains(string(second), "token") || strings.Contains(string(secondClaude), "token") {
		t.Fatal("config contains a token field")
	}
}
