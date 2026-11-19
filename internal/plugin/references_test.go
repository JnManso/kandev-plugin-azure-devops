package plugin

import "testing"

func TestParsePullRequestReference(t *testing.T) {
	cases := map[string]pullRequestReference{
		"https://dev.azure.com/acme/Platform%20Team/_git/api/pullrequest/42": {Organization: "acme", Project: "Platform Team", Repository: "api", ID: 42},
		"https://acme.visualstudio.com/Platform/_git/api/pullrequest/7":      {Organization: "acme", Project: "Platform", Repository: "api", ID: 7},
		"Platform/api!9": {Project: "Platform", Repository: "api", ID: 9},
		"!12":            {ID: 12},
		"#13":            {ID: 13},
		" 14 ":           {ID: 14},
	}
	for input, want := range cases {
		got, err := parsePullRequestReference(input)
		if err != nil || got != want {
			t.Errorf("parsePullRequestReference(%q) = %+v, %v; want %+v", input, got, err, want)
		}
	}
	for _, input := range []string{"", "abc", "!0", "a/b/c!1", "http://dev.azure.com/acme/P/_git/r/pullrequest/1",
		"https://github.com/acme/api/pull/1", "https://dev.azure.com/acme/P/_git/r/pullrequest/x", "https://user@dev.azure.com/acme/P/_git/r/pullrequest/1"} {
		if _, err := parsePullRequestReference(input); err == nil {
			t.Errorf("parsePullRequestReference(%q) succeeded", input)
		}
	}
}

func TestParseWorkItemReference(t *testing.T) {
	cases := map[string]workItemReference{
		"https://dev.azure.com/acme/Platform/_workitems/edit/101": {Organization: "acme", Project: "Platform", ID: 101},
		"Platform#101": {Project: "Platform", ID: 101},
		"#101":         {ID: 101},
		"101":          {ID: 101},
	}
	for input, want := range cases {
		got, err := parseWorkItemReference(input)
		if err != nil || got != want {
			t.Errorf("parseWorkItemReference(%q) = %+v, %v; want %+v", input, got, err, want)
		}
	}
	for _, input := range []string{"", "x", "#", "a/b#1", "https://dev.azure.com/acme/P/_workitems/101"} {
		if _, err := parseWorkItemReference(input); err == nil {
			t.Errorf("parseWorkItemReference(%q) succeeded", input)
		}
	}
}
