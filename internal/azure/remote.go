package azure

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// RemoteRepository is the organization, project, and repository named by an
// Azure Repos Git remote URL. Project and repository are unescaped.
type RemoteRepository struct {
	Organization string
	Project      string
	Repository   string
}

// CloneURL returns the canonical credential-free HTTPS clone URL.
func (r RemoteRepository) CloneURL() string {
	return CanonicalCloneURL(r.Organization, r.Project, r.Repository)
}

// CanonicalCloneURL builds `https://dev.azure.com/<org>/<project>/_git/<repo>`.
func CanonicalCloneURL(organization, project, repository string) string {
	return fmt.Sprintf("https://%s/%s/%s/_git/%s", OrganizationHost,
		url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repository))
}

var errNotAzureRemote = errors.New("not an Azure Repos remote URL")

// ParseRemoteURL accepts the four Azure Repos remote forms:
//
//	https://dev.azure.com/<org>/<project>/_git/<repo>
//	https://<user>@dev.azure.com/<org>/<project>/_git/<repo>
//	git@ssh.dev.azure.com:v3/<org>/<project>/<repo>
//	https://<org>.visualstudio.com/[DefaultCollection/]<project>/_git/<repo>
func ParseRemoteURL(raw string) (RemoteRepository, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RemoteRepository{}, errNotAzureRemote
	}
	if rest, ok := strings.CutPrefix(raw, "git@ssh.dev.azure.com:"); ok {
		return parseSSHPath(rest)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return RemoteRepository{}, errNotAzureRemote
	}
	host := strings.ToLower(parsed.Hostname())
	if parsed.Scheme == "ssh" && host == "ssh.dev.azure.com" {
		return parseSSHPath(strings.TrimPrefix(parsed.Path, "/"))
	}
	if parsed.Scheme != "https" || parsed.Port() != "" {
		return RemoteRepository{}, errNotAzureRemote
	}
	segments, err := pathSegments(parsed)
	if err != nil {
		return RemoteRepository{}, err
	}
	switch {
	case host == OrganizationHost:
		if len(segments) != 4 || segments[2] != "_git" {
			return RemoteRepository{}, errNotAzureRemote
		}
		return newRemote(segments[0], segments[1], segments[3])
	case strings.HasSuffix(host, ".visualstudio.com"):
		organization := strings.TrimSuffix(host, ".visualstudio.com")
		if len(segments) == 4 && strings.EqualFold(segments[0], "DefaultCollection") {
			segments = segments[1:]
		}
		if len(segments) != 3 || segments[1] != "_git" {
			return RemoteRepository{}, errNotAzureRemote
		}
		return newRemote(organization, segments[0], segments[2])
	default:
		return RemoteRepository{}, errNotAzureRemote
	}
}

func parseSSHPath(path string) (RemoteRepository, error) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) != 4 || segments[0] != "v3" {
		return RemoteRepository{}, errNotAzureRemote
	}
	project, err := url.PathUnescape(segments[2])
	if err != nil {
		return RemoteRepository{}, errNotAzureRemote
	}
	repository, err := url.PathUnescape(segments[3])
	if err != nil {
		return RemoteRepository{}, errNotAzureRemote
	}
	return newRemote(segments[1], project, repository)
}

func pathSegments(parsed *url.URL) ([]string, error) {
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errNotAzureRemote
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for _, segment := range segments {
		if segment == "" {
			return nil, errNotAzureRemote
		}
	}
	return segments, nil
}

func newRemote(organization, project, repository string) (RemoteRepository, error) {
	repository = strings.TrimSuffix(repository, ".git")
	if !organizationNameRE.MatchString(organization) || strings.TrimSpace(project) == "" || strings.TrimSpace(repository) == "" {
		return RemoteRepository{}, errNotAzureRemote
	}
	return RemoteRepository{Organization: organization, Project: project, Repository: repository}, nil
}

// IsNotAzureRemote reports whether err means the URL is not an Azure remote.
func IsNotAzureRemote(err error) bool { return errors.Is(err, errNotAzureRemote) }
