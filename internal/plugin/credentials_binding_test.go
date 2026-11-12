package plugin

import (
	"context"
	"errors"
	"testing"

	"kandev-plugin-azure-devops/internal/store"
)

func TestAGitSecretIsNeverReadForAConnectionReplacedAfterVerification(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	services, _ := h.runtime.ready()
	verified := mustConnection(t, h, "ws-1")
	if status, body := h.action("ws-1", "connection.save", map[string]any{
		"organizationUrl": "https://dev.azure.com/fabrikam", "pat": "fabrikam-pat",
	}); status != 200 {
		t.Fatalf("replace = %d %v", status, body)
	}
	if secret, _, err := services.connections.gitSecret(context.Background(), "ws-1", verified); err == nil {
		t.Fatalf("gitSecret returned %q for the replaced connection", secret)
	}
	current := mustConnection(t, h, "ws-1")
	if secret, _, err := services.connections.gitSecret(context.Background(), "ws-1", current); err != nil || secret != "fabrikam-pat" {
		t.Fatalf("gitSecret for the current connection = %q, %v", secret, err)
	}
}

func TestAMicrosoftClientStopsOnceItsConnectionIsReplaced(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	services, _ := h.runtime.ready()
	bound := mustConnection(t, h, "ws-1")
	if _, _, err := services.connections.boundAccessToken(context.Background(), "ws-1", bound); err != nil {
		t.Fatalf("token for the bound connection: %v", err)
	}
	h.connect("ws-1")
	if _, _, err := services.connections.boundAccessToken(context.Background(), "ws-1", bound); !errors.Is(err, errConnectionChanged) {
		t.Fatalf("token after replacement err = %v, want errConnectionChanged", err)
	}
	if _, err := services.connections.renewAccessToken(context.Background(), "ws-1", bound, "rejected"); !errors.Is(err, errConnectionChanged) {
		t.Fatalf("renewal after replacement err = %v, want errConnectionChanged", err)
	}
}

func TestARecreatedConnectionNeverReusesAGeneration(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	deleted := mustConnection(t, h, "ws-1").Generation
	if status, _ := h.action("ws-1", "connection.delete", nil); status != 200 {
		t.Fatalf("delete status = %d", status)
	}
	h.connect("ws-1")
	if recreated := mustConnection(t, h, "ws-1").Generation; recreated <= deleted {
		t.Fatalf("recreated generation = %d, want more than the deleted %d", recreated, deleted)
	}
}

func TestAMicrosoftSignInCompletedAfterSignOutIsRejected(t *testing.T) {
	h := newHarness(t)
	h.signIn("ws-1")
	_, start := h.actionAs("ws-1", "user-1", "oauth.start", map[string]any{
		"organizationUrl": "https://dev.azure.com/acme", "clientId": testClientID, "redirectUrl": testRedirect,
	})
	if status, _ := h.action("ws-1", "oauth.sign_out", nil); status != 200 {
		t.Fatalf("sign-out status = %d", status)
	}
	signedOut := mustConnection(t, h, "ws-1")
	status, body := h.actionAs("ws-1", "user-1", "oauth.complete", map[string]any{"code": goodCode, "state": start["state"]})
	if status == 200 {
		t.Fatalf("completion after sign-out = %d %v, want a rejection", status, body)
	}
	after := mustConnection(t, h, "ws-1")
	if !after.ReauthRequired || after.Generation != signedOut.Generation {
		t.Fatalf("connection after the rejected completion = %+v", after)
	}
	if _, found := h.host.Secret(store.OAuthAccessKey("ws-1")); found {
		t.Fatal("the rejected completion stored a token")
	}
}
