package plugin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"kandev-plugin-azure-devops/internal/auth"
	"kandev-plugin-azure-devops/internal/azure"
	"kandev-plugin-azure-devops/internal/store"
)

const (
	oauthFlowTTL            = 10 * time.Minute
	oauthStateBytes         = 32
	oauthRenewalMargin      = 5 * time.Minute
	oauthPersistenceTimeout = 10 * time.Second
	defaultClientIDConfig   = "default_oauth_client_id"
)

var (
	errOAuthUnavailable = errors.New("Microsoft sign-in needs an application (client) ID")
	errOAuthFlowInvalid = errors.New("the Microsoft sign-in attempt is invalid or expired; start again")
	errOAuthSuperseded  = errors.New("the Azure DevOps connection changed during this Microsoft sign-in; sign in again")
	errNoRefreshToken   = errors.New("Microsoft sign-in returned no refresh token")
	errNoAccess         = errors.New("the signed-in account cannot access this organization")
	errNotOAuth         = errors.New("this workspace has no Microsoft account connection")
)

// OAuthStartInput selects the organization, default project, application, and
// the page origin Microsoft redirects back to.
type OAuthStartInput struct {
	OrganizationURL    string `json:"organizationUrl"`
	DefaultProjectID   string `json:"defaultProjectId"`
	DefaultProjectName string `json:"defaultProjectName"`
	ClientID           string `json:"clientId"`
	RedirectURL        string `json:"redirectUrl"`
}

// OAuthStart is the authorization URL and the state the browser matches.
type OAuthStart struct {
	AuthURL string `json:"authUrl"`
	State   string `json:"state"`
}

// OAuthAvailability tells the settings page whether an installation default
// application exists.
type OAuthAvailability struct {
	DefaultClientID string `json:"defaultClientId,omitempty"`
}

// Availability returns the installation default application, if any.
func (c *Connections) Availability(ctx context.Context) OAuthAvailability {
	return OAuthAvailability{DefaultClientID: c.defaultClientID(ctx)}
}

func (c *Connections) defaultClientID(ctx context.Context) string {
	config, err := c.config(ctx)
	if err != nil {
		return ""
	}
	value, _ := config[defaultClientIDConfig].(string)
	value = strings.ToLower(strings.TrimSpace(value))
	if !auth.IsGUID(value) {
		return ""
	}
	return value
}

// StartOAuth records a pending sign-in for the workspace and actor and returns
// the Microsoft authorization URL.
func (c *Connections) StartOAuth(ctx context.Context, workspaceID, actorID string, input OAuthStartInput) (OAuthStart, error) {
	organizationURL, err := azure.ValidateOrganizationURL(strings.TrimSpace(input.OrganizationURL))
	if err != nil {
		return OAuthStart{}, invalidArgument(err)
	}
	clientID, err := c.signInClientID(ctx, workspaceID, input.ClientID)
	if err != nil {
		return OAuthStart{}, err
	}
	redirectURL := strings.TrimSpace(input.RedirectURL)
	if err := validateRedirectURL(redirectURL); err != nil {
		return OAuthStart{}, invalidArgument(err)
	}
	tenant, err := c.entra.DiscoverTenant(ctx, organizationURL)
	if err != nil {
		return OAuthStart{}, unavailable(err)
	}
	state, err := randomURLToken(oauthStateBytes)
	if err != nil {
		return OAuthStart{}, err
	}
	verifier, err := randomURLToken(oauthStateBytes)
	if err != nil {
		return OAuthStart{}, err
	}
	current, currentFound, err := c.store.Connection(ctx, workspaceID)
	if err != nil {
		return OAuthStart{}, unavailable(err)
	}
	now := c.now().UTC()
	flow := store.Flow{
		StateHash: hashState(state), WorkspaceID: workspaceID, ActorID: actorID,
		OrganizationURL: organizationURL, DefaultProjectID: strings.TrimSpace(input.DefaultProjectID),
		DefaultProjectName: strings.TrimSpace(input.DefaultProjectName), TenantID: tenant, ClientID: clientID,
		PKCEVerifier: verifier, RedirectURL: redirectURL, ExpiresAt: now.Add(oauthFlowTTL),
		ConnectionFound: currentFound, ConnectionGeneration: current.Generation,
	}
	if err := c.flows.Put(ctx, flow, now); err != nil {
		return OAuthStart{}, err
	}
	return OAuthStart{
		AuthURL: c.entra.AuthorizeURL(auth.AuthorizeRequest{
			Tenant: tenant, ClientID: clientID, RedirectURL: redirectURL, State: state, CodeChallenge: pkceChallenge(verifier),
		}),
		State: state,
	}, nil
}

