package models_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/moltisctl/internal/models"
)

func config() models.Config {
	return models.Config{Provider: "custom-test", APIURL: "https://provider.example/v1", APIType: "openai-responses", APIKey: "fixture-secret", Names: []string{"one", "two"}}
}

func TestConfigurePreservesUnrelatedSettingsAndRotatesCredentials(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "moltis")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, "moltis.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[chat]
priority_models = ["other"]
[providers]
offered = ["anthropic"]
[providers.anthropic]
api_key = "unrelated-secret"
[providers.custom-test]
url = "https://old.example/v1"
alias = "keep-me"
fetch_models = false
[providers.custom-test.model_overrides.one]
context_window = 12345
`), 0o644))
	c := config()
	for range 2 {
		require.NoError(t, models.Configure(home, c))
		c.APIKey = "rotated-secret"
		c.APIType = "openai"
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc struct {
		Chat struct {
			PriorityModels []string `toml:"priority_models"`
		}
		Providers struct {
			Offered   []string
			Anthropic struct {
				APIKey string `toml:"api_key"`
			}
			Custom struct {
				Enabled     bool
				APIKey      string `toml:"api_key"`
				BaseURL     string `toml:"base_url"`
				URL         *string
				Models      []string
				Alias       string
				FetchModels bool   `toml:"fetch_models"`
				WireAPI     string `toml:"wire_api"`
				Overrides   map[string]struct {
					ContextWindow int `toml:"context_window"`
				} `toml:"model_overrides"`
			} `toml:"custom-test"`
		}
	}
	require.NoError(t, toml.Unmarshal(data, &doc))
	require.Equal(t, []string{"other"}, doc.Chat.PriorityModels)
	require.Equal(t, []string{"anthropic", "custom-test"}, doc.Providers.Offered)
	require.Equal(t, "unrelated-secret", doc.Providers.Anthropic.APIKey)
	p := doc.Providers.Custom
	require.True(t, p.Enabled)
	require.Equal(t, "rotated-secret", p.APIKey)
	require.Equal(t, c.APIURL, p.BaseURL)
	require.Nil(t, p.URL)
	require.Equal(t, []string{"one", "two"}, p.Models)
	require.Equal(t, "keep-me", p.Alias)
	require.False(t, p.FetchModels)
	require.Equal(t, "chat-completions", p.WireAPI)
	require.Equal(t, 12345, p.Overrides["one"].ContextWindow)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestConfigureRefusesMalformedOrLinkedConfiguration(t *testing.T) {
	for _, scenario := range []string{"malformed", "file link", "directory link", "invalid provider table"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".config", "moltis")
			require.NoError(t, os.MkdirAll(dir, 0o700))
			path := filepath.Join(dir, "moltis.toml")
			original := []byte(`api_key = "fixture-secret`)
			if scenario == "invalid provider table" {
				original = []byte(`providers = "fixture-secret"`)
			}
			outside := filepath.Join(t.TempDir(), "outside")
			require.NoError(t, os.WriteFile(outside, original, 0o600))
			switch scenario {
			case "file link":
				require.NoError(t, os.Symlink(outside, path))
			case "directory link":
				require.NoError(t, os.Remove(dir))
				require.NoError(t, os.Symlink(filepath.Dir(outside), dir))
			default:
				require.NoError(t, os.WriteFile(path, original, 0o600))
			}
			err := models.Configure(home, config())
			require.Error(t, err)
			require.NotContains(t, err.Error(), "fixture-secret")
			unchanged, err := os.ReadFile(outside)
			require.NoError(t, err)
			require.Equal(t, original, unchanged)
			if scenario == "malformed" || scenario == "invalid provider table" {
				unchanged, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, original, unchanged)
			}
		})
	}
}
