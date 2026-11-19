package azure

import "testing"

func TestParseRemoteURLAcceptsTheFourAzureForms(t *testing.T) {
	want := RemoteRepository{Organization: "acme", Project: "Platform Team", Repository: "api"}
	for _, raw := range []string{
		"https://dev.azure.com/acme/Platform%20Team/_git/api",
		"https://acme@dev.azure.com/acme/Platform%20Team/_git/api",
		"https://dev.azure.com/acme/Platform%20Team/_git/api.git",
		"git@ssh.dev.azure.com:v3/acme/Platform%20Team/api",
		"ssh://git@ssh.dev.azure.com/v3/acme/Platform%20Team/api",
		"https://acme.visualstudio.com/Platform%20Team/_git/api",
		"https://acme.visualstudio.com/DefaultCollection/Platform%20Team/_git/api",
	} {
		got, err := ParseRemoteURL(raw)
		if err != nil {
			t.Fatalf("ParseRemoteURL(%q) error = %v", raw, err)
		}
		if got != want {
			t.Fatalf("ParseRemoteURL(%q) = %+v, want %+v", raw, got, want)
		}
	}
}

func TestParseRemoteURLRejectsOtherHostsAndShapes(t *testing.T) {
	for _, raw := range []string{
		"", "https://github.com/acme/api", "http://dev.azure.com/acme/p/_git/r",
		"https://dev.azure.com/acme/p/r", "https://dev.azure.com/acme/_git/r",
		"https://dev.azure.com:8443/acme/p/_git/r", "https://dev.azure.com/acme/p/_git/r?x=1",
		"git@ssh.dev.azure.com:v2/acme/p/r", "https://dev.azure.com/-acme/p/_git/r",
		"https://acme.visualstudio.com/p/r",
	} {
		if _, err := ParseRemoteURL(raw); !IsNotAzureRemote(err) {
			t.Fatalf("ParseRemoteURL(%q) error = %v, want not-an-Azure-remote", raw, err)
		}
	}
}

func TestCanonicalCloneURLEscapesProjectAndRepository(t *testing.T) {
	got := CanonicalCloneURL("acme", "Platform Team", "api")
	if got != "https://dev.azure.com/acme/Platform%20Team/_git/api" {
		t.Fatalf("CanonicalCloneURL() = %q", got)
	}
}
