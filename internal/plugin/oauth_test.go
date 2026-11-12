package plugin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/auth"
	"kandev-plugin-azure-devops/internal/store"
)

func TestAvailabilityReportsOnlyAGUIDDefaultApplication(t *testing.T) {
	h := newHarness(t)
	if _, body := h.action("ws-1", "oauth.availability", nil); body["defaultClientId"] != nil {
		t.Fatalf("availability without config = %v", body)
	}
	h.host.Config = map[string]any{"default_oauth_client_id": "not-a-guid"}
	if _, body := h.action("ws-1", "oauth.availability", nil); body["defaultClientId"] != nil {
		t.Fatalf("availability with an invalid client ID = %v", body)
	}
	h.host.Config = map[string]any{"default_oauth_client_id": strings.ToUpper(testClientID)}
	if _, body := h.action("ws-1", "oauth.availability", nil); body["defaultClientId"] != testClientID {
		t.Fatalf("availability = %v", body)
	}
}

func TestStartRequiresAnApplicationAndAValidRedirect(t *testing.T) {
	h := newHarness(t)
	base := map[string]any{"organizationUrl": "https://dev.azure.com/acme", "redirectUrl": testRedirect}
	if status, body := h.actionAs("ws-1", "user-1", "oauth.start", base); status != 400 || !strings.Contains(body["error"].(string), "client") {
		t.Fatalf("start without an application status = %d body = %v", status, body)
	}
	for _, redirect := range []string{"http://kandev.example", "https://kandev.example/?x=1", "relative", ""} {
		input := map[string]any{"organizationUrl": "https://dev.azure.com/acme", "clientId": testClientID, "redirectUrl": redirect}
		if status, _ := h.actionAs("ws-1", "user-1", "oauth.start", input); status != 400 {
			t.Fatalf("redirect %q status = %d, want 400", redirect, status)
		}
	}
	h.host.Config = map[string]any{"default_oauth_client_id": testClientID}
	status, body := h.actionAs("ws-1", "user-1", "oauth.start", base)
	if status != 200 || !strings.Contains(body["authUrl"].(string), "client_id="+testClientID) || body["state"] == "" {
		t.Fatalf("start with the default application status = %d body = %v", status, body)
	}
	keys := h.host.StateKeys("workspace", "ws-1")
	if len(keys) != 1 || keys[0] != "oauth.flow."+hashState(body["state"].(string)) {
		t.Fatalf("flow keys = %v", keys)
	}
}

func TestCompleteCreatesAMicrosoftAccountConnectionAndRemovesThePAT(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	body := h.signIn("ws-1")
	connection := field(body, "connection")
	if field(connection, "authMethod") != "oauth" || field(connection, "oauthAccount") != "Ada" ||
		field(connection, "oauthClientId") != testClientID || field(connection, "hasSecret") != true ||
		field(connection, "lastOk") != true || field(connection, "defaultProjectName") != "Platform Team" {
		t.Fatalf("connection = %v", connection)
	}
	if _, found := h.host.Secret(store.PATKey("ws-1")); found {
		t.Fatal("the replaced PAT was kept")
	}
	if refresh, _ := h.host.Secret(store.OAuthRefreshKey("ws-1")); refresh != "refresh-1" {
		t.Fatalf("refresh token = %q", refresh)
	}
	if h.entra.lastExchange.redirect != testRedirect || h.entra.lastExchange.tenant != "tenant-1" || h.entra.lastExchange.verifier == "" {
		t.Fatalf("exchange = %+v", h.entra.lastExchange)
	}
	stored, _, _ := store.New(h.host).Connection(t.Context(), "ws-1")
	if stored.Generation != 2 {
		t.Fatalf("generation = %d, want 2", stored.Generation)
	}
	if strings.Contains(flatten(body), "access-1") || strings.Contains(flatten(body), "refresh-1") {
		t.Fatalf("response leaks a token: %v", body)
	}
}

