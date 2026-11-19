package plugin

import "testing"

func TestWorkspacesDescribeNamesOnlyTheRequestedWorkspaces(t *testing.T) {
	h := newHarness(t)
	h.host.AddNamedWorkspace("ws-1", "Bravo")
	h.host.AddNamedWorkspace("ws-2", "Alpha")
	h.host.AddNamedWorkspace("ws-3", "Hidden")
	h.connect("ws-2")
	status, body := h.action("ws-1", "workspaces.describe", map[string]any{"workspaceIds": []string{"ws-1", "ws-2", "ws-missing"}})
	if status != 200 {
		t.Fatalf("status = %d body = %v", status, body)
	}
	workspaces, _ := field(body, "workspaces").([]any)
	if len(workspaces) != 2 || field(workspaces[0], "name") != "Alpha" || field(workspaces[0], "configured") != true ||
		field(workspaces[1], "name") != "Bravo" || field(workspaces[1], "configured") != false {
		t.Fatalf("workspaces = %v", workspaces)
	}
}
