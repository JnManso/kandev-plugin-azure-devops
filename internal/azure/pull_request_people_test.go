package azure

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
)

func TestDistinctPeopleSortsByNameAndDropsDuplicates(t *testing.T) {
	people := DistinctPeople([]Identity{
		{ID: "b", DisplayName: "rui costa"},
		{ID: "a", DisplayName: "Ana Silva"},
		{ID: "B", DisplayName: "Rui Costa"},
		{ID: "", DisplayName: "No id"},
		{ID: "c", DisplayName: "[Contoso]\\Offers Team"},
	})
	var names []string
	for _, person := range people {
		names = append(names, person.DisplayName)
	}
	if want := []string{"[Contoso]\\Offers Team", "Ana Silva", "rui costa"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("people = %v, want %v", names, want)
	}
}

func TestRESTClientListsThePeopleOfARepositorysPullRequests(t *testing.T) {
	var skips []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/acme/project-1/_apis/git/repositories/repo-1/pullrequests" || r.URL.Query().Get("searchCriteria.status") != "all" {
			t.Errorf("request = %s", r.URL.String())
		}
		skip, _ := strconv.Atoi(r.URL.Query().Get("$skip"))
		skips = append(skips, r.URL.Query().Get("$skip"))
		count := 100
		if skip == 100 {
			count = 3
		}
		value := make([]map[string]any, 0, count)
		for index := range count {
			creator := map[string]string{"id": "ana", "displayName": "Ana Silva"}
			if index == 1 {
				creator = map[string]string{"id": "rui", "displayName": "Rui Costa"}
			}
			value = append(value, map[string]any{
				"pullRequestId": skip + index, "createdBy": creator,
				"reviewers": []map[string]any{{"id": "team", "displayName": "[Contoso]\\Offers Team", "isContainer": true}, {"id": "rui", "displayName": "Rui Costa"}},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"value": value})
	}))
	t.Cleanup(server.Close)

	people, err := newTestRESTClient(t, server, "pat").ListPullRequestPeople(context.Background(), "project-1", "repo-1")
	if err != nil {
		t.Fatalf("ListPullRequestPeople: %v", err)
	}
	if !reflect.DeepEqual(skips, []string{"0", "100"}) {
		t.Fatalf("pages = %v", skips)
	}
	if len(people.Creators) != 2 || people.Creators[0].DisplayName != "Ana Silva" || len(people.Reviewers) != 2 || people.Reviewers[1].ID != "rui" {
		t.Fatalf("people = %+v", people)
	}
}

func TestRESTClientListsThePeopleOfAProjectsPullRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/acme/project-1/_apis/git/pullrequests" {
			t.Errorf("request = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"createdBy": map[string]string{"id": "ana", "displayName": "Ana Silva"}}}})
	}))
	t.Cleanup(server.Close)
	people, err := newTestRESTClient(t, server, "pat").ListPullRequestPeople(context.Background(), "project-1", "")
	if err != nil || len(people.Creators) != 1 || len(people.Reviewers) != 0 {
		t.Fatalf("people = %+v, %v", people, err)
	}
}
