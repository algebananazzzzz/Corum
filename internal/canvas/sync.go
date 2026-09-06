package canvas

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/algebananazzzzz/Corum/internal/config"
)

// Sync captures exactly the selected Canvas sources. It does not construct a
// client, so disabled and dry-run paths never read credentials or touch HTTP.
func Sync(ctx context.Context, root string, workspace config.Workspace, course config.Course, client SourceClient, dryRun bool) (StageResult, error) {
	result := StageResult{Course: course.Code, DryRun: dryRun, Changes: []Change{}, Failures: []Failure{}, Sources: []SourceResult{}}
	if !config.Effective(workspace, course).Canvas {
		result.Status = "disabled"
		return result, nil
	}
	if course.Canvas == nil || workspace.Canvas == nil {
		result.Status = "disabled"
		return result, nil
	}
	if dryRun && client == nil {
		result.Status = "pending"
		return result, nil
	}
	if client == nil {
		return result, fmt.Errorf("Canvas client is required")
	}
	statePath, manifestPath, err := canvasPaths(root, course, !dryRun)
	if err != nil {
		return result, err
	}
	state, err := readState(statePath, course.Canvas.Sources)
	if err != nil {
		return result, err
	}
	courseDir := filepath.Dir(filepath.Dir(statePath))
	rawDir := filepath.Join(courseDir, "raw")
	if info, statErr := os.Lstat(rawDir); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("raw directory must not be a symlink")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return result, statErr
	} else if os.IsNotExist(statErr) && !dryRun {
		if err := os.MkdirAll(rawDir, 0755); err != nil {
			return result, err
		}
	}
	updates := map[string]any{}
	for _, source := range course.Canvas.Sources {
		changes, failures, update, err := captureSource(ctx, source, courseDir, course.Canvas.ID, state.Sources[source], course.Canvas.Folders, workspace.Canvas.URL, workspace.Workspace.Timezone, client, dryRun)
		sr := SourceResult{Source: source, Changes: []string{}, Failures: []string{}}
		for _, change := range changes {
			result.Changes = append(result.Changes, change)
			sr.Changes = append(sr.Changes, change.ID)
		}
		for _, failure := range failures {
			result.Failures = append(result.Failures, failure)
			sr.Failures = append(sr.Failures, failure.ID)
		}
		if err != nil {
			failure := failureOf(source, source, "", err.Error(), "")
			result.Failures = append(result.Failures, failure)
			sr.Failures = append(sr.Failures, failure.ID)
		}
		if err != nil {
			sr.Status = "failed"
		} else if len(sr.Failures) > 0 && len(sr.Changes) > 0 {
			sr.Status = "partial"
		} else if len(sr.Failures) > 0 {
			sr.Status = "failed"
		} else if len(sr.Changes) > 0 {
			sr.Status = "changed"
		} else {
			sr.Status = "up_to_date"
		}
		result.Sources = append(result.Sources, sr)
		if err == nil && len(failures) == 0 {
			updates[source] = update
		}
	}
	if len(result.Failures) > 0 {
		if len(result.Changes) > 0 {
			result.Status = "partial"
		} else {
			result.Status = "failed"
		}
	} else if len(result.Changes) > 0 {
		result.Status = "changed"
	} else {
		result.Status = "up_to_date"
	}
	sortedResult(&result)
	if dryRun {
		return result, nil
	}
	for source, update := range updates {
		if source == "syllabus" {
			state.Sources[source] = update
			continue
		}
		values, ok := update.(map[string]any)
		if !ok {
			continue
		}
		existing, _ := state.Sources[source].(map[string]any)
		if existing == nil {
			existing = map[string]any{}
		}
		for key, value := range values {
			existing[key] = value
		}
		state.Sources[source] = existing
	}
	if err := writeState(statePath, state, workspace.Workspace.Timezone); err != nil {
		return result, err
	}
	if err := persistManifest(manifestPath, workspace, course, result); err != nil {
		return result, err
	}
	return result, nil
}

