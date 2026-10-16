package models_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/moltisctl/internal/models"
)

func TestImportPublishesAllProvidersAndPreservesMatchingTuning(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "moltis")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, "moltis.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[chat]
priority_models = ["existing"]
[providers]
offered = ["anthropic"]
[providers.anthropic]
api_key = "unrelated-secret"
[providers.custom-test]
base_url = "https://provider.example/v1"
api_key = "fixture-secret"
wire_api = "responses"
models = ["local-extra"]
fetch_models = true
alias = "keep-me"
`), 0o600))
	first := config()
	second := models.Config{Provider: "custom-second", APIURL: "https://second.example/v1", APIType: "openai", APIKey: "second-secret", Names: []string{"second-model"}}
	for range 2 {
		require.NoError(t, models.Import(home, []models.Config{first, second}))
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc struct {
		Chat struct {
			Priority []string `toml:"priority_models"`
		}
		Providers struct {
			Offered   []string
			Anthropic struct {
				APIKey string `toml:"api_key"`
			}
			First struct {
				APIKey      string `toml:"api_key"`
				Models      []string
				FetchModels bool `toml:"fetch_models"`
				Alias       string
			} `toml:"custom-test"`
			Second struct {
				APIKey  string `toml:"api_key"`
				WireAPI string `toml:"wire_api"`
				Models  []string
			} `toml:"custom-second"`
		}
	}
	require.NoError(t, toml.Unmarshal(data, &doc))
	require.Equal(t, []string{"existing"}, doc.Chat.Priority)
	require.Equal(t, []string{"anthropic", "custom-test", "custom-second"}, doc.Providers.Offered)
	require.Equal(t, "unrelated-secret", doc.Providers.Anthropic.APIKey)
	require.Equal(t, "fixture-secret", doc.Providers.First.APIKey)
	require.Equal(t, []string{"local-extra", "one", "two"}, doc.Providers.First.Models)
	require.True(t, doc.Providers.First.FetchModels)
	require.Equal(t, "keep-me", doc.Providers.First.Alias)
	require.Equal(t, "second-secret", doc.Providers.Second.APIKey)
	require.Equal(t, "chat-completions", doc.Providers.Second.WireAPI)
	require.Equal(t, []string{"second-model"}, doc.Providers.Second.Models)
}

func TestImportRejectsWholeBatchWithoutChangingDestination(t *testing.T) {
	for _, scenario := range []string{"credential conflict", "endpoint conflict", "format conflict", "disabled", "invalid models", "unsupported", "duplicate", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			c := config()
			require.NoError(t, models.Configure(home, c))
			path := filepath.Join(home, ".config", "moltis", "moltis.toml")
			// Check the late member of the batch so sequential writes would fail.
			first := models.Config{Provider: "custom-first", APIURL: "https://first.example/v1", APIType: "openai", APIKey: "first-secret", Names: []string{"first-model"}}
			batch := []models.Config{first, c}
			switch scenario {
			case "credential conflict":
				batch[1].APIKey = "different-secret"
			case "endpoint conflict":
				batch[1].APIURL = "https://different.example/v1"
			case "format conflict":
				batch[1].APIType = "openai"
			case "unsupported":
				batch[1].APIType = "anthropic"
			case "duplicate":
				batch = []models.Config{c, c}
			case "empty":
				batch = nil
			case "disabled", "invalid models":
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, toml.Unmarshal(data, &doc))
				p := doc["providers"].(map[string]any)[c.Provider].(map[string]any)
				if scenario == "disabled" {
					p["enabled"] = false
				} else {
					p["models"] = "invalid"
				}
				data, err = toml.Marshal(doc)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, data, 0o600))
			}
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			err = models.Import(home, batch)
			require.Error(t, err)
			for _, secret := range []string{"fixture-secret", "first-secret", "different-secret"} {
				require.NotContains(t, err.Error(), secret)
			}
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
