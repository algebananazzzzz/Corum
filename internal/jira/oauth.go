package jira

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

const rovoMCPURL = "https://mcp.atlassian.com/v2/mcp"

// rovoHTTPTimeout bounds every OAuth and MCP request so a stalled endpoint
// cannot hang the process without a deadline.
const rovoHTTPTimeout = 5 * time.Minute

// LoginRequired identifies requests that would otherwise need browser OAuth.
var LoginRequired = errors.New("Jira session is missing or revoked; run corum auth jira")

// OpenOptions permits the CLI and tests to choose interaction without exposing
// the internal OAuth record.
type OpenOptions struct {
	Endpoint         string
	CachePath        string
	Interactive      bool
	ForceReauth      bool
	Out              io.Writer
	BrowserOpen      func(string) error
	AuthorizationURL func(string)
	Timeout          time.Duration
	HTTPClient       *http.Client
}

func (o OpenOptions) endpoint() string {
	if o.Endpoint != "" {
		return strings.TrimSuffix(o.Endpoint, "?tools=all")
	}
	return rovoMCPURL
}

// Open connects to the canonical Rovo endpoint first, then opens the expanded
// tool catalogue with exactly the same OAuth handler/token source.
func Open(ctx context.Context, options OpenOptions) (*RovoSession, error) {
	options = withDefaults(options)
	path := options.CachePath
	if path == "" {
		return nil, errors.New("Jira authentication cache requires a project path")
	}
	interactive := options.Interactive
	if options.ForceReauth && !interactive {
		return nil, LoginRequired
	}
	record, cacheErr := loadAuthCache(path)
	if cacheErr != nil && !os.IsNotExist(cacheErr) {
		return nil, cacheErr
	}
	if os.IsNotExist(cacheErr) && !interactive {
		return nil, LoginRequired
	}
	if options.ForceReauth {
		// An explicit login replaces the cached account, so bootstrap the
		// authorization-code flow without the cached client or token.
		record = authRecord{}
		cacheErr = nil
	}
	var callback *loopbackCallback
	var err error
	if interactive {
		out := options.Out
		if out == nil {
			out = io.Discard
		}
		timeout := options.Timeout
		if timeout == 0 {
			timeout = 5 * time.Minute
		}
		opener := options.BrowserOpen
		if opener == nil {
			opener = defaultBrowserOpen
		}
		callback, err = newLoopbackCallback(out, opener, options.AuthorizationURL, timeout)
		if err != nil {
			return nil, fmt.Errorf("start Jira authorization: %w", err)
		}
	}
	closeCallback := true
	defer func() {
		if closeCallback && callback != nil {
			_ = callback.Close()
		}
	}()
	cacheTransaction := newAuthCacheTransaction(path)
	cached := cacheErr == nil && !options.ForceReauth
	bootstrapHandler, err := newOAuthHandler(ctx, record, cached, callback, options.HTTPClient, cacheTransaction)
	if err != nil {
		return nil, redactOAuthError(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "corum", Version: "1"}, nil)
	canonical, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: options.endpoint(), HTTPClient: options.HTTPClient, OAuthHandler: bootstrapHandler, DisableStandaloneSSE: true}, nil)
	if err != nil {
		return nil, redactOAuthError(err)
	}
	bootstrapRecord, ok := cacheTransaction.Staged()
	if !ok {
		_ = canonical.Close()
		return nil, errors.New("could not connect to Atlassian")
	}
	bootstrapTokenSource, err := bootstrapHandler.TokenSource(ctx)
	if err != nil || bootstrapTokenSource == nil {
		_ = canonical.Close()
		return nil, errors.New("could not connect to Atlassian")
	}
	fullHandler, err := newAllToolsOAuthHandler(bootstrapRecord, bootstrapTokenSource, options.HTTPClient, cacheTransaction)
	if err != nil {
		_ = canonical.Close()
		return nil, redactOAuthError(err)
	}
	if err := canonical.Close(); err != nil {
		return nil, redactOAuthError(err)
	}
	full, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: options.endpoint() + "?tools=all", HTTPClient: options.HTTPClient, OAuthHandler: fullHandler, DisableStandaloneSSE: true}, nil)
	if err != nil {
		return nil, redactOAuthError(err)
	}
	session := &RovoSession{session: full, callback: callback}
	if _, err := session.toolsFor(ctx); err != nil {
		_ = full.Close()
		return nil, err
	}
	if err := cacheTransaction.Commit(); err != nil {
		_ = full.Close()
		return nil, redactOAuthError(err)
	}
	closeCallback = false
	return session, nil
}

func newOAuthHandler(ctx context.Context, record authRecord, cached bool, callback *loopbackCallback, client *http.Client, cacheTransaction *authCacheTransaction) (*auth.AuthorizationCodeHandler, error) {
	if callback == nil && !cached {
		return nil, LoginRequired
	}
	redirect := record.RedirectURL
	if callback != nil {
		redirect = callback.URL()
	}
	config := &auth.AuthorizationCodeHandlerConfig{
		RedirectURL:         redirect,
		Client:              client,
		RequestRefreshToken: true,
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			if callback == nil {
				return nil, LoginRequired
			}
			value, err := callback.Fetch(ctx, args.URL)
			if err != nil {
				return nil, err
			}
			record.Issuer = value.Iss
			return &auth.AuthorizationResult{Code: value.Code, State: value.State, Iss: value.Iss}, nil
		},
	}
	if cached && record.ClientID != "" {
		credentials := &oauthex.ClientCredentials{ClientID: record.ClientID, Issuer: record.Issuer}
		if record.ClientSecret != "" {
			credentials.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: record.ClientSecret}
		}
		config.PreregisteredClient = credentials
		config.InitialTokenSource = newTransactionalSavingTokenSource(tokenSourceFor(ctx, record), record, cacheTransaction.Stage)
	} else {
		config.DynamicClientRegistrationConfig = &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
			ClientName: "Corum", RedirectURIs: []string{redirect}, GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"}, TokenEndpointAuthMethod: "none",
		}}
	}
	config.NewTokenSource = func(sourceCtx context.Context, oauthConfig *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
		updated := recordFromOAuthConfig(record, oauthConfig, token)
		if err := cacheTransaction.Stage(updated); err != nil {
			return nil, err
		}
		return newTransactionalSavingTokenSource(oauthConfig.TokenSource(sourceCtx, token), updated, cacheTransaction.Stage), nil
	}
	return auth.NewAuthorizationCodeHandler(config)
}

