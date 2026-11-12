package azure

import "testing"

func TestOrganizationURLValidation(t *testing.T) {
	for _, raw := range []string{"https://dev.azure.com/acme", "https://dev.azure.com/team-42"} {
		if got, err := ValidateOrganizationURL(raw); err != nil || got != raw {
			t.Errorf("ValidateOrganizationURL(%q) = %q, %v", raw, got, err)
		}
	}
	if got, err := ValidateOrganizationURL("https://dev.azure.com/acme/"); err != nil || got != "https://dev.azure.com/acme" {
		t.Errorf("ValidateOrganizationURL() = %q, %v, want canonical URL", got, err)
	}
	for _, raw := range []string{
		"", "http://dev.azure.com/acme", "https://example.com/acme",
		"https://dev.azure.com", "https://dev.azure.com/acme/project",
		"https://user@dev.azure.com/acme", "https://dev.azure.com:443/acme",
		"https://dev.azure.com/acme?x=1", "https://dev.azure.com/acme#fragment",
		"https://dev.azure.com/-acme", "https://dev.azure.com/acme_works",
	} {
		if _, err := ValidateOrganizationURL(raw); err == nil {
			t.Errorf("ValidateOrganizationURL(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestOrganizationNameReturnsTheSingleSegment(t *testing.T) {
	if got, err := OrganizationName("https://dev.azure.com/acme/"); err != nil || got != "acme" {
		t.Fatalf("OrganizationName() = %q, %v", got, err)
	}
}
