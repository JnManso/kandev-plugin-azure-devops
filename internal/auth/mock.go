package auth

import (
	"context"
	"fmt"
	"net/url"
	"sync"
)

// MockCode is the only authorization code MockEntra accepts.
const MockCode = "mock-authorization-code"

// MockEntra stands in for Microsoft in mock mode. Its authorization URL is
// the redirect URL itself with a code, so the sign-in popup returns at once.
type MockEntra struct {
	mu     sync.Mutex
	issued int
}

func (m *MockEntra) DiscoverTenant(context.Context, string) (string, error) {
	return "mock-tenant", nil
}

func (m *MockEntra) AuthorizeURL(request AuthorizeRequest) string {
	return request.RedirectURL + "/?" + url.Values{"code": {MockCode}, "state": {request.State}}.Encode()
}

func (m *MockEntra) ExchangeCode(_ context.Context, _, _, code, _, _ string) (*Token, error) {
	if code != MockCode {
		return nil, &EntraError{Code: "invalid_grant", Status: 400}
	}
	return m.issue(), nil
}

func (m *MockEntra) Refresh(context.Context, string, string, string) (*Token, error) {
	return m.issue(), nil
}

func (m *MockEntra) issue() *Token {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.issued++
	return &Token{
		AccessToken:  fmt.Sprintf("mock-access-%d", m.issued),
		RefreshToken: fmt.Sprintf("mock-refresh-%d", m.issued),
		ExpiresIn:    3600,
	}
}

var _ IdentityProvider = (*MockEntra)(nil)