// signInClientID picks the form value, then the workspace's saved
// application, then the installation default.
func (c *Connections) signInClientID(ctx context.Context, workspaceID, requested string) (string, error) {
	if requested = strings.ToLower(strings.TrimSpace(requested)); requested != "" {
		if !auth.IsGUID(requested) {
			return "", invalidArgument(errors.New("the application (client) ID must be a GUID"))
		}
		return requested, nil
	}
	if connection, found, err := c.store.Connection(ctx, workspaceID); err == nil && found && connection.OAuthClientID != "" {
		return connection.OAuthClientID, nil
	}
	if fallback := c.defaultClientID(ctx); fallback != "" {
		return fallback, nil
	}
	return "", invalidArgument(errOAuthUnavailable)
}

// validateRedirectURL accepts HTTPS origins and loopback HTTP origins without
// credentials, query, or fragment. Microsoft also checks the registration.
func validateRedirectURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return errors.New("the redirect URL must be an absolute URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return errors.New("the redirect URL must not include credentials, query, or fragment")
	}
	if parsed.Scheme == "https" || (parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return nil
	}
	return errors.New("the redirect URL must use HTTPS outside localhost")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// CompleteOAuth redeems the authorization code of a pending sign-in started by
// the same actor in the same workspace, checks the account can access the
// organization, and switches the connection to the Microsoft account.
func (c *Connections) CompleteOAuth(ctx context.Context, workspaceID, actorID, code, state string) (ConnectionView, error) {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(state) == "" {
		return ConnectionView{}, invalidArgument(errOAuthFlowInvalid)
	}
	flow, found, err := c.flows.Claim(ctx, workspaceID, hashState(state), c.now().UTC())
	if err != nil {
		return ConnectionView{}, err
	}
	if !found || flow.WorkspaceID != workspaceID || flow.ActorID != actorID {
		return ConnectionView{}, invalidArgument(errOAuthFlowInvalid)
	}
	token, err := c.entra.ExchangeCode(ctx, flow.TenantID, flow.ClientID, code, flow.RedirectURL, flow.PKCEVerifier)
	if err != nil {
		return ConnectionView{}, signInError(err)
	}
	if token.RefreshToken == "" {
		return ConnectionView{}, invalidArgument(errNoRefreshToken)
	}
	identity, err := c.newClient(flow.OrganizationURL, azure.NewBearerAuthorizer(token.AccessToken)).TestAuth(ctx)
	if err != nil {
		return ConnectionView{}, invalidArgument(fmt.Errorf("%w: %s", errNoAccess, probeError(err)))
	}
	if identity == nil || !identity.OK {
		return ConnectionView{}, invalidArgument(errNoAccess)
	}
	if err := c.replaceWithOAuth(ctx, flow, accountName(identity), token); err != nil {
		return ConnectionView{}, err
	}
	c.RecordHealth(ctx, workspaceID)
	return c.Get(ctx, workspaceID)
}

func signInError(err error) error {
	var entraErr *auth.EntraError
	if errors.As(err, &entraErr) {
		return invalidArgument(fmt.Errorf("Microsoft sign-in failed: %s", entraErr.Code))
	}
	return unavailable(err)
}

