// Package auth implements Microsoft Entra sign-in for Azure DevOps.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"kandev-plugin-azure-devops/internal/azure"
)

const (
	// azureDevOpsResourceScope is the Azure DevOps Services resource in
	// Microsoft Entra ID; `.default` requests its delegated permissions.
	azureDevOpsResourceScope = "499b84ac-1321-427f-aa17-267ca6975798/.default"
	Scopes                   = azureDevOpsResourceScope + " offline_access openid profile"
	DefaultTenant            = "organizations"
	entraLoginBaseURL        = "https://login.microsoftonline.com"
	devAzureBaseURL          = "https://dev.azure.com"
	entraRequestTimeout      = 15 * time.Second
	maxEntraResponseBytes    = 1 << 20
	resourceTenantHeader     = "X-VSS-ResourceTenant"
	zeroTenantID             = "00000000-0000-0000-0000-000000000000"
)

// IsGUID reports whether value is a GUID, the shape of tenant and
// application (client) IDs.
func IsGUID(value string) bool { return tenantIDRE.MatchString(value) }

var tenantIDRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Token is the token-endpoint result. Values are secrets.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type AuthorizeRequest struct {
	Tenant        string
	ClientID      string
	RedirectURL   string
	State         string
	CodeChallenge string
}

// IdentityProvider is the Microsoft identity platform surface used for
// Azure DevOps sign-in. Tests and the E2E mock replace the HTTP client.
type IdentityProvider interface {
	DiscoverTenant(ctx context.Context, organizationURL string) (string, error)
	AuthorizeURL(req AuthorizeRequest) string
	ExchangeCode(ctx context.Context, tenant, clientID, code, redirectURL, verifier string) (*Token, error)
	Refresh(ctx context.Context, tenant, clientID, refreshToken string) (*Token, error)
}

// EntraError is a token-endpoint failure. Only the OAuth error code is kept so
// provider descriptions never reach logs or API responses.
type EntraError struct {
	Code   string
	Status int
}

func (e *EntraError) Error() string { return "microsoft sign-in failed: " + e.Code }

// RequiresSignIn reports whether only a new interactive sign-in can recover.
func (e *EntraError) RequiresSignIn() bool {
	switch e.Code {
	case "invalid_grant", "interaction_required", "consent_required", "login_required":
		return true
	}
	return false
}

type EntraClient struct {
	loginBaseURL    string
	devAzureBaseURL string
	httpClient      *http.Client
}

func NewEntraClient() *EntraClient {
	return &EntraClient{
		loginBaseURL:    entraLoginBaseURL,
		devAzureBaseURL: devAzureBaseURL,
		httpClient: &http.Client{
			Timeout: entraRequestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// DiscoverTenant returns the Entra tenant that backs an organization, read
// from Azure DevOps' unauthenticated challenge. Guests need tokens from this
// resource tenant rather than from their home tenant.
func (c *EntraClient) DiscoverTenant(ctx context.Context, organizationURL string) (string, error) {
	canonical, err := azure.ValidateOrganizationURL(organizationURL)
	if err != nil {
		return "", err
	}
	organization := strings.TrimPrefix(canonical, devAzureBaseURL+"/")
	endpoint := c.devAzureBaseURL + "/" + url.PathEscape(organization) + "/_apis/connectionData"
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create tenant discovery request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("discover azure devops tenant: %w", err)
	}
	_ = resp.Body.Close()
	for _, value := range resp.Header.Values(resourceTenantHeader) {
		for _, candidate := range strings.Split(value, ",") {
			candidate = strings.TrimSpace(candidate)
			if tenantIDRE.MatchString(candidate) && candidate != zeroTenantID {
				return strings.ToLower(candidate), nil
			}
		}
	}
	return DefaultTenant, nil
}

func (c *EntraClient) AuthorizeURL(req AuthorizeRequest) string {
	params := url.Values{
		"client_id":             {req.ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {req.RedirectURL},
		"response_mode":         {"query"},
		"scope":                 {Scopes},
		"state":                 {req.State},
		"code_challenge":        {req.CodeChallenge},
		"code_challenge_method": {"S256"},
		"prompt":                {"select_account"},
	}
	return entraLoginBaseURL + "/" + url.PathEscape(req.Tenant) + "/oauth2/v2.0/authorize?" + params.Encode()
}

func (c *EntraClient) ExchangeCode(
	ctx context.Context, tenant, clientID, code, redirectURL, verifier string,
) (*Token, error) {
	return c.postToken(ctx, tenant, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURL},
		"code_verifier": {verifier},
		"scope":         {Scopes},
	})
}

func (c *EntraClient) Refresh(ctx context.Context, tenant, clientID, refreshToken string) (*Token, error) {
	return c.postToken(ctx, tenant, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
		"scope":         {Scopes},
	})
}

func (c *EntraClient) postToken(ctx context.Context, tenant string, form url.Values) (*Token, error) {
	endpoint := c.loginBaseURL + "/" + url.PathEscape(tenant) + "/oauth2/v2.0/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("microsoft token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxEntraResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read microsoft token response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &failure)
		if failure.Error == "" {
			failure.Error = "http_" + http.StatusText(resp.StatusCode)
		}
		return nil, &EntraError{Code: failure.Error, Status: resp.StatusCode}
	}
	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, errors.New("decode microsoft token response")
	}
	if token.AccessToken == "" {
		return nil, errors.New("microsoft token response has no access token")
	}
	return &token, nil
}
