package jira

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
)

type freshOAuthMCPServer struct {
	server        *httptest.Server
	mu            sync.Mutex
	registrations int
	bearers       []string
}

func newFreshOAuthMCPServer(t *testing.T) *freshOAuthMCPServer {
	fixture := &freshOAuthMCPServer{}
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	for _, name := range requiredTools {
		mcpServer.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, nil)
	}
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := fixture.server.URL
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			writeOAuthJSON(w, map[string]any{"issuer": base, "authorization_endpoint": base + "/authorize", "token_endpoint": base + "/token", "registration_endpoint": base + "/register", "response_types_supported": []string{"code"}, "token_endpoint_auth_methods_supported": []string{"none"}, "code_challenge_methods_supported": []string{"S256"}})
		case "/register":
			fixture.mu.Lock()
			fixture.registrations++
			fixture.mu.Unlock()
			writeOAuthJSON(w, map[string]any{"client_id": "dynamic-client", "token_endpoint_auth_method": "none"})
		case "/token":
			writeOAuthJSON(w, map[string]any{"access_token": "fresh-bearer", "token_type": "bearer", "expires_in": 3600, "refresh_token": "fresh-refresh"})
		case "/v2/mcp":
			bearer := r.Header.Get("Authorization")
			fixture.mu.Lock()
			fixture.bearers = append(fixture.bearers, r.URL.RawQuery+"|"+bearer)
			fixture.mu.Unlock()
			if bearer != "Bearer fresh-bearer" {
				w.Header().Set("WWW-Authenticate", "Bearer")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			mcpHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func writeOAuthJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (f *freshOAuthMCPServer) registrationCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.registrations
}
func (f *freshOAuthMCPServer) fullBearerSeen() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, value := range f.bearers {
		if strings.HasPrefix(value, "tools=all|Bearer fresh-bearer") {
			return true
		}
	}
	return false
}

func callbackBrowser(t *testing.T, calls *int) func(string) error {
	t.Helper()
	return func(raw string) error {
		*calls++
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		redirect, err := url.Parse(u.Query().Get("redirect_uri"))
		if err != nil {
			return err
		}
		query := redirect.Query()
		query.Set("code", "test-code")
		query.Set("state", u.Query().Get("state"))
		redirect.RawQuery = query.Encode()
		response, err := http.Get(redirect.String())
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil
	}
}

func TestOAuthSavingTokenSourcePersistsRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	record := authRecord{Version: 1, ClientID: "client", Token: &oauth2.Token{AccessToken: "old", RefreshToken: "refresh", Expiry: time.Now().Add(-time.Hour)}}
	if err := saveAuthCache(path, record); err != nil {
		t.Fatal(err)
	}
	source := newSavingTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fresh", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}), path, record)
	if _, err := source.Token(); err != nil {
		t.Fatal(err)
	}
	got, err := loadAuthCache(path)
	if err != nil || got.Token.AccessToken != "fresh" {
		t.Fatalf("cache = %+v, err = %v", got, err)
	}
}