func accountName(identity *azure.TestConnectionResult) string {
	if identity.DisplayName != "" {
		return identity.DisplayName
	}
	return identity.Email
}

// replaceWithOAuth installs the new tokens and connection under the workspace
// lock, so an in-flight renewal for the previous credential cannot overwrite
// them. On failure the previous Microsoft tokens are restored.
func (c *Connections) replaceWithOAuth(ctx context.Context, flow store.Flow, account string, token *auth.Token) error {
	unlock := c.locks.lock(flow.WorkspaceID)
	defer unlock()
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), oauthPersistenceTimeout)
	defer cancel()
	workspaceID := flow.WorkspaceID
	previous, found, err := c.store.Connection(persistCtx, workspaceID)
	if err != nil {
		return err
	}
	// A deletion, sign-out, or other save while the code was redeemed wins:
	// the claimed flow cannot be purged, so it is checked here.
	if found != flow.ConnectionFound || previous.Generation != flow.ConnectionGeneration {
		return invalidArgument(errOAuthSuperseded)
	}
	snapshot, err := c.snapshotOAuthTokens(persistCtx, workspaceID)
	if err != nil {
		return err
	}
	expiresAt := c.now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
	next := store.Connection{
		OrganizationURL: flow.OrganizationURL, AuthMethod: store.AuthMethodOAuth,
		DefaultProjectID: flow.DefaultProjectID, DefaultProjectName: flow.DefaultProjectName,
		Generation: previous.Generation + 1, UpdatedAt: c.now().UTC(),
		OAuthClientID: flow.ClientID, OAuthTenantID: flow.TenantID, OAuthAccount: account, TokenExpiresAt: &expiresAt,
	}
	if err := c.storeOAuthTokens(persistCtx, workspaceID, token.RefreshToken, token.AccessToken); err != nil {
		return errors.Join(err, c.restoreOAuthTokens(persistCtx, workspaceID, snapshot))
	}
	if err := c.store.SaveConnection(persistCtx, workspaceID, next); err != nil {
		return errors.Join(err, c.restoreOAuthTokens(persistCtx, workspaceID, snapshot))
	}
	_ = c.store.DeletePAT(persistCtx, workspaceID)
	return nil
}

// SignOut removes the Microsoft tokens and keeps the settings for a later
// sign-in. It does not end the Microsoft session or revoke tokens there.
func (c *Connections) SignOut(ctx context.Context, workspaceID string) (ConnectionView, error) {
	if err := c.signOut(ctx, workspaceID); err != nil {
		return ConnectionView{}, err
	}
	return c.Get(ctx, workspaceID)
}

