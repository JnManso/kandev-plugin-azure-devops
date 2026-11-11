package plugin

import (
	"strings"
	"testing"

	"kandev-plugin-azure-devops/internal/store"
)

func TestSaveStoresThePATAsAWorkspaceSecretAndReportsHealth(t *testing.T) {
	h := newHarness(t)
	status, body := h.action("ws-1", "connection.save", map[string]any{
		"organizationUrl": "https://dev.azure.com/acme/", "defaultProjectName": "Platform Team", "pat": "good-pat",
	})
	if status != 200 {
		t.Fatalf("status = %d body = %v", status, body)
	}
	if secret, _ := h.host.Secret(store.PATKey("ws-1")); secret != "good-pat" {
		t.Fatalf("stored PAT = %q", secret)
	}
	connection := field(body, "connection")
	if field(connection, "organizationUrl") != "https://dev.azure.com/acme" || field(connection, "hasSecret") != true ||
		field(connection, "lastOk") != true || field(connection, "defaultProjectName") != "Platform Team" {
		t.Fatalf("connection = %v", connection)
	}
	if strings.Contains(flatten(body), "good-pat") {
		t.Fatalf("the response leaks the PAT: %v", body)
	}
}

func TestSaveRejectsANonCanonicalOrganizationAndAMissingPAT(t *testing.T) {
	h := newHarness(t)
	if status, body := h.action("ws-1", "connection.save", map[string]any{"organizationUrl": "https://acme.visualstudio.com", "pat": "good-pat"}); status != 400 {
		t.Fatalf("visualstudio.com organization status = %d body = %v", status, body)
	}
	status, body := h.action("ws-1", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/acme"})
	if status != 400 || !strings.Contains(field(body, "error").(string), "personal access token") {
		t.Fatalf("missing PAT status = %d body = %v", status, body)
	}
}

func TestSaveWithoutAPATKeepsTheStoredTokenAndGeneration(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	before, _, _ := store.New(h.host).Connection(t.Context(), "ws-1")
	status, _ := h.action("ws-1", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/acme", "defaultProjectName": "Ops"})
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	after, _, _ := store.New(h.host).Connection(t.Context(), "ws-1")
	if after.Generation != before.Generation || after.DefaultProjectName != "Ops" {
		t.Fatalf("generation %d -> %d, project %q", before.Generation, after.Generation, after.DefaultProjectName)
	}
	if secret, _ := h.host.Secret(store.PATKey("ws-1")); secret != "good-pat" {
		t.Fatalf("stored PAT = %q", secret)
	}
}

func TestANewPATIncrementsTheGenerationAndRecordsFailedHealth(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	status, body := h.action("ws-1", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/acme", "pat": "expired-pat"})
	if status != 200 {
		t.Fatalf("status = %d body = %v", status, body)
	}
	connection, _, _ := store.New(h.host).Connection(t.Context(), "ws-1")
	if connection.Generation != 2 {
		t.Fatalf("generation = %d, want 2", connection.Generation)
	}
	if field(body, "connection", "lastOk") != false || !strings.Contains(field(body, "connection", "lastError").(string), "401") {
		t.Fatalf("connection = %v", field(body, "connection"))
	}
}

func TestAFailedStateWriteRestoresThePreviousPAT(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	h.host.FailSetState = "connection"
	status, _ := h.action("ws-1", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/acme", "pat": "other-pat"})
	if status < 500 {
		t.Fatalf("status = %d, want a server error", status)
	}
	if secret, _ := h.host.Secret(store.PATKey("ws-1")); secret != "good-pat" {
		t.Fatalf("PAT after failed save = %q, want the previous one", secret)
	}
}

func TestTestProbesWithoutPersistingAndReportsRejection(t *testing.T) {
	h := newHarness(t)
	status, body := h.action("ws-1", "connection.test", map[string]any{"organizationUrl": "https://dev.azure.com/acme", "pat": "good-pat"})
	if status != 200 || field(body, "result", "ok") != true || field(body, "result", "displayName") != "Ada" {
		t.Fatalf("status = %d body = %v", status, body)
	}
	if _, found := h.host.Secret(store.PATKey("ws-1")); found {
		t.Fatal("connection.test persisted the PAT")
	}
	_, body = h.action("ws-1", "connection.test", map[string]any{"organizationUrl": "https://dev.azure.com/acme", "pat": "bad"})
	if field(body, "result", "ok") != false || !strings.Contains(field(body, "result", "error").(string), "401") {
		t.Fatalf("rejected probe = %v", body)
	}
}

func TestTestWithoutAPATUsesTheStoredTokenOnlyForTheStoredOrganization(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	if _, body := h.action("ws-1", "connection.test", map[string]any{"organizationUrl": "https://dev.azure.com/acme"}); field(body, "result", "ok") != true {
		t.Fatalf("stored-token probe = %v", body)
	}
	if status, _ := h.action("ws-1", "connection.test", map[string]any{"organizationUrl": "https://dev.azure.com/other"}); status != 400 {
		t.Fatalf("probe of another organization without a PAT status = %d, want 400", status)
	}
}

func TestCopyCopiesSettingsButNeverTheToken(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-source")
	status, body := h.action("ws-target", "connection.copy", map[string]any{"sourceWorkspaceId": "ws-source"})
	if status != 200 {
		t.Fatalf("status = %d body = %v", status, body)
	}
	if _, found := h.host.Secret(store.PATKey("ws-target")); found {
		t.Fatal("copy copied the PAT")
	}
	if field(body, "connection", "reauthRequired") != true || field(body, "connection", "organizationUrl") != "https://dev.azure.com/acme" {
		t.Fatalf("copied connection = %v", field(body, "connection"))
	}
	if status, _ := h.action("ws-target", "repositories.list", map[string]any{}); status != 409 {
		t.Fatalf("repositories.list before re-entering the PAT status = %d, want 409", status)
	}
	status, body = h.action("ws-target", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/acme", "pat": "good-pat"})
	if status != 200 || field(body, "connection", "reauthRequired") != false {
		t.Fatalf("save after copy status = %d body = %v", status, body)
	}
}

func TestCopyKeepsTheTargetTokenForTheSameOrganization(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-source")
	h.connect("ws-target")
	_, body := h.action("ws-target", "connection.copy", map[string]any{"sourceWorkspaceId": "ws-source"})
	if field(body, "connection", "reauthRequired") != false {
		t.Fatalf("copied connection = %v", field(body, "connection"))
	}
	if secret, _ := h.host.Secret(store.PATKey("ws-target")); secret != "good-pat" {
		t.Fatalf("target PAT = %q", secret)
	}
}

func TestCopyRejectsTheSameOrAnUnconfiguredSource(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	if status, _ := h.action("ws-1", "connection.copy", map[string]any{"sourceWorkspaceId": "ws-1"}); status != 400 {
		t.Fatalf("same-workspace copy status = %d", status)
	}
	if status, _ := h.action("ws-1", "connection.copy", map[string]any{"sourceWorkspaceId": "ws-none"}); status != 404 {
		t.Fatalf("unconfigured-source copy status = %d", status)
	}
}

func TestDeleteRemovesTheConnectionAndToken(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	if status, _ := h.action("ws-1", "connection.delete", nil); status != 200 {
		t.Fatalf("delete status = %d", status)
	}
	if _, found := h.host.Secret(store.PATKey("ws-1")); found {
		t.Fatal("delete kept the PAT")
	}
	if _, body := h.action("ws-1", "connection.get", nil); field(body, "connection", "configured") != false {
		t.Fatalf("connection after delete = %v", body)
	}
}

func TestProjectsListUsesTheStoredConnection(t *testing.T) {
	h := newHarness(t)
	if status, _ := h.action("ws-1", "projects.list", nil); status != 409 {
		t.Fatalf("unconfigured projects.list status = %d, want 409", status)
	}
	h.connect("ws-1")
	_, body := h.action("ws-1", "projects.list", nil)
	if projects, _ := field(body, "projects").([]any); len(projects) != 2 {
		t.Fatalf("projects = %v", body)
	}
}

func TestHealthPollerProbesOnlyConfiguredWorkspaces(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-1")
	h.host.AddWorkspace("ws-1")
	h.host.AddWorkspace("ws-2")
	h.azure.validPAT = "rotated"
	services, _ := h.runtime.ready()
	NewHealthPoller(services.connections, h.host.Workspaces()).PollOnce(t.Context())
	health, _, _ := store.New(h.host).Health(t.Context(), "ws-1")
	if health.OK || !strings.Contains(health.Error, "401") {
		t.Fatalf("ws-1 health = %+v", health)
	}
	if _, found, _ := store.New(h.host).Health(t.Context(), "ws-2"); found {
		t.Fatal("the poller recorded health for an unconfigured workspace")
	}
}

func TestCopyKeepsTheTargetTokenWhenSavingTheCopyFails(t *testing.T) {
	h := newHarness(t)
	h.connect("ws-target")
	if status, body := h.action("ws-source", "connection.save", map[string]any{"organizationUrl": "https://dev.azure.com/other", "pat": "good-pat"}); status != 200 {
		t.Fatalf("source save status = %d body = %v", status, body)
	}
	h.host.FailSetState = "connection"
	if status, _ := h.action("ws-target", "connection.copy", map[string]any{"sourceWorkspaceId": "ws-source"}); status == 200 {
		t.Fatal("copy succeeded although the connection could not be saved")
	}
	if secret, _ := h.host.Secret(store.PATKey("ws-target")); secret != "good-pat" {
		t.Fatalf("target PAT after a failed copy = %q", secret)
	}
}