func TestOAuthFreshBootstrapReusesBearerForAllTools(t *testing.T) {
	fixture := newFreshOAuthMCPServer(t)
	var browserCalls int
	session, err := Open(context.Background(), OpenOptions{Endpoint: fixture.server.URL + "/v2/mcp", CachePath: filepath.Join(t.TempDir(), "corum", "auth.json"), Interactive: true, BrowserOpen: callbackBrowser(t, &browserCalls), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if browserCalls != 1 || fixture.registrationCount() != 1 || !fixture.fullBearerSeen() {
		t.Fatalf("browser calls=%d registrations=%d full bearer=%v", browserCalls, fixture.registrationCount(), fixture.fullBearerSeen())
	}
}

func TestOAuthForceReauthIgnoresValidCachedToken(t *testing.T) {
	fixture := newFreshOAuthMCPServer(t)
	path := filepath.Join(t.TempDir(), "corum", "auth.json")
	record := authRecord{Version: 1, ClientID: "cached-client", Token: &oauth2.Token{AccessToken: "fresh-bearer", Expiry: time.Now().Add(time.Hour)}}
	if err := saveAuthCache(path, record); err != nil {
		t.Fatal(err)
	}
	var browserCalls int
	session, err := Open(context.Background(), OpenOptions{Endpoint: fixture.server.URL + "/v2/mcp", CachePath: path, Interactive: true, ForceReauth: true, BrowserOpen: callbackBrowser(t, &browserCalls), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if browserCalls != 1 || fixture.registrationCount() != 1 {
		t.Fatalf("forced reauth browser calls=%d registrations=%d, want a fresh authorization", browserCalls, fixture.registrationCount())
	}
}

func TestOAuthSecondAuthorizationFailsClosedWithoutAnotherBrowser(t *testing.T) {
	fixture := newFreshOAuthMCPServer(t)
	var browserCalls int
	callback, err := newLoopbackCallback(io.Discard, callbackBrowser(t, &browserCalls), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	transaction := newAuthCacheTransaction(filepath.Join(t.TempDir(), "corum", "auth.json"))
	bootstrap, err := newOAuthHandler(context.Background(), authRecord{}, false, callback, fixture.server.Client(), transaction)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.Authorize(context.Background(), oauthRequest(fixture.server.URL+"/v2/mcp"), unauthorizedResponse()); err != nil {
		t.Fatal(err)
	}
	record, ok := transaction.Staged()
	if !ok {
		t.Fatal("bootstrap did not stage its dynamic client record")
	}
	tokenSource, err := bootstrap.TokenSource(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	allTools, err := newAllToolsOAuthHandler(record, tokenSource, fixture.server.Client(), transaction)
	if err != nil {
		t.Fatal(err)
	}
	allToolsTokenSource, err := allTools.TokenSource(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	token, err := allToolsTokenSource.Token()
	if err != nil || token == nil || token.AccessToken != "fresh-bearer" {
		t.Fatalf("all-tools token = %#v, err = %v", token, err)
	}
	err = allTools.Authorize(context.Background(), oauthRequest(fixture.server.URL+"/v2/mcp?tools=all"), unauthorizedResponse())
	if err == nil {
		t.Fatal("second authorization succeeded")
	}
	if browserCalls != 1 || fixture.registrationCount() != 1 {
		t.Fatalf("browser calls=%d registrations=%d, want one initial authorization only", browserCalls, fixture.registrationCount())
	}
}

func oauthRequest(endpoint string) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, endpoint, nil)
	return request
}
func unauthorizedResponse() *http.Response {
	return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"WWW-Authenticate": []string{"Bearer"}}, Body: io.NopCloser(strings.NewReader(""))}
}

func TestOAuthCacheTransactionDefersReplacementUntilCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	previous := authRecord{Version: 1, ClientID: "old-client", Token: &oauth2.Token{AccessToken: "old-access", RefreshToken: "old-refresh"}}
	if err := saveAuthCache(path, previous); err != nil {
		t.Fatal(err)
	}
	transaction := newAuthCacheTransaction(path)
	if err := transaction.Stage(authRecord{Version: 1, ClientID: "new-client", Token: &oauth2.Token{AccessToken: "new-access", RefreshToken: "new-refresh"}}); err != nil {
		t.Fatal(err)
	}
	got, err := loadAuthCache(path)
	if err != nil || got.ClientID != "old-client" {
		t.Fatalf("cache before commit = %+v, err = %v", got, err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err = loadAuthCache(path)
	if err != nil || got.ClientID != "new-client" {
		t.Fatalf("cache after commit = %+v, err = %v", got, err)
	}
}

func TestOAuthCacheTransactionCancellationPreservesPreviousCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	previous := authRecord{Version: 1, ClientID: "old-client", Token: &oauth2.Token{AccessToken: "old-access", RefreshToken: "old-refresh"}}
	if err := saveAuthCache(path, previous); err != nil {
		t.Fatal(err)
	}
	transaction := newAuthCacheTransaction(path)
	if err := transaction.Stage(authRecord{Version: 1, ClientID: "new-client", Token: &oauth2.Token{AccessToken: "new-access", RefreshToken: "new-refresh"}}); err != nil {
		t.Fatal(err)
	}
	transaction.Rollback()
	got, err := loadAuthCache(path)
	if err != nil || got.ClientID != "old-client" || got.Token.AccessToken != "old-access" {
		t.Fatalf("cache after cancellation = %+v, err = %v", got, err)
	}
}

func TestOAuthNonInteractiveMissingCacheNeverStartsBrowser(t *testing.T) {
	called := false
	_, err := Open(context.Background(), OpenOptions{CachePath: filepath.Join(t.TempDir(), "missing.json"), Interactive: false, BrowserOpen: func(string) error { called = true; return nil }})
	if err == nil {
		t.Fatal("Open succeeded without cached authentication")
	}
	if called {
		t.Fatal("noninteractive Open invoked browser authorization")
	}
}

func TestOAuthBootstrapsCanonicalEndpointBeforeAllTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	for _, name := range requiredTools {
		server.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, nil)
	}
	var mu sync.Mutex
	var queries []string
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mu.Lock()
			queries = append(queries, r.URL.RawQuery)
			mu.Unlock()
		}
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := saveAuthCache(path, authRecord{Version: 1, ClientID: "client", RedirectURL: "http://127.0.0.1/callback", Token: &oauth2.Token{AccessToken: "cached", Expiry: time.Now().Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	session, err := Open(context.Background(), OpenOptions{Endpoint: httpServer.URL, CachePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	mu.Lock()
	defer mu.Unlock()
	first, allTools := -1, -1
	for index, query := range queries {
		if query == "" && first < 0 {
			first = index
		}
		if query == "tools=all" && allTools < 0 {
			allTools = index
		}
	}
	if first < 0 || allTools < 0 || first >= allTools {
		t.Fatalf("endpoint query order = %v", queries)
	}
}