func TestCompleteRejectsForeignReusedAndExpiredAttemptsWithoutWriting(t *testing.T) {
	h := newHarness(t)
	start := func() string {
		_, body := h.actionAs("ws-1", "user-1", "oauth.start", map[string]any{
			"organizationUrl": "https://dev.azure.com/acme", "clientId": testClientID, "redirectUrl": testRedirect,
		})
		return body["state"].(string)
	}
	cases := map[string]func() (string, string, string){
		"unknown state":     func() (string, string, string) { return "ws-1", "user-1", "unknown" },
		"another workspace": func() (string, string, string) { return "ws-2", "user-1", start() },
		"another actor":     func() (string, string, string) { return "ws-1", "user-2", start() },
	}
	for name, attempt := range cases {
		workspaceID, actorID, state := attempt()
		if status, _ := h.actionAs(workspaceID, actorID, "oauth.complete", map[string]any{"code": goodCode, "state": state}); status != 400 {
			t.Fatalf("%s: status = %d, want 400", name, status)
		}
	}
	state := start()
	h.signInWithState("ws-1", state)
	if status, _ := h.actionAs("ws-1", "user-1", "oauth.complete", map[string]any{"code": goodCode, "state": state}); status != 400 {
		t.Fatalf("reused state status = %d, want 400", status)
	}
	services, _ := h.runtime.ready()
	expired := start()
	services.connections.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	if status, _ := h.actionAs("ws-1", "user-1", "oauth.complete", map[string]any{"code": goodCode, "state": expired}); status != 400 {
		t.Fatalf("expired state status = %d, want 400", status)
	}
	if _, found := h.host.Secret(store.OAuthRefreshKey("ws-2")); found {
		t.Fatal("a rejected attempt wrote tokens")
	}
}

func (h *harness) signInWithState(workspaceID, state string) {
	h.t.Helper()
	if status, body := h.actionAs(workspaceID, "user-1", "oauth.complete", map[string]any{"code": goodCode, "state": state}); status != 200 {
		h.t.Fatalf("complete status = %d body = %v", status, body)
	}
}

func TestAFailedExchangeKeepsTheExistingPATConnection(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	_, start := h.actionAs("ws-1", "user-1", "oauth.start", map[string]any{
		"organizationUrl": "https://dev.azure.com/acme", "clientId": testClientID, "redirectUrl": testRedirect,
	})
	if status, _ := h.actionAs("ws-1", "user-1", "oauth.complete", map[string]any{"code": "bad", "state": start["state"]}); status != 400 {
		t.Fatalf("status = %d, want 400", status)
	}
	_, body := h.action("ws-1", "connection.get", nil)
	if field(body, "connection", "authMethod") != "pat" {
		t.Fatalf("connection = %v", body)
	}
	if pat, _ := h.host.Secret(store.PATKey("ws-1")); pat != "good-pat" {
		t.Fatalf("PAT = %q", pat)
	}
}

func setTokenExpiry(t *testing.T, h *harness, workspaceID string, expiresAt time.Time) {
	t.Helper()
	s := store.New(h.host)
	connection, _, _ := s.Connection(t.Context(), workspaceID)
	connection.TokenExpiresAt = &expiresAt
	if err := s.SaveConnection(t.Context(), workspaceID, connection); err != nil {
		t.Fatal(err)
	}
}

func TestAccessTokenRenewsNearExpiryWithTheSigningClientAndPersistsRotation(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	h.host.Config = map[string]any{"default_oauth_client_id": otherClientID}
	if _, body := h.action("ws-1", "projects.list", nil); len(body["projects"].([]any)) != 2 {
		t.Fatalf("projects = %v", body)
	}
	if h.entra.refreshCount() != 0 {
		t.Fatal("a fresh token was renewed")
	}
	setTokenExpiry(t, h, "ws-1", time.Now().Add(time.Minute))
	if _, body := h.action("ws-1", "projects.list", nil); len(body["projects"].([]any)) != 2 {
		t.Fatalf("projects after renewal = %v", body)
	}
	if h.entra.refreshCount() != 1 || h.entra.refreshCalls[0] != testClientID || h.entra.refreshTokens[0] != "refresh-1" {
		t.Fatalf("refresh calls = %v tokens = %v", h.entra.refreshCalls, h.entra.refreshTokens)
	}
	if refresh, _ := h.host.Secret(store.OAuthRefreshKey("ws-1")); refresh != "refresh-2" {
		t.Fatalf("rotated refresh token = %q", refresh)
	}
}

func TestARejectedBearerIsRenewedOnceAndTheRequestRetried(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	h.azure.mu.Lock()
	delete(h.azure.validBearers, "access-1")
	h.azure.mu.Unlock()
	if status, body := h.action("ws-1", "projects.list", nil); status != 200 {
		t.Fatalf("status = %d body = %v", status, body)
	}
	if h.entra.refreshCount() != 1 {
		t.Fatalf("refresh calls = %d, want 1", h.entra.refreshCount())
	}
}

