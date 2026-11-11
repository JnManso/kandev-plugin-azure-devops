package plugin

import (
	"context"
	"fmt"
	"sync"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/auth"
)

const (
	goodCode      = "good-code"
	testClientID  = "11111111-2222-3333-4444-555555555555"
	otherClientID = "99999999-8888-7777-6666-555555555555"
	testRedirect  = "http://localhost:38429"
)

// fakeEntra issues tokens the fake Azure DevOps accepts.
type fakeEntra struct {
	mu            sync.Mutex
	azure         *fakeAzure
	issued        int
	expiresIn     int64
	refreshCalls  []string // client IDs
	refreshTokens []string // refresh tokens redeemed
	refreshErr    error
	refreshGate   chan struct{}
	lastExchange  struct{ clientID, redirect, verifier, tenant string }
}

func newFakeEntra(azure *fakeAzure) *fakeEntra {
	return &fakeEntra{azure: azure, expiresIn: 3600}
}

func (f *fakeEntra) DiscoverTenant(context.Context, string) (string, error) { return "tenant-1", nil }

func (f *fakeEntra) AuthorizeURL(request auth.AuthorizeRequest) string {
	return "https://login.example/" + request.Tenant + "/authorize?client_id=" + request.ClientID + "&state=" + request.State
}

func (f *fakeEntra) ExchangeCode(_ context.Context, tenant, clientID, code, redirectURL, verifier string) (*auth.Token, error) {
	f.mu.Lock()
	f.lastExchange = struct{ clientID, redirect, verifier, tenant string }{clientID, redirectURL, verifier, tenant}
	f.mu.Unlock()
	if code != goodCode {
		return nil, &auth.EntraError{Code: "invalid_grant", Status: 400}
	}
	return f.issue(), nil
}

func (f *fakeEntra) Refresh(_ context.Context, _, clientID, refreshToken string) (*auth.Token, error) {
	f.mu.Lock()
	f.refreshCalls = append(f.refreshCalls, clientID)
	f.refreshTokens = append(f.refreshTokens, refreshToken)
	gate, err := f.refreshGate, f.refreshErr
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if err != nil {
		return nil, err
	}
	return f.issue(), nil
}

func (f *fakeEntra) issue() *auth.Token {
	f.mu.Lock()
	f.issued++
	n, expiresIn := f.issued, f.expiresIn
	f.mu.Unlock()
	access := fmt.Sprintf("access-%d", n)
	f.azure.mu.Lock()
	f.azure.validBearers[access] = true
	f.azure.mu.Unlock()
	return &auth.Token{AccessToken: access, RefreshToken: fmt.Sprintf("refresh-%d", n), ExpiresIn: expiresIn}
}

func (f *fakeEntra) refreshCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.refreshCalls)
}

// actionAs invokes an action as one actor.
func (h *harness) actionAs(workspaceID, actorID, key string, body any) (int, map[string]any) {
	h.t.Helper()
	return h.actionWith(pluginsdk.VerifiedActionContext{WorkspaceID: workspaceID, ActorID: actorID}, key, body)
}

// signIn runs a complete Microsoft sign-in for workspaceID as user-1.
func (h *harness) signIn(workspaceID string) map[string]any {
	h.t.Helper()
	status, start := h.actionAs(workspaceID, "user-1", "oauth.start", map[string]any{
		"organizationUrl": "https://dev.azure.com/acme", "clientId": testClientID, "redirectUrl": testRedirect,
		"defaultProjectName": "Platform Team",
	})
	if status != 200 {
		h.t.Fatalf("oauth.start status = %d body = %v", status, start)
	}
	status, body := h.actionAs(workspaceID, "user-1", "oauth.complete", map[string]any{"code": goodCode, "state": start["state"]})
	if status != 200 {
		h.t.Fatalf("oauth.complete status = %d body = %v", status, body)
	}
	return body
}