func (c *Connections) signOut(ctx context.Context, workspaceID string) error {
	unlock := c.locks.lock(workspaceID)
	defer unlock()
	connection, found, err := c.store.Connection(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !found || connection.AuthMethod != store.AuthMethodOAuth {
		return invalidArgument(errNotOAuth)
	}
	for _, key := range []string{store.OAuthRefreshKey(workspaceID), store.OAuthAccessKey(workspaceID)} {
		if err := c.store.DeleteSecret(ctx, key); err != nil {
			return err
		}
	}
	connection.ReauthRequired = true
	connection.OAuthAccount = ""
	connection.TokenExpiresAt = nil
	connection.Generation++
	connection.UpdatedAt = c.now().UTC()
	return c.store.SaveConnection(ctx, workspaceID, connection)
}

// oauthAuthorizer reads the access token of the connection its client was
// built for. Renewal within that connection keeps working; once the
// connection is replaced, signed out, or deleted, requests fail instead of
// sending another connection's token to this client's organization.
type oauthAuthorizer struct {
	connections *Connections
	workspaceID string
	bound       store.Connection
}

func (a oauthAuthorizer) Header(ctx context.Context) (string, error) {
	token, _, err := a.connections.boundAccessToken(ctx, a.workspaceID, a.bound)
	if err != nil {
		return "", err
	}
	return azure.BearerPrefix + token, nil
}

func (a oauthAuthorizer) Renew(ctx context.Context, rejectedHeader string) (bool, error) {
	return a.connections.renewAccessToken(ctx, a.workspaceID, a.bound, strings.TrimPrefix(rejectedHeader, azure.BearerPrefix))
}

func (a oauthAuthorizer) Redact(text string) string {
	token, _, err := a.connections.store.Secret(context.Background(), store.OAuthAccessKey(a.workspaceID))
	if err != nil {
		return text
	}
	return azure.RedactToken(text, token)
}

// boundAccessToken returns an access token of the bound connection, valid
// for at least the renewal margin, and when it expires. It reads under the
// workspace lock, which every credential change holds while it writes the
// token and the connection, so the token always belongs to the connection
// that was checked.
func (c *Connections) boundAccessToken(ctx context.Context, workspaceID string, bound store.Connection) (string, time.Time, error) {
	unlock := c.locks.lock(workspaceID)
	defer unlock()
	connection, err := c.boundConnection(ctx, workspaceID, bound)
	if err != nil {
		return "", time.Time{}, err
	}
	if connection.TokenExpiresAt == nil || connection.TokenExpiresAt.Sub(c.now()) <= oauthRenewalMargin {
		if err := c.refreshLocked(ctx, workspaceID); err != nil {
			return "", time.Time{}, err
		}
		if connection, err = c.boundConnection(ctx, workspaceID, bound); err != nil {
			return "", time.Time{}, err
		}
	}
	token, ok, err := c.store.Secret(ctx, store.OAuthAccessKey(workspaceID))
	if err != nil || !ok || connection.TokenExpiresAt == nil {
		return "", time.Time{}, azure.ErrReauthRequired
	}
	return token, *connection.TokenExpiresAt, nil
}

// boundConnection returns the workspace connection while it is still the
// bound one: same organization, sign-in method, and credential generation,
// and usable. Callers hold the workspace lock.
func (c *Connections) boundConnection(ctx context.Context, workspaceID string, bound store.Connection) (store.Connection, error) {
	connection, found, err := c.store.Connection(ctx, workspaceID)
	if err != nil {
		return store.Connection{}, err
	}
	if !found || connection.ReauthRequired || connection.Generation != bound.Generation ||
		connection.AuthMethod != bound.AuthMethod || connection.OrganizationURL != bound.OrganizationURL {
		return store.Connection{}, errConnectionChanged
	}
	return connection, nil
}

// renewAccessToken replaces a token Azure rejected. When a concurrent caller
// already replaced it, it does not renew again, which would spend the freshly
// rotated refresh token.
func (c *Connections) renewAccessToken(ctx context.Context, workspaceID string, bound store.Connection, rejected string) (bool, error) {
	unlock := c.locks.lock(workspaceID)
	defer unlock()
	if _, err := c.boundConnection(ctx, workspaceID, bound); err != nil {
		return false, err
	}
	if current, ok, err := c.store.Secret(ctx, store.OAuthAccessKey(workspaceID)); err == nil && ok && rejected != "" && current != rejected {
		return true, nil
	}
	if err := c.refreshLocked(ctx, workspaceID); err != nil {
		return false, err
	}
	return true, nil
}

// oauthConnection returns a usable Microsoft account connection.
func (c *Connections) oauthConnection(ctx context.Context, workspaceID string) (store.Connection, error) {
	connection, found, err := c.store.Connection(ctx, workspaceID)
	if err != nil {
		return store.Connection{}, err
	}
	if !found || connection.AuthMethod != store.AuthMethodOAuth {
		return store.Connection{}, azure.ErrNotConfigured
	}
	if connection.ReauthRequired {
		return store.Connection{}, azure.ErrReauthRequired
	}
	return connection, nil
}

// refreshLocked redeems the refresh token with the application that signed
// in. Callers hold the workspace lock.
func (c *Connections) refreshLocked(ctx context.Context, workspaceID string) error {
	connection, err := c.oauthConnection(ctx, workspaceID)
	if err != nil {
		return err
	}
	clientID := connection.OAuthClientID
	if clientID == "" {
		clientID = c.defaultClientID(ctx)
	}
	if clientID == "" {
		return errOAuthUnavailable
	}
	refreshToken, ok, err := c.store.Secret(ctx, store.OAuthRefreshKey(workspaceID))
	if err != nil {
		return err
	}
	if !ok {
		return c.requireSignIn(ctx, workspaceID)
	}
	tenant := connection.OAuthTenantID
	if tenant == "" {
		tenant = auth.DefaultTenant
	}
	token, err := c.entra.Refresh(ctx, tenant, clientID, refreshToken)
	if err != nil {
		var entraErr *auth.EntraError
		if errors.As(err, &entraErr) && entraErr.RequiresSignIn() {
			return c.requireSignIn(ctx, workspaceID)
		}
		return fmt.Errorf("renew Microsoft access token: %w", err)
	}
	return c.persistRenewedToken(ctx, workspaceID, refreshToken, token)
}

// persistRenewedToken stores the rotated refresh token before the access
// token and keeps the previous refresh token when Microsoft returns none.
func (c *Connections) persistRenewedToken(ctx context.Context, workspaceID, previousRefreshToken string, token *auth.Token) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), oauthPersistenceTimeout)
	defer cancel()
	refreshToken := token.RefreshToken
	if refreshToken == "" {
		refreshToken = previousRefreshToken
	}
	if err := c.storeOAuthTokens(persistCtx, workspaceID, refreshToken, token.AccessToken); err != nil {
		return err
	}
	connection, found, err := c.store.Connection(persistCtx, workspaceID)
	if err != nil || !found {
		return errors.Join(azure.ErrNotConfigured, err)
	}
	expiresAt := c.now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
	connection.TokenExpiresAt = &expiresAt
	return c.store.SaveConnection(persistCtx, workspaceID, connection)
}

