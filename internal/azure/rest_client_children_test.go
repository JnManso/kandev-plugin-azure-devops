package azure

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

var wiqlSourceIDs = regexp.MustCompile(`\[Source\]\.\[System\.Id\] IN \(([0-9,]+)\)`)

// childrenServer links parent 1 to children 11 (Closed), 12 (Active), and
// 13 (Removed), parent 2 to bugs 21 and 22, and parent 3 to extra children
// 1000 to 1000+extra-1.
type childrenServer struct {
	mu          sync.Mutex
	wiqlQueries []string
	stateReads  []string
	extra       int
}

func (s *childrenServer) links(parent int) []int {
	switch parent {
	case 1:
		return []int{11, 12, 13}
	case 2:
		return []int{22, 21}
	case 3:
		ids := make([]int, s.extra)
		for index := range ids {
			ids[index] = 1000 + index
		}
		return ids
	}
	return nil
}

func (s *childrenServer) child(id int) map[string]any {
	itemType, state, priority := "Task", "Active", float64(id)
	switch id {
	case 11:
		state, priority = "Closed", 2
	case 12:
		priority = 1
	case 13:
		state = "Removed"
	case 21, 22:
		itemType, state = "Bug", "Done"
	}
	return map[string]any{"id": id, "rev": 1, "fields": map[string]any{
		"System.Title": "Child " + strconv.Itoa(id), "System.WorkItemType": itemType, "System.State": state,
		"System.TeamProject": "Platform", "Microsoft.VSTS.Common.BacklogPriority": priority,
	}}
}

func (s *childrenServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/acme/project-1/_apis/wit/wiql":
		var request struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		s.mu.Lock()
		s.wiqlQueries = append(s.wiqlQueries, request.Query)
		s.mu.Unlock()
		relations := []map[string]any{}
		match := wiqlSourceIDs.FindStringSubmatch(request.Query)
		for _, raw := range strings.Split(match[1], ",") {
			parent, _ := strconv.Atoi(raw)
			relations = append(relations, map[string]any{"rel": nil, "source": nil, "target": map[string]int{"id": parent}})
			for _, child := range s.links(parent) {
				relations = append(relations, map[string]any{
					"rel": hierarchyForward, "source": map[string]int{"id": parent}, "target": map[string]int{"id": child},
				})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"queryType": "oneHop", "workItemRelations": relations})
	case r.URL.Path == "/acme/project-1/_apis/wit/workitemsbatch":
		var request struct {
			IDs []int `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		value := make([]map[string]any, 0, len(request.IDs))
		for _, id := range request.IDs {
			value = append(value, s.child(id))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": value})
	case strings.HasPrefix(r.URL.Path, "/acme/Platform/_apis/wit/workitemtypes/"):
		itemType := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/acme/Platform/_apis/wit/workitemtypes/"), "/states")
		s.mu.Lock()
		s.stateReads = append(s.stateReads, itemType)
		s.mu.Unlock()
		states := []map[string]string{
			{"name": "New", "category": "Proposed"}, {"name": "Active", "category": "InProgress"},
			{"name": "Closed", "category": "Completed"}, {"name": "Done", "category": "Completed"},
			{"name": "Removed", "category": "Removed"},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": states})
	default:
		http.NotFound(w, r)
	}
}

func TestRESTClientListChildrenGroupsChildrenWithStateCategories(t *testing.T) {
	handler := &childrenServer{}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	result, err := newTestRESTClient(t, server, "pat").ListChildren(t.Context(), "project-1", []int{2, 1, 4})
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if result.Truncated || len(result.Children) != 2 {
		t.Fatalf("result = %+v", result)
	}
	bugs, tasks := result.Children[0], result.Children[1]
	if bugs.ParentID != 2 || len(bugs.Items) != 2 || bugs.Items[0].ID != 21 || bugs.Items[0].StateCategory != "Completed" {
		t.Fatalf("children of 2 = %+v", bugs)
	}
	if tasks.ParentID != 1 || len(tasks.Items) != 2 || tasks.Items[0].ID != 12 || tasks.Items[0].StateCategory != "InProgress" ||
		tasks.Items[1].ID != 11 || tasks.Items[1].StateCategory != "Completed" {
		t.Fatalf("children of 1 = %+v", tasks)
	}
	if len(handler.stateReads) != 2 {
		t.Fatalf("state reads = %v, want one per type", handler.stateReads)
	}
}

func TestRESTClientListChildrenChunksParentsAndBoundsChildren(t *testing.T) {
	handler := &childrenServer{extra: maxBoardChildren + 5}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	parents := make([]int, 450)
	for index := range parents {
		parents[index] = index + 3
	}

	result, err := newTestRESTClient(t, server, "pat").ListChildren(t.Context(), "project-1", parents)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if len(handler.wiqlQueries) != 3 {
		t.Fatalf("link queries = %d, want 3", len(handler.wiqlQueries))
	}
	if !result.Truncated || len(result.Children) != 1 || len(result.Children[0].Items) != maxBoardChildren {
		t.Fatalf("truncated = %v, groups = %d", result.Truncated, len(result.Children))
	}
}

func TestRESTClientListChildrenWithoutParentsReadsNothing(t *testing.T) {
	handler := &childrenServer{}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	result, err := newTestRESTClient(t, server, "pat").ListChildren(t.Context(), "project-1", nil)
	if err != nil || len(result.Children) != 0 || len(handler.wiqlQueries) != 0 {
		t.Fatalf("result = %+v, %v, queries %d", result, err, len(handler.wiqlQueries))
	}
}
