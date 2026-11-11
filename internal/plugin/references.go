package plugin

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

var errInvalidReference = errors.New("enter an Azure DevOps URL or a reference such as Project/Repository!42")

// pullRequestReference identifies a pull request typed by a user. Empty
// project and repository mean "a repository of the task".
type pullRequestReference struct {
	Organization string
	Project      string
	Repository   string
	ID           int
}

// workItemReference identifies a work item typed by a user. Work item IDs are
// unique within an organization, so the project is optional.
type workItemReference struct {
	Organization string
	Project      string
	ID           int
}

// parsePullRequestReference accepts a pull request URL, Project/Repository!42,
// !42, #42, or 42.
func parsePullRequestReference(raw string) (pullRequestReference, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return pullRequestReference{}, errInvalidReference
	}
	if strings.Contains(value, "://") {
		return parsePullRequestURL(value)
	}
	if index := strings.LastIndex(value, "!"); index > 0 {
		parts := strings.Split(value[:index], "/")
		id, ok := positiveID(value[index+1:])
		if len(parts) != 2 || !ok || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return pullRequestReference{}, errInvalidReference
		}
		return pullRequestReference{Project: strings.TrimSpace(parts[0]), Repository: strings.TrimSpace(parts[1]), ID: id}, nil
	}
	id, ok := positiveID(strings.TrimLeft(value, "!#"))
	if !ok {
		return pullRequestReference{}, errInvalidReference
	}
	return pullRequestReference{ID: id}, nil
}

// parseWorkItemReference accepts a work item URL, Project#42, #42, or 42.
func parseWorkItemReference(raw string) (workItemReference, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return workItemReference{}, errInvalidReference
	}
	if strings.Contains(value, "://") {
		return parseWorkItemURL(value)
	}
	if index := strings.LastIndex(value, "#"); index > 0 {
		id, ok := positiveID(value[index+1:])
		project := strings.TrimSpace(value[:index])
		if !ok || project == "" || strings.Contains(project, "/") {
			return workItemReference{}, errInvalidReference
		}
		return workItemReference{Project: project, ID: id}, nil
	}
	id, ok := positiveID(strings.TrimPrefix(value, "#"))
	if !ok {
		return workItemReference{}, errInvalidReference
	}
	return workItemReference{ID: id}, nil
}

// azureURLSegments returns the organization and the path segments that follow
// it for dev.azure.com and <org>.visualstudio.com addresses.
func azureURLSegments(raw string) (string, []string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return "", nil, false
	}
	segments := make([]string, 0, 8)
	for _, segment := range strings.Split(parsed.EscapedPath(), "/") {
		if segment == "" {
			continue
		}
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			return "", nil, false
		}
		segments = append(segments, decoded)
	}
	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "dev.azure.com":
		if len(segments) == 0 {
			return "", nil, false
		}
		return segments[0], segments[1:], true
	case strings.HasSuffix(host, ".visualstudio.com"):
		organization := strings.TrimSuffix(host, ".visualstudio.com")
		if organization == "" || strings.Contains(organization, ".") {
			return "", nil, false
		}
		return organization, segments, true
	default:
		return "", nil, false
	}
}

func parsePullRequestURL(raw string) (pullRequestReference, error) {
	organization, segments, ok := azureURLSegments(raw)
	// <project>/_git/<repository>/pullrequest/<id>
	if !ok || len(segments) != 5 || segments[1] != "_git" || !strings.EqualFold(segments[3], "pullrequest") {
		return pullRequestReference{}, errInvalidReference
	}
	id, ok := positiveID(segments[4])
	if !ok {
		return pullRequestReference{}, errInvalidReference
	}
	return pullRequestReference{Organization: organization, Project: segments[0], Repository: segments[2], ID: id}, nil
}

func parseWorkItemURL(raw string) (workItemReference, error) {
	organization, segments, ok := azureURLSegments(raw)
	// <project>/_workitems/edit/<id>
	if !ok || len(segments) != 4 || segments[1] != "_workitems" || segments[2] != "edit" {
		return workItemReference{}, errInvalidReference
	}
	id, ok := positiveID(segments[3])
	if !ok {
		return workItemReference{}, errInvalidReference
	}
	return workItemReference{Organization: organization, Project: segments[0], ID: id}, nil
}

func positiveID(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 10 {
		return 0, false
	}
	id, err := strconv.Atoi(value)
	return id, err == nil && id > 0
}

// workItemReferenceKey is the composer key of a work item: Project#42.
func workItemReferenceKey(project string, id int) string {
	return project + "#" + strconv.Itoa(id)
}

// pullRequestReferenceKey is the composer key of a pull request:
// Project/Repository!42.
func pullRequestReferenceKey(project, repository string, id int) string {
	return project + "/" + repository + "!" + strconv.Itoa(id)
}