// Reusing the bootstrap handler could invoke its one-shot callback again.
func newAllToolsOAuthHandler(record authRecord, initial oauth2.TokenSource, client *http.Client, cacheTransaction *authCacheTransaction) (*auth.AuthorizationCodeHandler, error) {
	if record.ClientID == "" || record.Token == nil || initial == nil {
		return nil, LoginRequired
	}
	credentials := &oauthex.ClientCredentials{ClientID: record.ClientID, Issuer: record.Issuer}
	if record.ClientSecret != "" {
		credentials.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: record.ClientSecret}
	}
	config := &auth.AuthorizationCodeHandlerConfig{
		RedirectURL:         record.RedirectURL,
		PreregisteredClient: credentials,
		InitialTokenSource:  initial,
		Client:              client,
		RequestRefreshToken: true,
		AuthorizationCodeFetcher: func(context.Context, *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			return nil, LoginRequired
		},
	}
	config.NewTokenSource = func(sourceCtx context.Context, oauthConfig *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
		updated := recordFromOAuthConfig(record, oauthConfig, token)
		if err := cacheTransaction.Stage(updated); err != nil {
			return nil, err
		}
		return newTransactionalSavingTokenSource(oauthConfig.TokenSource(sourceCtx, token), updated, cacheTransaction.Stage), nil
	}
	return auth.NewAuthorizationCodeHandler(config)
}

func recordFromOAuthConfig(record authRecord, oauthConfig *oauth2.Config, token *oauth2.Token) authRecord {
	updated := record
	updated.Version = authCacheVersion
	updated.ClientID = oauthConfig.ClientID
	updated.ClientSecret = oauthConfig.ClientSecret
	updated.AuthURL = oauthConfig.Endpoint.AuthURL
	updated.TokenURL = oauthConfig.Endpoint.TokenURL
	updated.AuthStyle = int(oauthConfig.Endpoint.AuthStyle)
	updated.RedirectURL = oauthConfig.RedirectURL
	updated.Scopes = append([]string(nil), oauthConfig.Scopes...)
	updated.Token = token
	return updated
}

func tokenSourceFor(ctx context.Context, record authRecord) oauth2.TokenSource {
	config := &oauth2.Config{ClientID: record.ClientID, ClientSecret: record.ClientSecret, Endpoint: oauth2.Endpoint{AuthURL: record.AuthURL, TokenURL: record.TokenURL, AuthStyle: oauth2.AuthStyle(record.AuthStyle)}, RedirectURL: record.RedirectURL, Scopes: append([]string(nil), record.Scopes...)}
	return config.TokenSource(ctx, record.Token)
}

type savingTokenSource struct {
	source oauth2.TokenSource
	record authRecord
	save   func(authRecord) error
}

func newSavingTokenSource(source oauth2.TokenSource, path string, record authRecord) oauth2.TokenSource {
	return newTransactionalSavingTokenSource(source, record, func(updated authRecord) error { return saveAuthCache(path, updated) })
}

func newTransactionalSavingTokenSource(source oauth2.TokenSource, record authRecord, save func(authRecord) error) oauth2.TokenSource {
	return &savingTokenSource{source: source, record: record, save: save}
}
func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.source.Token()
	if err != nil {
		return nil, err
	}
	updated := s.record
	updated.Token = token
	if err := s.save(updated); err != nil {
		return nil, err
	}
	s.record = updated
	return token, nil
}

type authCacheTransaction struct {
	path      string
	pending   *authRecord
	committed bool
}

func newAuthCacheTransaction(path string) *authCacheTransaction {
	return &authCacheTransaction{path: path}
}

func (t *authCacheTransaction) Stage(record authRecord) error {
	if t.committed {
		return saveAuthCache(t.path, record)
	}
	copy := record
	t.pending = &copy
	return nil
}

func (t *authCacheTransaction) Commit() error {
	if t.pending != nil {
		if err := saveAuthCache(t.path, *t.pending); err != nil {
			return err
		}
	}
	t.committed = true
	return nil
}

func (t *authCacheTransaction) Rollback() { t.pending = nil }

func (t *authCacheTransaction) Staged() (authRecord, bool) {
	if t.pending == nil {
		return authRecord{}, false
	}
	return *t.pending, true
}

// withDefaults fills the bounded default HTTP client and validation surface
// without changing an explicitly supplied client.
func withDefaults(options OpenOptions) OpenOptions {
	if options.HTTPClient == nil {
		options.HTTPClient = defaultHTTPClient()
	}
	return options
}

// defaultHTTPClient returns a finite-timeout client for OAuth and MCP traffic.
func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: rovoHTTPTimeout}
}

func redactOAuthError(err error) error {
	if errors.Is(err, LoginRequired) || errors.Is(err, ErrAuthorizationTimeout) {
		return err
	}
	return errors.New("could not connect to Atlassian")
}
