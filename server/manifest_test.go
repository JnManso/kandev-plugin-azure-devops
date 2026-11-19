package main

import (
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"kandev-plugin-azure-devops/internal/plugin"
)

// officialIconSHA256 pins assets/icon.svg to the unmodified Microsoft icon
// recorded in assets/NOTICE.md.
const officialIconSHA256 = "15b5068ceeb28f0f3cf566aa8cf99619f99d47059d7a91fda0ef47091a71a041"

type manifestIdentity struct {
	ID         string   `yaml:"id"`
	APIVersion int      `yaml:"api_version"`
	Version    string   `yaml:"version"`
	Categories []string `yaml:"categories"`
	RepoURL    string   `yaml:"repo_url"`
	Icon       string   `yaml:"icon"`
}

func readManifest(t *testing.T) manifestIdentity {
	t.Helper()
	contents, err := os.ReadFile("../manifest.yaml")
	require.NoError(t, err)
	var manifest manifestIdentity
	require.NoError(t, yaml.Unmarshal(contents, &manifest))
	return manifest
}

func TestManifestIdentityMatchesRuntimeAndMakefile(t *testing.T) {
	manifest := readManifest(t)
	require.Equal(t, plugin.ID, manifest.ID)
	require.Equal(t, 2, manifest.APIVersion)
	require.Equal(t, []string{"connector"}, manifest.Categories)
	require.Equal(t, "https://github.com/JnManso/kandev-plugin-azure-devops", manifest.RepoURL)

	makefile, err := os.ReadFile("../Makefile")
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`(?m)^BIN := bin/`+regexp.QuoteMeta(manifest.ID)+`$`), string(makefile))
	require.Regexp(t, regexp.MustCompile(`(?m)^VERSION := `+regexp.QuoteMeta(manifest.Version)+`$`), string(makefile))
}

func TestManifestIconIsThePinnedOfficialMark(t *testing.T) {
	manifest := readManifest(t)
	require.Equal(t, "assets/icon.svg", manifest.Icon)

	icon, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(manifest.Icon)))
	require.NoError(t, err)
	require.Equal(t, officialIconSHA256, fmt.Sprintf("%x", sha256.Sum256(icon)))

	var root struct{ XMLName xml.Name }
	require.NoError(t, xml.Unmarshal(icon, &root))
	require.Equal(t, "svg", root.XMLName.Local)

	lower := strings.ToLower(string(icon))
	for _, forbidden := range []string{"<script", "<foreignobject", "href=", "url(http", "url(//", "currentcolor"} {
		require.NotContains(t, lower, forbidden)
	}
}

// actionKeyPattern mirrors Kandev's manifest validator; plugin-pack does not
// check it, so an invalid key would only fail at install time.
var actionKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func TestManifestActionKeysMatchTheHostGrammar(t *testing.T) {
	contents, err := os.ReadFile("../manifest.yaml")
	require.NoError(t, err)
	var manifest struct {
		Actions []struct {
			Key string `yaml:"key"`
		} `yaml:"actions"`
	}
	require.NoError(t, yaml.Unmarshal(contents, &manifest))
	require.NotEmpty(t, manifest.Actions)
	for _, action := range manifest.Actions {
		require.Regexp(t, actionKeyPattern, action.Key)
	}
}
