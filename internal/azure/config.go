package azure

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	// ErrInvalidConfig marks caller input the client rejects before any request.
	ErrInvalidConfig = errors.New("azure devops: invalid configuration")
	// ErrNotConfigured marks a workspace without a complete connection.
	ErrNotConfigured = errors.New("azure devops: workspace not configured")
	// ErrReauthRequired means only a new Microsoft sign-in restores access.
	ErrReauthRequired = errors.New("azure devops: Microsoft sign-in expired; sign in again")
)

var organizationNameRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,48}[A-Za-z0-9])?$`)

// OrganizationHost is the only Azure DevOps Services API host.
const OrganizationHost = "dev.azure.com"

// TestConnectionResult reports the result of an authenticated identity probe.
type TestConnectionResult struct {
	OK          bool   `json:"ok"`
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ValidateOrganizationURL accepts Azure DevOps Services organization URLs and
// returns the canonical form `https://dev.azure.com/<org>` without a trailing
// slash.
func ValidateOrganizationURL(raw string) (string, error) {
	normalized := strings.TrimSuffix(raw, "/")
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme != "https" || parsed.Host != OrganizationHost {
		return "", errors.New("organizationUrl must use https://dev.azure.com")
	}
	if parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return "", errors.New("organizationUrl must be a canonical organization URL")
	}
	organization := strings.TrimPrefix(parsed.Path, "/")
	if parsed.Path != "/"+organization || !organizationNameRE.MatchString(organization) || parsed.String() != normalized {
		return "", errors.New("organizationUrl must contain exactly one valid organization name")
	}
	return normalized, nil
}

// OrganizationName returns the organization segment of a canonical URL.
func OrganizationName(organizationURL string) (string, error) {
	canonical, err := ValidateOrganizationURL(organizationURL)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(canonical, "https://"+OrganizationHost+"/"), nil
}

// WithResolvedAssignee returns the request with the concrete assignee the
// caller resolved for its assignee action; an empty name unassigns.
func (r WorkItemAssignmentRequest) WithResolvedAssignee(assignee string) WorkItemAssignmentRequest {
	r.resolvedAssignee = assignee
	r.hasResolvedAssignee = true
	return r
}

// WithResolvedAssignee returns the board update with a resolved assignee.
func (r BoardWorkItemUpdateRequest) WithResolvedAssignee(assignee string) BoardWorkItemUpdateRequest {
	r.resolvedAssignee = assignee
	r.hasResolvedAssignee = true
	return r
}

func validateWorkItemAssignment(request WorkItemAssignmentRequest) error {
	if request.Revision <= 0 {
		return fmt.Errorf("%w: revision required", ErrInvalidConfig)
	}
	if request.AssigneeAction == nil || (*request.AssigneeAction != AssignCurrentUserAction && *request.AssigneeAction != UnassignAction) {
		return fmt.Errorf("%w: an assign_current_user or unassign action is required", ErrInvalidConfig)
	}
	return nil
}

func validateBoardWorkItemUpdate(request BoardWorkItemUpdateRequest) error {
	if request.Revision <= 0 {
		return fmt.Errorf("%w: revision required", ErrInvalidConfig)
	}
	if request.AssigneeAction != nil && *request.AssigneeAction != AssignCurrentUserAction && *request.AssigneeAction != UnassignAction {
		return fmt.Errorf("%w: unsupported assignee action", ErrInvalidConfig)
	}
	if request.AssigneeAction == nil && request.ColumnID == nil && request.ColumnDone == nil {
		return fmt.Errorf("%w: an assignee action, column, or split state is required", ErrInvalidConfig)
	}
	return nil
}
