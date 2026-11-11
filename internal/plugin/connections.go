package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/auth"
	"kandev-plugin-azure-devops/internal/azure"
	"kandev-plugin-azure-devops/internal/store"
)

var (
	errNotConfigured         = errors.New("Azure DevOps is not configured for this workspace")
	errPATRequired           = errors.New("a personal access token is required")
	errReauthRequired        = errors.New("Azure DevOps credentials must be entered again for this workspace")
	errSignInForOrganization = errors.New("sign in with Microsoft for this organization")
)

// ConnectionInput is the browser-supplied connection form. An empty PAT keeps
// the stored token. AuthMethod "oauth" only updates the default project of a
// signed-in Microsoft account connection.
type ConnectionInput struct {
	OrganizationURL    string `json:"organizationUrl"`
	DefaultProjectID   string `json:"defaultProjectId"`
	DefaultProjectName string `json:"defaultProjectName"`
	AuthMethod         string `json:"authMethod"`
	PAT                string `json:"pat"`
}

// ConnectionView is the credential-free connection returned to the browser.
type ConnectionView struct {
	Configured         bool       `json:"configured"`
	OrganizationURL    string     `json:"organizationUrl,omitempty"`
	DefaultProjectID   string     `json:"defaultProjectId,omitempty"`
	DefaultProjectName string     `json:"defaultProjectName,omitempty"`
	AuthMethod         string     `json:"authMethod,omitempty"`
	HasSecret          bool       `json:"hasSecret"`
	ReauthRequired     bool       `json:"reauthRequired"`
	OAuthAccount       string     `json:"oauthAccount,omitempty"`
	OAuthClientID      string     `json:"oauthClientId,omitempty"`
	LastCheckedAt      *time.Time `json:"lastCheckedAt,omitempty"`
	LastOK             bool       `json:"lastOk"`
	LastError          string     `json:"lastError,omitempty"`
}

// Connections owns workspace connection lifecycle, Microsoft sign-in, token
// renewal, and health. Every credential write holds the workspace lock.
type Connections struct {
	store     *store.Store
	flows     *store.Flows
	newClient ClientFactory
	entra     auth.IdentityProvider
	config    func(context.Context) (map[string]any, error)
	locks     workspaceLocks
	now       func() time.Time
}

// ConnectionsOptions configures optional collaborators.
type ConnectionsOptions struct {
	NewClient ClientFactory
	Entra     auth.IdentityProvider
	// Config returns the plugin's operator settings (default_oauth_client_id).
	Config func(context.Context) (map[string]any, error)
	// StateKeys lists a workspace's state keys, for purging sign-in flows.
	StateKeys func(ctx context.Context, workspaceID string) ([]string, error)
}

// NewConnections returns a connection service.
func NewConnections(s *store.Store, options ConnectionsOptions) *Connections {
	if options.NewClient == nil {
		options.NewClient = DefaultClientFactory
	}
	if options.Entra == nil {
		options.Entra = auth.NewEntraClient()
	}
	if options.Config == nil {
		options.Config = func(context.Context) (map[string]any, error) { return map[string]any{}, nil }
	}
	return &Connections{
		store: s, flows: store.NewFlows(s, options.StateKeys), newClient: options.NewClient,
		entra: options.Entra, config: options.Config, now: time.Now,
	}
}

// Get returns the credential-free connection view.
func (c *Connections) Get(ctx context.Context, workspaceID string) (ConnectionView, error) {
	connection, found, err := c.store.Connection(ctx, workspaceID)
	if err != nil || !found {
		return ConnectionView{}, err
	}
	secretKey := store.PATKey(workspaceID)
	if connection.AuthMethod == store.AuthMethodOAuth {
		secretKey = store.OAuthRefreshKey(workspaceID)
	}
	_, hasSecret, err := c.store.Secret(ctx, secretKey)
	if err != nil {
		return ConnectionView{}, err
	}
	view := ConnectionView{
		Configured: true, OrganizationURL: connection.OrganizationURL,
		DefaultProjectID: connection.DefaultProjectID, DefaultProjectName: connection.DefaultProjectName,
		AuthMethod: connection.AuthMethod, HasSecret: hasSecret, ReauthRequired: connection.ReauthRequired,
		OAuthAccount: connection.OAuthAccount, OAuthClientID: connection.OAuthClientID,
	}
	health, found, err := c.store.Health(ctx, workspaceID)
	if err != nil {
		return ConnectionView{}, err
	}
	if found && health.Generation == connection.Generation {
		checkedAt := health.CheckedAt
		view.LastCheckedAt, view.LastOK, view.LastError = &checkedAt, health.OK, health.Error
	}
	return view, nil
}

