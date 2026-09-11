package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	google "github.com/algebananazzzzz/Corum/internal/google"
)

func (u configureUI) checkGoogle(ctx context.Context) error { _, err := google.Lists(ctx); return err }

func (u configureUI) connectGoogle(ctx context.Context, force bool) error {
	if !force && u.checkGoogle(ctx) == nil {
		return nil
	}
	if _, err := exec.LookPath("gws"); err != nil && os.Getenv("CORUM_GWS_BIN") == "" {
		if _, err := exec.LookPath("npx"); err != nil {
			return fmt.Errorf("install Node.js LTS from https://nodejs.org, then return here; Corum will obtain the Google connector automatically")
		}
	}
	// Ask the CLI where it stores credentials instead of assuming an OS path.
	data, err := google.Command(ctx, "auth", "status").Output()
	if err != nil {
		return fmt.Errorf("could not inspect Google connection: %w", err)
	}
	var status struct {
		ClientConfig string `json:"client_config"`
		Exists       bool   `json:"client_config_exists"`
		Method       string `json:"auth_method"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return err
	}
	if !force && status.Method != "" && status.Method != "none" {
		index, e := u.prompts.Select("Google connection needs attention", []Choice{{Label: "Sign in again"}, {Label: "Back — check API access or administrator restrictions"}})
		if e != nil {
			return e
		}
		if index != 0 {
			return ErrCancelled
		}
	}
	if !status.Exists && os.Getenv("GOOGLE_WORKSPACE_CLI_CLIENT_ID") == "" {
		if err := u.googleClientSetup(status.ClientConfig); err != nil {
			return err
		}
	}
	if err := ShowNotice("Connect Google", "Your browser will open. Choose the Google account to use and allow Google Tasks access. Your organization may require administrator approval.", u.in, u.out); err != nil {
		return err
	}
	if err := google.RunInteractive(ctx, u.in, u.out, "auth", "login", "-s", "tasks"); err != nil {
		return err
	}
	return u.checkGoogle(ctx)
}

func (u configureUI) googleClientSetup(target string) error {
	steps := []struct{ title, text string }{
		{"Google connection — one-time preparation", "Corum does not yet ship a registered Google login application. Prepare a Desktop OAuth client once; Corum will import it and handle future sign-ins. Open https://console.cloud.google.com/projectselector2/home/dashboard and select or create a project."},
		{"Enable Google Tasks", "In your selected project, open https://console.cloud.google.com/apis/library/tasks.googleapis.com and select Enable."},
		{"Configure Google sign-in", "Open https://console.cloud.google.com/auth/overview and complete branding, audience, and contact details. For an External application in Testing, add your Google account under Audience → Test users. Organization policy may require your administrator."},
		{"Download the login configuration", "Open https://console.cloud.google.com/auth/clients → Create client → Desktop app. Give it a name such as Corum and download the JSON file. Corum will import the file on the next screen."},
	}
	for _, step := range steps {
		if start := strings.Index(step.text, "https://"); start >= 0 {
			link := strings.Fields(step.text[start:])[0]
			openSetupLink(link)
		}
		if err := ShowNotice(step.title, step.text, u.in, u.out); err != nil {
			return err
		}
	}
	path, err := u.prompts.Input("Path to downloaded Google client JSON", "")
	if err != nil {
		return err
	}
	path = strings.Trim(strings.TrimSpace(path), "\"'")
	if strings.HasPrefix(path, "~/") {
		dir, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		path = filepath.Join(dir, path[2:])
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not read the downloaded file: %w", err)
	}
	return importGoogleClient(target, data)
}

func openSetupLink(link string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", link)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	default:
		cmd = exec.Command("xdg-open", link)
	}
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
	}
}

func importGoogleClient(target string, data []byte) error {
	var value struct {
		Installed struct {
			ID      string `json:"client_id"`
			Secret  string `json:"client_secret"`
			Project string `json:"project_id"`
		} `json:"installed"`
	}
	if json.Unmarshal(data, &value) != nil || value.Installed.ID == "" || value.Installed.Secret == "" || value.Installed.Project == "" {
		return fmt.Errorf("choose the downloaded Desktop app JSON containing client_id, client_secret and project_id")
	}
	if target == "" {
		return fmt.Errorf("Google connector did not provide a credential location")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