func sourceState(value any) map[string]any {
	if values, ok := value.(map[string]any); ok {
		return values
	}
	return map[string]any{}
}
func str(record map[string]any, key string) string { value, _ := record[key].(string); return value }
func id(record map[string]any) string {
	switch value := record["id"].(type) {
	case string:
		return value
	case float64:
		return fmt.Sprintf("%.0f", value)
	case json.Number:
		return value.String()
	default:
		return ""
	}
}
func sourceURL(courseID int, source string) string {
	return fmt.Sprintf("/api/v1/courses/%d/%s", courseID, source)
}
func changeOf(source, kind, item, summary, path string, details map[string]any) Change {
	itemCopy := item
	pathCopy := path
	return Change{ID: "canvas:" + source + ":" + item, Source: source, ItemID: &itemCopy, Kind: kind, Status: "changed", Summary: summary, RawPath: &pathCopy, Details: details}
}
func failureOf(source, kind, item, message, path string) Failure {
	var itemCopy, pathCopy *string
	if item != "" {
		value := item
		itemCopy = &value
	}
	if path != "" {
		value := path
		pathCopy = &value
	}
	return Failure{ID: "canvas:" + source + func() string {
		if item != "" {
			return ":" + item
		}
		return ""
	}(), Source: source, ItemID: itemCopy, Kind: kind, Status: "failed", Error: message, RawPath: pathCopy, Details: map[string]any{}}
}
func isNew(seen map[string]any, item string) bool { _, ok := seen[item]; return !ok }
func minute(value string) string {
	if value == "" {
		return ""
	}
	moment, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return moment.Truncate(time.Minute).Format(time.RFC3339)
}
func digest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha1.Sum(data)
	return "sha1:" + hex.EncodeToString(sum[:])
}