// Save validates and stores a connection, then records a fresh health probe
// when the credential or organization changed.
func (c *Connections) Save(ctx context.Context, workspaceID string, input ConnectionInput) (ConnectionView, error) {
	organizationURL, err := azure.ValidateOrganizationURL(strings.TrimSpace(input.OrganizationURL))
	if err != nil {
		return ConnectionView{}, invalidArgument(err)
	}
	var changed bool
	if strings.TrimSpace(input.AuthMethod) == store.AuthMethodOAuth {
		err = c.saveOAuthSettings(ctx, workspaceID, organizationURL, input)
	} else {
		changed, err = c.savePAT(ctx, workspaceID, organizationURL, input)
	}
	if err != nil {
		return ConnectionView{}, err
	}
	if changed {
		c.RecordHealth(ctx, workspaceID)
	}
	return c.Get(ctx, workspaceID)
}

// saveOAuthSettings changes only the default project. Another organization
// needs a new sign-in, because the tokens belong to its directory.
func (c *Connections) saveOAuthSettings(ctx context.Context, workspaceID, organizationURL string, input ConnectionInput) error {
	unlock := c.locks.lock(workspaceID)
	defer unlock()
	connection, found, err := c.store.Connection(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !found || connection.AuthMethod != store.AuthMethodOAuth || connection.OrganizationURL != organizationURL {
		return invalidArgument(errSignInForOrganization)
	}
	connection.DefaultProjectID = strings.TrimSpace(input.DefaultProjectID)
	connection.DefaultProjectName = strings.TrimSpace(input.DefaultProjectName)
	connection.UpdatedAt = c.now().UTC()
	return c.store.SaveConnection(ctx, workspaceID, connection)
}

// savePAT stores a PAT connection. Replacing a Microsoft account connection
// removes its tokens only after the PAT connection is saved.
func (c *Connections) savePAT(ctx context.Context, workspaceID, organizationURL string, input ConnectionInput) (bool, error) {
	unlock := c.locks.lock(workspaceID)
	defer unlock()
	previous, found, err := c.store.Connection(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	previousPAT, hadPAT, err := c.store.PAT(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	wasOAuth := found && previous.AuthMethod == store.AuthMethodOAuth
	pat := strings.TrimSpace(input.PAT)
	if pat == "" && (!hadPAT || wasOAuth || (found && previous.ReauthRequired)) {
		return false, invalidArgument(errPATRequired)
	}
	changed := !found || wasOAuth || previous.OrganizationURL != organizationURL || (pat != "" && pat != previousPAT)
	next := store.Connection{
		OrganizationURL: organizationURL, AuthMethod: store.AuthMethodPAT,
		DefaultProjectID:   strings.TrimSpace(input.DefaultProjectID),
		DefaultProjectName: strings.TrimSpace(input.DefaultProjectName),
		Generation:         previous.Generation, UpdatedAt: c.now().UTC(),
	}
	if changed {
		next.Generation++
	}
	if pat != "" {
		if err := c.store.SetPAT(ctx, workspaceID, pat); err != nil {
			return false, err
		}
	}
	if err := c.store.SaveConnection(ctx, workspaceID, next); err != nil {
		return false, errors.Join(err, c.restorePAT(ctx, workspaceID, pat != "", previousPAT, hadPAT))
	}
	if wasOAuth {
		c.discardOAuthTokens(ctx, workspaceID)
	}
	return changed, nil
}

func (c *Connections) restorePAT(ctx context.Context, workspaceID string, replaced bool, previous string, hadPrevious bool) error {
	if !replaced {
		return nil
	}
	if hadPrevious {
		return c.store.SetPAT(ctx, workspaceID, previous)
	}
	return c.store.DeletePAT(ctx, workspaceID)
}

// discardOAuthTokens removes replaced Microsoft tokens. Readers follow the
// connection's auth method, so a leftover token is never used.
func (c *Connections) discardOAuthTokens(ctx context.Context, workspaceID string) {
	_ = c.store.DeleteSecret(ctx, store.OAuthRefreshKey(workspaceID))
	_ = c.store.DeleteSecret(ctx, store.OAuthAccessKey(workspaceID))
}

// Test probes the submitted organization and token without persisting them.
// Without a token it probes the stored credential of the stored organization.
func (c *Connections) Test(ctx context.Context, workspaceID string, input ConnectionInput) (*azure.TestConnectionResult, error) {
	organizationURL, err := azure.ValidateOrganizationURL(strings.TrimSpace(input.OrganizationURL))
	if err != nil {
		return nil, invalidArgument(err)
	}
	var client Client
	if pat := strings.TrimSpace(input.PAT); pat != "" {
		client = c.newClient(organizationURL, azure.NewPATAuthorizer(pat))
	} else {
		stored, connection, err := c.Client(ctx, workspaceID)
		if err != nil || connection.OrganizationURL != organizationURL {
			if connection.AuthMethod == store.AuthMethodOAuth {
				return nil, invalidArgument(errSignInForOrganization)
			}
			return nil, invalidArgument(errPATRequired)
		}
		client = stored
	}
	result, err := client.TestAuth(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return &azure.TestConnectionResult{OK: false, Error: probeError(err)}, nil
	}
	return result, nil
}

// Copy replaces the target workspace connection with the source's non-secret
// settings. Tokens are never copied: the action context verifies only the
// target workspace. A PAT target keeps its own token for the same
// organization; a Microsoft account target always signs in again.
func (c *Connections) Copy(ctx context.Context, targetWorkspaceID, sourceWorkspaceID string) (ConnectionView, error) {
	sourceWorkspaceID = strings.TrimSpace(sourceWorkspaceID)
	if sourceWorkspaceID == "" || sourceWorkspaceID == targetWorkspaceID {
		return ConnectionView{}, invalidArgument(errors.New("choose a different source workspace"))
	}
	source, found, err := c.store.Connection(ctx, sourceWorkspaceID)
	if err != nil {
		return ConnectionView{}, err
	}
	if !found {
		return ConnectionView{}, notFound(errors.New("the source workspace has no Azure DevOps connection"))
	}
	if err := c.copyInto(ctx, targetWorkspaceID, source); err != nil {
		return ConnectionView{}, err
	}
	return c.Get(ctx, targetWorkspaceID)
}

func (c *Connections) copyInto(ctx context.Context, targetWorkspaceID string, source store.Connection) error {
	unlock := c.locks.lock(targetWorkspaceID)
	defer unlock()
	target, targetFound, err := c.store.Connection(ctx, targetWorkspaceID)
	if err != nil {
		return err
	}
	_, targetHasPAT, err := c.store.PAT(ctx, targetWorkspaceID)
	if err != nil {
		return err
	}
	keepPAT := source.AuthMethod != store.AuthMethodOAuth && targetFound && targetHasPAT &&
		target.AuthMethod == store.AuthMethodPAT && !target.ReauthRequired && target.OrganizationURL == source.OrganizationURL
	next := store.Connection{
		OrganizationURL: source.OrganizationURL, AuthMethod: source.AuthMethod,
		DefaultProjectID: source.DefaultProjectID, DefaultProjectName: source.DefaultProjectName,
		ReauthRequired: !keepPAT, Generation: target.Generation, UpdatedAt: c.now().UTC(),
	}
	if next.AuthMethod == "" {
		next.AuthMethod = store.AuthMethodPAT
	}
	if next.AuthMethod == store.AuthMethodOAuth {
		next.OAuthClientID = source.OAuthClientID
	}
	if !keepPAT {
		next.Generation++
		if err := c.store.DeletePAT(ctx, targetWorkspaceID); err != nil {
			return err
		}
		c.discardOAuthTokens(ctx, targetWorkspaceID)
	}
	return c.store.SaveConnection(ctx, targetWorkspaceID, next)
}

// Delete removes the connection, its tokens, and pending sign-ins.
func (c *Connections) Delete(ctx context.Context, workspaceID string) error {
	unlock := c.locks.lock(workspaceID)
	defer unlock()
	if err := c.store.DeletePAT(ctx, workspaceID); err != nil {
		return err
	}
	for _, key := range []string{store.OAuthRefreshKey(workspaceID), store.OAuthAccessKey(workspaceID)} {
		if err := c.store.DeleteSecret(ctx, key); err != nil {
			return err
		}
	}
	c.flows.Purge(ctx, workspaceID, c.now(), true)
	return c.store.DeleteConnection(ctx, workspaceID)
}

// Client returns an authenticated client for a usable workspace connection.
func (c *Connections) Client(ctx context.Context, workspaceID string) (Client, store.Connection, error) {
	connection, found, err := c.store.Connection(ctx, workspaceID)
	if err != nil {
		return nil, store.Connection{}, unavailable(err)
	}
	if !found {
		return nil, store.Connection{}, failedPrecondition(errNotConfigured)
	}
	if connection.ReauthRequired {
		return nil, connection, failedPrecondition(errReauthRequired)
	}
	if connection.AuthMethod == store.AuthMethodOAuth {
		return c.newClient(connection.OrganizationURL, oauthAuthorizer{connections: c, workspaceID: workspaceID}), connection, nil
	}
	pat, ok, err := c.store.PAT(ctx, workspaceID)
	if err != nil {
		return nil, connection, unavailable(err)
	}
	if !ok {
		return nil, connection, failedPrecondition(errReauthRequired)
	}
	return c.newClient(connection.OrganizationURL, azure.NewPATAuthorizer(pat)), connection, nil
}

// gitSecret returns the secret Git sends for a usable connection and when it
// stops being valid.
func (c *Connections) gitSecret(ctx context.Context, workspaceID string, connection store.Connection) (string, time.Time, error) {
	limit := c.now().Add(gitCredentialLifetime)
	if connection.AuthMethod != store.AuthMethodOAuth {
		pat, ok, err := c.store.PAT(ctx, workspaceID)
		if err != nil || !ok {
			return "", time.Time{}, ErrCredentialUnavailable
		}
		return pat, limit, nil
	}
	token, err := c.accessToken(ctx, workspaceID)
	if err != nil {
		return "", time.Time{}, err
	}
	refreshed, _, err := c.store.Connection(ctx, workspaceID)
	if err == nil && refreshed.TokenExpiresAt != nil && refreshed.TokenExpiresAt.Before(limit) {
		limit = *refreshed.TokenExpiresAt
	}
	return token, limit, nil
}

// RecordHealth probes the stored connection and records the result for its
// current generation. Failures to probe are recorded, not returned.
func (c *Connections) RecordHealth(ctx context.Context, workspaceID string) {
	client, connection, err := c.Client(ctx, workspaceID)
	if err != nil {
		stored, found, loadErr := c.store.Connection(ctx, workspaceID)
		if loadErr != nil || !found {
			return
		}
		_ = c.store.SaveHealth(ctx, workspaceID, store.Health{Generation: stored.Generation, CheckedAt: c.now().UTC(), Error: probeError(err)})
		return
	}
	health := store.Health{Generation: connection.Generation, CheckedAt: c.now().UTC()}
	result, err := client.TestAuth(ctx)
	switch {
	case ctx.Err() != nil:
		return
	case err != nil:
		health.Error = probeError(err)
	case result == nil || !result.OK:
		health.Error = "Azure DevOps did not return an authenticated identity"
	default:
		health.OK = true
	}
	if latest, found, err := c.store.Connection(ctx, workspaceID); err == nil && found && latest.Generation == health.Generation {
		_ = c.store.SaveHealth(ctx, workspaceID, health)
	}
}

// probeError returns a credential-free, bounded description of a probe error.
func probeError(err error) string {
	var apiErr *azure.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401:
			return "Azure DevOps rejected the credentials (401)"
		case 403:
			return "The credentials lack access to this organization (403)"
		case 404:
			return "The Azure DevOps organization was not found (404)"
		default:
			return fmt.Sprintf("Azure DevOps returned HTTP %d", apiErr.StatusCode)
		}
	}
	if errors.Is(err, azure.ErrReauthRequired) {
		return "Microsoft sign-in expired; sign in again"
	}
	var entraErr *auth.EntraError
	if errors.As(err, &entraErr) {
		return "Microsoft sign-in failed: " + entraErr.Code
	}
	var actionErr *pluginsdk.ActionError
	if errors.As(err, &actionErr) && actionErr.Err != nil {
		return actionErr.Err.Error()
	}
	return "Azure DevOps could not be reached"
}
