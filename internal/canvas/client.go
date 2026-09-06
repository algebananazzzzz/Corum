package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// maxPaginationPages bounds list retrieval so a malformed or hostile server
// cannot loop the client indefinitely.
const maxPaginationPages = 200

// verifierParamRE matches Canvas verifier query parameters in arbitrary text
// so transport errors never leak the user-specific secret.
var verifierParamRE = regexp.MustCompile(`[?&]?verifier=[^&"'\s)]*`)

// redactTransportError returns an error whose message carries no verifier
// secrets. The concrete URL is never re-parsed, so a malformed error cannot
// panic.
func redactTransportError(err error) error {
	if err == nil {
		return nil
	}
	message := verifierParamRE.ReplaceAllString(err.Error(), "")
	message = strings.ReplaceAll(message, "?&", "?")
	return errors.New(message)
}

// SourceClient is the capture boundary. It enables deterministic sync tests
// without constructing an authenticated HTTP client for disabled or dry runs.
type SourceClient interface {
	Get(context.Context, string, url.Values) (map[string]any, error)
	GetAll(context.Context, string, url.Values) ([]map[string]any, error)
	Download(context.Context, string, string) error
}

type Client struct {
	origin *url.URL
	token  string
	http   *http.Client
}

// NewClient constructs a client for one credential-free HTTPS Canvas origin.
func NewClient(origin, token string) (*Client, error) {
	u, err := parseOrigin(origin)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("CORUM_CANVAS_TOKEN environment variable is required")
	}
	return &Client{origin: u, token: token, http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// NewClientFromEnvironment resolves the effective token (environment first,
// then project-local, then the global fallback) when a real Canvas operation
// starts.
func NewClientFromEnvironment(origin, root string) (*Client, error) {
	token, err := LoadCredential(root)
	if err != nil {
		return nil, err
	}
	return NewClient(origin, token)
}

// CourseInfo is one Canvas course entry for the auth course list.
type CourseInfo struct {
	ID         string `json:"id"`
	CourseCode string `json:"course_code"`
	Name       string `json:"name"`
	Current    bool   `json:"current"`
	Conclusion string `json:"conclusion_date"`
}

// Courses lists the courses the authenticated token can access.
func (c *Client) Courses(ctx context.Context) ([]CourseInfo, error) {
	raw, err := c.GetAll(ctx, "/api/v1/courses", url.Values{})
	if err != nil {
		return nil, err
	}
	records := make([]CourseInfo, 0, len(raw))
	for _, record := range raw {
		records = append(records, CourseInfo{
			ID:         id(record),
			Name:       str(record, "name"),
			CourseCode: str(record, "course_code"),
			Current:    record["is_enrolled"] == true || record["current"] == true,
		})
	}
	return records, nil
}

func parseOrigin(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("Canvas URL must be a credential-free HTTPS origin")
	}
	u.Path = ""
	return u, nil
}
func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
func (c *Client) requestURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !u.IsAbs() {
		u = c.origin.ResolveReference(u)
	}
	if u.User != nil || !sameOrigin(c.origin, u) {
		return nil, fmt.Errorf("Canvas request must stay on configured origin")
	}
	return u, nil
}

func (c *Client) do(ctx context.Context, raw string, query url.Values, destination string) (*http.Response, error) {
	u, err := c.requestURL(raw)
	if err != nil {
		return nil, err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	for redirects := 0; ; redirects++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		response, err := c.http.Do(req)
		if err != nil {
			return nil, redactTransportError(fmt.Errorf("Canvas request failed: %w", err))
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			location := response.Header.Get("Location")
			response.Body.Close()
			if location == "" || redirects >= 5 {
				return nil, fmt.Errorf("Canvas redirect could not be followed safely")
			}
			next, err := u.Parse(location)
			if err != nil || !sameOrigin(c.origin, next) || next.User != nil {
				return nil, fmt.Errorf("Canvas redirect leaves configured origin")
			}
			u = next
			query = nil
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			return nil, fmt.Errorf("Canvas request failed with HTTP %d: %s", response.StatusCode, StripVerifier(u.String()))
		}
		return response, nil
	}
}

func (c *Client) Get(ctx context.Context, endpoint string, query url.Values) (map[string]any, error) {
	response, err := c.do(ctx, endpoint, query, "")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Canvas response: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("Canvas response must be an object")
	}
	return result, nil
}
func (c *Client) GetAll(ctx context.Context, endpoint string, query url.Values) ([]map[string]any, error) {
	if query == nil {
		query = url.Values{}
	}
	cloned := url.Values{}
	for key, values := range query {
		cloned[key] = append([]string(nil), values...)
	}
	query = cloned
	if query.Get("per_page") == "" {
		query.Set("per_page", "100")
	}
	next := endpoint
	records := []map[string]any{}
	visited := map[string]bool{}
	pages := 0
	for next != "" {
		pages++
		if pages > maxPaginationPages {
			return nil, errors.New("Canvas pagination exceeded page bound")
		}
		resolved, err := c.requestURL(next)
		if err != nil {
			return nil, err
		}
		key := resolved.String()
		if query != nil {
			key = key + "?" + query.Encode()
		}
		if visited[key] {
			return nil, errors.New("Canvas pagination repeated a request without forward progress")
		}
		visited[key] = true
		response, err := c.do(ctx, next, query, "")
		if err != nil {
			return nil, err
		}
		var page []map[string]any
		decodeErr := json.NewDecoder(response.Body).Decode(&page)
		link := nextLink(response.Header.Get("Link"))
		response.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("Canvas list response must be an array: %w", decodeErr)
		}
		records = append(records, page...)
		if len(page) == 0 && link != "" {
			return nil, errors.New("Canvas pagination returned an empty page with a next link")
		}
		if link == "" {
			break
		}
		next = link
		query = nil
	}
	return records, nil
}
func nextLink(header string) string {
	for _, segment := range strings.Split(header, ",") {
		if strings.Contains(segment, "rel=\"next\"") || strings.Contains(segment, "rel=next") {
			left := strings.Index(segment, "<")
			right := strings.Index(segment, ">")
			if left >= 0 && right > left {
				return strings.TrimSpace(segment[left+1 : right])
			}
		}
	}
	return ""
}
func (c *Client) Download(ctx context.Context, raw, target string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return fmt.Errorf("attachment URL must be a credential-free HTTP(S) URL")
	}
	var response *http.Response
	for redirects := 0; ; redirects++ {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if requestErr != nil {
			return requestErr
		}
		if sameOrigin(c.origin, u) {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		response, err = c.http.Do(req)
		if err != nil {
			return redactTransportError(fmt.Errorf("Canvas download failed: %w", err))
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			location := response.Header.Get("Location")
			response.Body.Close()
			if location == "" || redirects >= 5 {
				return fmt.Errorf("Canvas download redirect could not be followed safely")
			}
			next, parseErr := u.Parse(location)
			if parseErr != nil || next.User != nil || (next.Scheme != "https" && next.Scheme != "http") {
				return fmt.Errorf("Canvas download redirect is invalid")
			}
			u = next
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			return fmt.Errorf("Canvas download failed with HTTP %d: %s", response.StatusCode, StripVerifier(u.String()))
		}
		break
	}
	defer response.Body.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".canvas-download-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err = io.Copy(temporary, response.Body); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, target)
}