func captureSource(ctx context.Context, source, courseDir string, courseID int, prior any, folders map[string]string, origin, zone string, client SourceClient, dry bool) ([]Change, []Failure, any, error) {
	seen := sourceState(prior)
	switch source {
	case "announcements":
		return captureBodies(ctx, source, courseDir, courseID, seen, origin, zone, client, dry)
	case "assignments":
		return captureBodies(ctx, source, courseDir, courseID, seen, origin, zone, client, dry)
	case "files":
		return captureFiles(ctx, courseDir, courseID, seen, folders, client, dry)
	case "pages":
		return capturePages(ctx, courseDir, courseID, seen, origin, zone, client, dry)
	case "modules":
		return captureModules(ctx, courseDir, courseID, seen, zone, client, dry)
	case "syllabus":
		return captureSyllabus(ctx, courseDir, courseID, prior, origin, zone, client, dry)
	default:
		return nil, nil, nil, fmt.Errorf("unsupported Canvas source %q", source)
	}
}
func captureBodies(ctx context.Context, source, courseDir string, courseID int, seen map[string]any, origin, zone string, client SourceClient, dry bool) ([]Change, []Failure, any, error) {
	endpoint := source
	if source == "announcements" {
		endpoint = "discussion_topics"
	}
	query := url.Values{}
	if source == "announcements" {
		query.Set("only_announcements", "true")
	}
	records, err := client.GetAll(ctx, sourceURL(courseID, endpoint), query)
	if err != nil {
		return nil, nil, nil, err
	}
	sortRecords(records)
	changes := []Change{}
	failures := []Failure{}
	updates := map[string]any{}
	for _, record := range records {
		item := id(record)
		if item == "" {
			continue
		}
		changed := isNew(seen, item)
		kind := strings.TrimSuffix(source, "s")
		title := str(record, "title")
		if source == "assignments" {
			title = str(record, "name")
			old, _ := seen[item].(string)
			changed = changed || minute(str(record, "due_at")) != minute(old)
		}
		if !changed {
			continue
		}
		date := ""
		if source == "announcements" {
			date, _ = LocalTime(first(str(record, "posted_at"), str(record, "delayed_post_at"), str(record, "created_at")), zone)
		} else {
			date, _ = LocalTime(str(record, "due_at"), zone)
		}
		relative := source + "/" + Slug(title) + "-" + Slug(item) + ".md"
		summary := kind + " " + item + " · " + title
		if source == "assignments" {
			summary += " · due " + first(date, "none")
		}
		change := changeOf(source, kind, item, summary, relative, map[string]any{"title": title, "url": StripVerifier(str(record, "html_url"))})
		if dry {
			changes = append(changes, change)
			continue
		}
		body := str(record, "message")
		if source == "assignments" {
			body = str(record, "description")
		}
		imagePaths, imageErr := downloadImages(ctx, client, courseDir, relative, body)
		if imageErr != nil {
			failures = append(failures, failureOf(source, kind, item, "image not downloaded: "+imageErr.Error(), relative))
			continue
		}
		if err := writeBody(courseDir, relative, map[string]any{"source": "canvas", "kind": kind, "id": item, "title": title, "url": StripVerifier(str(record, "html_url")), "fetched": now(zone)}, body, origin, imagePaths); err != nil {
			return changes, nil, updates, err
		}
		changes = append(changes, change)
		if source == "announcements" {
			updates[item] = date
		} else {
			updates[item] = date
		}
	}
	return changes, failures, updates, nil
}
func captureFiles(ctx context.Context, courseDir string, courseID int, seen map[string]any, folders map[string]string, client SourceClient, dry bool) ([]Change, []Failure, any, error) {
	records, err := client.GetAll(ctx, sourceURL(courseID, "files"), url.Values{})
	if err != nil {
		return nil, nil, nil, err
	}
	folderRecords, err := client.GetAll(ctx, sourceURL(courseID, "folders"), url.Values{})
	if err != nil {
		return nil, nil, nil, err
	}
	folderPaths := map[string]string{}
	for _, record := range folderRecords {
		folderPaths[id(record)] = strings.TrimPrefix(str(record, "full_name"), rootPrefix)
	}
	sortRecords(records)
	changes := []Change{}
	failures := []Failure{}
	updates := map[string]any{}
	for _, record := range records {
		item := id(record)
		if item == "" || !isNew(seen, item) {
			continue
		}
		relative := Place(folderPaths[valueID(record, "folder_id")], str(record, "display_name"), folders)
		change := changeOf("files", "file", item, "file "+item+" · "+relative, relative, map[string]any{"display_name": str(record, "display_name")})
		if dry {
			changes = append(changes, change)
			continue
		}
		target, err := SafeDestination(filepath.Join(courseDir, "raw"), relative)
		if err == nil {
			err = client.Download(ctx, str(record, "url"), target)
		}
		if err != nil {
			failures = append(failures, failureOf("files", "file", item, "file "+item+" not downloaded: "+err.Error(), relative))
			continue
		}
		changes = append(changes, change)
		updates[item] = relative
	}
	return changes, failures, updates, nil
}
func capturePages(ctx context.Context, courseDir string, courseID int, seen map[string]any, origin, zone string, client SourceClient, dry bool) ([]Change, []Failure, any, error) {
	records, err := client.GetAll(ctx, sourceURL(courseID, "pages"), url.Values{})
	if err != nil {
		records, err = pagesViaModules(ctx, courseID, client)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if front, frontErr := client.Get(ctx, sourceURL(courseID, "front_page"), nil); frontErr == nil && str(front, "url") != "" {
		filtered := []map[string]any{front}
		for _, record := range records {
			if str(record, "url") != str(front, "url") {
				filtered = append(filtered, record)
			}
		}
		records = filtered
	}
	sort.Slice(records, func(i, j int) bool { return str(records[i], "url") < str(records[j], "url") })
	changes := []Change{}
	updates := map[string]any{}
	for _, record := range records {
		item := str(record, "url")
		if item == "" {
			continue
		}
		updated := str(record, "updated_at")
		old, known := seen[item].(string)
		if known && (old == "" || (updated != "" && updated <= old)) {
			continue
		}
		relative := "pages/" + Slug(str(record, "title")) + "-" + Slug(item) + ".md"
		changes = append(changes, changeOf("pages", "page", item, "page "+item+" · "+str(record, "title"), relative, map[string]any{"title": str(record, "title")}))
		if dry {
			continue
		}
		body := str(record, "body")
		if _, ok := record["body"]; !ok {
			record, err = client.Get(ctx, sourceURL(courseID, "pages/")+url.PathEscape(item), nil)
			if err != nil {
				return changes, nil, updates, err
			}
			body = str(record, "body")
		}
		if err := writeBody(courseDir, relative, map[string]any{"source": "canvas", "kind": "page", "page_url": item, "title": str(record, "title"), "fetched": now(zone)}, body, origin, nil); err != nil {
			return changes, nil, updates, err
		}
		updates[item] = updated
	}
	return changes, nil, updates, nil
}
func pagesViaModules(ctx context.Context, courseID int, client SourceClient) ([]map[string]any, error) {
	modules, err := client.GetAll(ctx, sourceURL(courseID, "modules"), nil)
	if err != nil {
		return nil, err
	}
	pages := []map[string]any{}
	for _, module := range modules {
		items, itemErr := client.GetAll(ctx, sourceURL(courseID, "modules/")+id(module)+"/items", nil)
		if itemErr != nil {
			return nil, itemErr
		}
		for _, item := range items {
			if str(item, "type") == "Page" && str(item, "page_url") != "" {
				page, pageErr := client.Get(ctx, sourceURL(courseID, "pages/")+url.PathEscape(str(item, "page_url")), nil)
				if pageErr != nil {
					return nil, pageErr
				}
				pages = append(pages, page)
			}
		}
	}
	return pages, nil
}
func captureModules(ctx context.Context, courseDir string, courseID int, seen map[string]any, zone string, client SourceClient, dry bool) ([]Change, []Failure, any, error) {
	modules, err := client.GetAll(ctx, sourceURL(courseID, "modules"), url.Values{})
	if err != nil {
		return nil, nil, nil, err
	}
	sortRecords(modules)
	changes := []Change{}
	updates := map[string]any{}
	lines := []string{}
	for _, module := range modules {
		moduleID := id(module)
		items, err := client.GetAll(ctx, sourceURL(courseID, "modules/")+moduleID+"/items", url.Values{})
		if err != nil {
			return changes, nil, updates, err
		}
		sortRecords(items)
		d := digest(map[string]any{"name": str(module, "name"), "position": module["position"], "items": items})
		updates[moduleID] = d
		if seen[moduleID] == d {
			continue
		}
		changes = append(changes, changeOf("modules", "module", moduleID, "module "+moduleID+" · "+str(module, "name"), "modules.md", map[string]any{"digest": d}))
		lines = append(lines, "## "+str(module, "name"), "", "| # | Type | Title |", "| --- | --- | --- |")
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("| %v | %s | %s |", item["position"], str(item, "type"), strings.ReplaceAll(str(item, "title"), "|", `\|`)))
		}
		lines = append(lines, "")
	}
	if len(changes) > 0 && !dry {
		target, err := SafeDestination(filepath.Join(courseDir, "raw"), "modules.md")
		if err != nil {
			return changes, nil, updates, err
		}
		content, err := Frontmatter(map[string]any{"source": "canvas", "kind": "modules", "fetched": now(zone)}, nil)
		if err != nil {
			return changes, nil, updates, err
		}
		if err = os.MkdirAll(filepath.Dir(target), 0755); err == nil {
			err = os.WriteFile(target, []byte(content+strings.Join(lines, "\n")), 0644)
		}
		if err != nil {
			return changes, nil, updates, err
		}
	}
	return changes, nil, updates, nil
}
func captureSyllabus(ctx context.Context, courseDir string, courseID int, seen any, origin, zone string, client SourceClient, dry bool) ([]Change, []Failure, any, error) {
	record, err := client.Get(ctx, fmt.Sprintf("/api/v1/courses/%d", courseID), url.Values{"include[]": {"syllabus_body"}})
	if err != nil {
		return nil, nil, nil, err
	}
	body := str(record, "syllabus_body")
	d := digest(body)
	if previous, ok := seen.(string); ok && previous == d {
		return nil, nil, d, nil
	}
	change := changeOf("syllabus", "syllabus", "syllabus", "syllabus changed", "syllabus.md", map[string]any{"digest": d})
	if dry {
		return []Change{change}, nil, d, nil
	}
	if err := writeBody(courseDir, "syllabus.md", map[string]any{"source": "canvas", "kind": "syllabus", "fetched": now(zone)}, body, origin, nil); err != nil {
		return nil, nil, nil, err
	}
	return []Change{change}, nil, d, nil
}
func downloadImages(ctx context.Context, client SourceClient, courseDir, relative, body string) (map[string]string, error) {
	images, err := Images(body)
	if err != nil {
		return nil, err
	}
	paths := map[string]string{}
	folder := filepath.Dir(relative)
	for index, image := range images {
		u, parseErr := url.Parse(image)
		if parseErr != nil {
			return nil, parseErr
		}
		name := filepath.Base(u.Path)
		if name == "." || name == "/" || name == "" {
			name = fmt.Sprintf("image-%d", index+1)
		}
		target, pathErr := SafeDestination(filepath.Join(courseDir, "raw"), filepath.ToSlash(filepath.Join(folder, name)))
		if pathErr != nil {
			return nil, pathErr
		}
		if err := client.Download(ctx, image, target); err != nil {
			return nil, err
		}
		paths[image] = name
	}
	return paths, nil
}
func writeBody(courseDir, relative string, fields map[string]any, body, origin string, imagePaths map[string]string) error {
	target, err := SafeDestination(filepath.Join(courseDir, "raw"), relative)
	if err != nil {
		return err
	}
	markdown, err := ToMarkdownImages(body, imagePaths)
	if err != nil {
		return err
	}
	links, err := Links(body, origin)
	if err != nil {
		return err
	}
	frontmatter, err := Frontmatter(fields, links)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(frontmatter+markdown), 0644)
}
func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func valueID(record map[string]any, key string) string {
	switch value := record[key].(type) {
	case string:
		return value
	case float64:
		return fmt.Sprintf("%.0f", value)
	default:
		return ""
	}
}
func sortRecords(records []map[string]any) {
	sort.Slice(records, func(i, j int) bool {
		return first(id(records[i]), str(records[i], "url")) < first(id(records[j]), str(records[j], "url"))
	})
}
func now(zone string) string {
	value := time.Now()
	if location, err := time.LoadLocation(zone); err == nil {
		value = value.In(location)
	}
	return value.Format(time.RFC3339)
}
func persistManifest(path string, workspace config.Workspace, course config.Course, result StageResult) error {
	canvasStage := map[string]any{"status": result.Status, "changes": result.Changes, "failures": result.Failures, "sources": result.Sources}
	manifest := map[string]any{"version": 2, "run_id": time.Now().Format("20060102T150405Z0700"), "course": course.Code, "effective_features": map[string]bool{"jira": config.Effective(workspace, course).Jira, "wiki": config.Effective(workspace, course).Wiki}, "canvas": canvasStage, "jira": map[string]any{"status": featureStatus(config.Effective(workspace, course).Jira)}, "wiki": map[string]any{"status": featureStatus(config.Effective(workspace, course).Wiki)}}
	if data, err := os.ReadFile(path); err == nil {
		var old map[string]json.RawMessage
		if json.Unmarshal(data, &old) == nil {
			for _, key := range []string{"jira", "wiki"} {
				if value, ok := old[key]; ok {
					var preserved any
					if json.Unmarshal(value, &preserved) == nil {
						manifest[key] = preserved
					}
				}
			}
		}
	}
	return atomicJSON(path, manifest)
}
func featureStatus(enabled bool) string {
	if enabled {
		return "pending"
	}
	return "disabled"
}
