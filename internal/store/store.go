// Package store keeps the plugin's per-workspace Azure DevOps connection in
// Kandev Host state and its credentials in Host secrets.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	scopeWorkspace = "workspace"
	connectionKey  = "connection"
	healthKey      = "health"
	// generationKey keeps the last generation of a deleted connection.
	generationKey = "connectionGeneration"

	// AuthMethodPAT authenticates with a Personal Access Token.
	AuthMethodPAT = "pat"
	// AuthMethodOAuth authenticates with a Microsoft account (Entra ID).
	AuthMethodOAuth = "oauth"
)

// Connection is the non-secret workspace connection. Generation increments
// whenever the credential or organization changes, so outstanding Git
// credential leases bound to an older generation are revoked.
type Connection struct {
	OrganizationURL    string    `json:"organizationUrl"`
	DefaultProjectID   string    `json:"defaultProjectId,omitempty"`
	DefaultProjectName string    `json:"defaultProjectName,omitempty"`
	AuthMethod         string    `json:"authMethod"`
	ReauthRequired     bool      `json:"reauthRequired,omitempty"`
	Generation         int64     `json:"generation"`
	UpdatedAt          time.Time `json:"updatedAt"`

	// Microsoft account connections only. OAuthClientID is the application
	// that signed in and must renew; OAuthTenantID is the organization's
	// resource tenant.
	OAuthClientID  string     `json:"oauthClientId,omitempty"`
	OAuthTenantID  string     `json:"oauthTenantId,omitempty"`
	OAuthAccount   string     `json:"oauthAccount,omitempty"`
	TokenExpiresAt *time.Time `json:"tokenExpiresAt,omitempty"`
}

// Health is the latest authenticated probe for one connection generation.
type Health struct {
	Generation int64     `json:"generation"`
	CheckedAt  time.Time `json:"checkedAt"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error,omitempty"`
}

// Host is the subset of pluginsdk.Host the store uses.
type Host interface {
	GetState(ctx context.Context, scope, scopeID, key string) (map[string]any, bool, error)
	SetState(ctx context.Context, scope, scopeID, key string, value map[string]any) error
	DeleteState(ctx context.Context, scope, scopeID, key string) error
	GetSecret(ctx context.Context, key string) (string, bool, error)
	SetSecret(ctx context.Context, key, value string) error
	DeleteSecret(ctx context.Context, key string) error
}

// Store reads and writes connection records.
type Store struct{ host Host }

// New returns a Store backed by host.
func New(host Host) *Store { return &Store{host: host} }

// PATKey is the workspace-isolated secret key of the Personal Access Token.
func PATKey(workspaceID string) string { return "ws." + workspaceID + ".pat" }

// OAuthRefreshKey is the secret key of the Microsoft refresh token.
func OAuthRefreshKey(workspaceID string) string { return "ws." + workspaceID + ".oauth.refresh" }

// OAuthAccessKey is the secret key of the Microsoft access token.
func OAuthAccessKey(workspaceID string) string { return "ws." + workspaceID + ".oauth.access" }

// Secret reads a secret; found is false for a missing or empty value.
func (s *Store) Secret(ctx context.Context, key string) (string, bool, error) {
	value, found, err := s.host.GetSecret(ctx, key)
	if err != nil {
		return "", false, fmt.Errorf("read secret: %w", err)
	}
	return value, found && value != "", nil
}

// SetSecret writes a secret.
func (s *Store) SetSecret(ctx context.Context, key, value string) error {
	if err := s.host.SetSecret(ctx, key, value); err != nil {
		return fmt.Errorf("store secret: %w", err)
	}
	return nil
}

// DeleteSecret removes a secret; a missing secret is not an error.
func (s *Store) DeleteSecret(ctx context.Context, key string) error {
	if err := s.host.DeleteSecret(ctx, key); err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}

// Connection loads the workspace connection.
func (s *Store) Connection(ctx context.Context, workspaceID string) (Connection, bool, error) {
	var connection Connection
	found, err := s.load(ctx, workspaceID, connectionKey, &connection)
	return connection, found, err
}