func (c *Connections) requireSignIn(ctx context.Context, workspaceID string) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), oauthPersistenceTimeout)
	defer cancel()
	connection, found, err := c.store.Connection(persistCtx, workspaceID)
	if err != nil || !found {
		return errors.Join(azure.ErrReauthRequired, err)
	}
	connection.ReauthRequired = true
	if err := c.store.SaveConnection(persistCtx, workspaceID, connection); err != nil {
		return errors.Join(azure.ErrReauthRequired, err)
	}
	return azure.ErrReauthRequired
}

// storeOAuthTokens writes the refresh token first: a failure after it leaves
// a credential that can still renew.
func (c *Connections) storeOAuthTokens(ctx context.Context, workspaceID, refreshToken, accessToken string) error {
	if err := c.store.SetSecret(ctx, store.OAuthRefreshKey(workspaceID), refreshToken); err != nil {
		return err
	}
	return c.store.SetSecret(ctx, store.OAuthAccessKey(workspaceID), accessToken)
}

type tokenSnapshot map[string]string

func (c *Connections) snapshotOAuthTokens(ctx context.Context, workspaceID string) (tokenSnapshot, error) {
	snapshot := tokenSnapshot{}
	for _, key := range []string{store.OAuthRefreshKey(workspaceID), store.OAuthAccessKey(workspaceID)} {
		value, ok, err := c.store.Secret(ctx, key)
		if err != nil {
			return nil, err
		}
		if ok {
			snapshot[key] = value
		}
	}
	return snapshot, nil
}

func (c *Connections) restoreOAuthTokens(ctx context.Context, workspaceID string, snapshot tokenSnapshot) error {
	var result error
	for _, key := range []string{store.OAuthRefreshKey(workspaceID), store.OAuthAccessKey(workspaceID)} {
		if value, ok := snapshot[key]; ok {
			result = errors.Join(result, c.store.SetSecret(ctx, key, value))
			continue
		}
		result = errors.Join(result, c.store.DeleteSecret(ctx, key))
	}
	return result
}

func hashState(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}

func randomURLToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