func TestConcurrentRenewalIssuesOneTokenRequest(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	setTokenExpiry(t, h, "ws-1", time.Now().Add(time.Minute))
	services, _ := h.runtime.ready()
	gate := make(chan struct{})
	h.entra.mu.Lock()
	h.entra.refreshGate = gate
	h.entra.mu.Unlock()
	results := make(chan string, 4)
	for range 4 {
		go func() {
			token, err := currentAccessToken(services.connections, "ws-1")
			if err != nil {
				token = "error: " + err.Error()
			}
			results <- token
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(gate)
	for range 4 {
		if token := <-results; token != "access-2" {
			t.Fatalf("token = %q, want access-2", token)
		}
	}
	if h.entra.refreshCount() != 1 {
		t.Fatalf("refresh calls = %d, want 1", h.entra.refreshCount())
	}
}

func TestRenewSkipsWhenTheRejectedTokenWasAlreadyRotated(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	services, _ := h.runtime.ready()
	renewed, err := services.connections.renewAccessToken(t.Context(), "ws-1", mustConnection(t, h, "ws-1"), "an-older-token")
	if err != nil || !renewed || h.entra.refreshCount() != 0 {
		t.Fatalf("renewed = %v err = %v refresh calls = %d", renewed, err, h.entra.refreshCount())
	}
}

func TestAnInvalidGrantRequiresSignInAndStopsCallingMicrosoft(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	h.entra.refreshErr = &auth.EntraError{Code: "invalid_grant", Status: 400}
	setTokenExpiry(t, h, "ws-1", time.Now().Add(time.Minute))
	if status, _ := h.action("ws-1", "projects.list", nil); status == 200 {
		t.Fatal("projects.list succeeded with an expired sign-in")
	}
	_, body := h.action("ws-1", "connection.get", nil)
	if field(body, "connection", "reauthRequired") != true {
		t.Fatalf("connection = %v", body)
	}
	calls := h.entra.refreshCount()
	h.action("ws-1", "projects.list", nil)
	if h.entra.refreshCount() != calls {
		t.Fatal("Microsoft was called again after sign-in became required")
	}
}

func TestATransientRenewalFailureDoesNotRequireSignIn(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	h.entra.refreshErr = &auth.EntraError{Code: "temporarily_unavailable", Status: 503}
	setTokenExpiry(t, h, "ws-1", time.Now().Add(time.Minute))
	h.action("ws-1", "projects.list", nil)
	if _, body := h.action("ws-1", "connection.get", nil); field(body, "connection", "reauthRequired") != false {
		t.Fatalf("connection = %v", body)
	}
}

func TestARenewalInFlightCannotOverwriteANewSignIn(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	setTokenExpiry(t, h, "ws-1", time.Now().Add(time.Minute))
	services, _ := h.runtime.ready()
	gate := make(chan struct{})
	h.entra.mu.Lock()
	h.entra.refreshGate = gate
	h.entra.mu.Unlock()
	renewal := make(chan error, 1)
	go func() {
		_, err := currentAccessToken(services.connections, "ws-1")
		renewal <- err
	}()
	time.Sleep(50 * time.Millisecond)
	_, start := h.actionAs("ws-1", "user-1", "oauth.start", map[string]any{
		"organizationUrl": "https://dev.azure.com/acme", "clientId": otherClientID, "redirectUrl": testRedirect,
	})
	h.entra.mu.Lock()
	h.entra.refreshGate = nil
	h.entra.mu.Unlock()
	signedIn := make(chan struct{})
	go func() {
		h.actionAs("ws-1", "user-1", "oauth.complete", map[string]any{"code": goodCode, "state": start["state"]})
		close(signedIn)
	}()
	time.Sleep(50 * time.Millisecond)
	close(gate)
	<-signedIn
	if err := <-renewal; err != nil {
		t.Fatalf("renewal error = %v", err)
	}
	connection, _, _ := store.New(h.host).Connection(t.Context(), "ws-1")
	if connection.OAuthClientID != otherClientID {
		t.Fatalf("client ID = %q, want the new sign-in's", connection.OAuthClientID)
	}
	// The sign-in exchanged its code (access-2) before the blocked renewal
	// issued access-3; holding the workspace lock, it writes last and wins.
	access, _ := h.host.Secret(store.OAuthAccessKey("ws-1"))
	if access != "access-2" {
		t.Fatalf("access token = %q, want the new sign-in's access-2", access)
	}
}

func TestSignOutRemovesTokensAndKeepsSettings(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	status, body := h.action("ws-1", "oauth.sign_out", nil)
	if status != 200 || field(body, "connection", "reauthRequired") != true || field(body, "connection", "organizationUrl") != "https://dev.azure.com/acme" ||
		field(body, "connection", "oauthAccount") != nil {
		t.Fatalf("status = %d body = %v", status, body)
	}
	for _, key := range []string{store.OAuthRefreshKey("ws-1"), store.OAuthAccessKey("ws-1")} {
		if _, found := h.host.Secret(key); found {
			t.Fatalf("%s survived sign-out", key)
		}
	}
	h.connect("ws-2")
	if status, _ := h.action("ws-2", "oauth.sign_out", nil); status != 400 {
		t.Fatalf("sign-out of a PAT connection status = %d, want 400", status)
	}
}

func TestMicrosoftConnectionSettingsTransitions(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	status, body := h.action("ws-1", "connection.save", map[string]any{
		"organizationUrl": "https://dev.azure.com/acme", "authMethod": "oauth", "defaultProjectName": "Ops",
	})
	if status != 200 || field(body, "connection", "defaultProjectName") != "Ops" || field(body, "connection", "hasSecret") != true {
		t.Fatalf("default project save status = %d body = %v", status, body)
	}
	if status, _ := h.action("ws-1", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/other", "authMethod": "oauth"}); status != 400 {
		t.Fatalf("organization change status = %d, want 400", status)
	}
	if status, _ := h.action("ws-1", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/acme"}); status != 400 {
		t.Fatalf("switch to PAT without a PAT status = %d, want 400", status)
	}
	status, body = h.action("ws-1", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/acme", "pat": "good-pat"})
	if status != 200 || field(body, "connection", "authMethod") != "pat" {
		t.Fatalf("switch to PAT status = %d body = %v", status, body)
	}
	if _, found := h.host.Secret(store.OAuthRefreshKey("ws-1")); found {
		t.Fatal("Microsoft tokens survived the switch to a PAT")
	}
}

func TestCopyOfAMicrosoftConnectionRequiresSignInInTheTarget(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-source")
	status, body := h.action("ws-target", "connection.copy", map[string]any{"sourceWorkspaceId": "ws-source"})
	if status != 200 || field(body, "connection", "authMethod") != "oauth" || field(body, "connection", "reauthRequired") != true ||
		field(body, "connection", "oauthClientId") != testClientID {
		t.Fatalf("status = %d body = %v", status, body)
	}
	if _, found := h.host.Secret(store.OAuthRefreshKey("ws-target")); found {
		t.Fatal("copy shared the refresh token")
	}
}

func TestDeleteRemovesMicrosoftTokensAndPendingSignIns(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	h.actionAs("ws-1", "user-1", "oauth.start", map[string]any{
		"organizationUrl": "https://dev.azure.com/acme", "clientId": testClientID, "redirectUrl": testRedirect,
	})
	h.action("ws-1", "connection.delete", nil)
	// Only the deleted connection's generation stays, for the next connection.
	if keys := h.host.StateKeys("workspace", "ws-1"); len(keys) != 1 || keys[0] != "connectionGeneration" {
		t.Fatalf("state after delete = %v", keys)
	}
	if _, found := h.host.Secret(store.OAuthRefreshKey("ws-1")); found {
		t.Fatal("refresh token survived delete")
	}
}

func TestGitCredentialUsesTheMicrosoftAccessTokenUntilSignOut(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	h.host.AddRepository(pluginsdk.Repository{ID: "repo-1", WorkspaceID: "ws-1", ProviderID: "azure_devops", RemoteURL: "https://dev.azure.com/acme/Platform%20Team/_git/api"})
	response, err := h.runtime.ResolveGitCredential(context.Background(), credentialRequest())
	if err != nil || response.Username != "kandev" || response.Secret != "access-1" {
		t.Fatalf("credential = %+v err = %v", response, err)
	}
	expiresAt, _ := time.Parse(time.RFC3339, response.ExpiresAt)
	if expiresAt.After(time.Now().Add(gitCredentialLifetime + time.Second)) {
		t.Fatalf("credential expires at %v, beyond the lease limit", expiresAt)
	}
	h.action("ws-1", "oauth.sign_out", nil)
	if _, err := h.runtime.ResolveGitCredential(context.Background(), credentialRequest()); err == nil {
		t.Fatal("a credential was issued after sign-out")
	}
}

// currentAccessToken reads a token bound to the connection stored now, like
// a client built at that moment.
func currentAccessToken(connections *Connections, workspaceID string) (string, error) {
	connection, _, err := connections.store.Connection(context.Background(), workspaceID)
	if err != nil {
		return "", err
	}
	token, _, err := connections.boundAccessToken(context.Background(), workspaceID, connection)
	return token, err
}

func mustConnection(t *testing.T, h *harness, workspaceID string) store.Connection {
	t.Helper()
	connection, found, err := store.New(h.host).Connection(t.Context(), workspaceID)
	if err != nil || !found {
		t.Fatalf("connection = %v, %v", found, err)
	}
	return connection
}