// SaveConnection replaces the workspace connection. A connection saved after
// a deletion continues past the deleted connection's generation, so a Git
// credential lease bound to the deleted credential never matches its
// replacement.
func (s *Store) SaveConnection(ctx context.Context, workspaceID string, connection Connection) error {
	var deleted deletedGeneration
	if _, err := s.load(ctx, workspaceID, generationKey, &deleted); err != nil {
		return err
	}
	if connection.Generation <= deleted.Generation {
		connection.Generation = deleted.Generation + 1
	}
	return s.save(ctx, workspaceID, connectionKey, connection)
}

// deletedGeneration is the generation of the last deleted connection.
type deletedGeneration struct {
	Generation int64 `json:"generation"`
}

// DeleteConnection removes the connection and its health record, keeping its
// generation for the next connection.
func (s *Store) DeleteConnection(ctx context.Context, workspaceID string) error {
	connection, found, err := s.Connection(ctx, workspaceID)
	if err != nil {
		return err
	}
	var deleted deletedGeneration
	if _, err := s.load(ctx, workspaceID, generationKey, &deleted); err != nil {
		return err
	}
	if found && connection.Generation > deleted.Generation {
		if err := s.save(ctx, workspaceID, generationKey, deletedGeneration{Generation: connection.Generation}); err != nil {
			return err
		}
	}
	if err := s.host.DeleteState(ctx, scopeWorkspace, workspaceID, connectionKey); err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	if err := s.host.DeleteState(ctx, scopeWorkspace, workspaceID, healthKey); err != nil {
		return fmt.Errorf("delete connection health: %w", err)
	}
	return nil
}

// Health loads the latest probe result.
func (s *Store) Health(ctx context.Context, workspaceID string) (Health, bool, error) {
	var health Health
	found, err := s.load(ctx, workspaceID, healthKey, &health)
	return health, found, err
}

// SaveHealth records a probe result.
func (s *Store) SaveHealth(ctx context.Context, workspaceID string, health Health) error {
	return s.save(ctx, workspaceID, healthKey, health)
}

// PAT reads the workspace Personal Access Token.
func (s *Store) PAT(ctx context.Context, workspaceID string) (string, bool, error) {
	value, found, err := s.host.GetSecret(ctx, PATKey(workspaceID))
	if err != nil {
		return "", false, fmt.Errorf("read personal access token: %w", err)
	}
	return value, found && value != "", nil
}

// SetPAT stores the workspace Personal Access Token.
func (s *Store) SetPAT(ctx context.Context, workspaceID, pat string) error {
	if err := s.host.SetSecret(ctx, PATKey(workspaceID), pat); err != nil {
		return fmt.Errorf("store personal access token: %w", err)
	}
	return nil
}

// DeletePAT removes the workspace Personal Access Token.
func (s *Store) DeletePAT(ctx context.Context, workspaceID string) error {
	if err := s.host.DeleteSecret(ctx, PATKey(workspaceID)); err != nil {
		return fmt.Errorf("delete personal access token: %w", err)
	}
	return nil
}

func (s *Store) load(ctx context.Context, workspaceID, key string, target any) (bool, error) {
	value, found, err := s.host.GetState(ctx, scopeWorkspace, workspaceID, key)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", key, err)
	}
	if !found {
		return false, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("encode %s: %w", key, err)
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		return false, fmt.Errorf("decode %s: %w", key, err)
	}
	return true, nil
}

func (s *Store) save(ctx context.Context, workspaceID, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", key, err)
	}
	var record map[string]any
	if err := json.Unmarshal(encoded, &record); err != nil {
		return fmt.Errorf("encode %s: %w", key, err)
	}
	if err := s.host.SetState(ctx, scopeWorkspace, workspaceID, key, record); err != nil {
		return fmt.Errorf("write %s: %w", key, err)
	}
	return nil
}

// LoadState decodes a workspace state entry into target.
func (s *Store) LoadState(ctx context.Context, workspaceID, key string, target any) (bool, error) {
	return s.load(ctx, workspaceID, key, target)
}

// SaveState encodes value into a workspace state entry.
func (s *Store) SaveState(ctx context.Context, workspaceID, key string, value any) error {
	return s.save(ctx, workspaceID, key, value)
}

// DeleteState removes a workspace state entry. Deleting a missing key is not an error.
func (s *Store) DeleteState(ctx context.Context, workspaceID, key string) error {
	if err := s.host.DeleteState(ctx, scopeWorkspace, workspaceID, key); err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	return nil
}
