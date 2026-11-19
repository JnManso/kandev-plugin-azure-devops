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

// iconSHA256 pins assets/icon.svg: the official Microsoft shape recorded in
// assets/NOTICE.md, filled with iconFill.
const iconSHA256 = "a069b591b94ace8577a6d34c85051e615abbaa52263aa4acbdbc6a8e50e36f2f"

// officialIconPath is the unchanged shape of the official Azure DevOps icon.
const officialIconPath = "M17,4v9.74l-4,3.28-6.2-2.26V17L3.29,12.41l10.23.8V4.44Zm-3.41.49L7.85,1V3.29L2.58,4.84,1,6.87v4.61l2.26,1V6.57Z"

// iconFill is the neutral gray the catalog icon uses on light and dark themes.
const iconFill = "#8a8a8a"

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

func TestManifestIconIsTheOfficialShapeInGray(t *testing.T) {
	manifest := readManifest(t)
	require.Equal(t, "assets/icon.svg", manifest.Icon)

	icon, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(manifest.Icon)))
	require.NoError(t, err)
	require.Equal(t, iconSHA256, fmt.Sprintf("%x", sha256.Sum256(icon)))
	require.Contains(t, string(icon), `d="`+officialIconPath+`"`)
	require.Equal(t, 1, strings.Count(string(icon), "fill="))
	require.Contains(t, string(icon), `fill="`+iconFill+`"`)

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
