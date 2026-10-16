package instance

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/moltisctl/internal/models"
)

func importSource(t *testing.T) (models.ShelleySource, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "instances", "source", "home", ".local", "state", "shelley", "shelley.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"version":1,"instances":{"source":{"name":"source"}}}`), 0o600))
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	script, err := os.ReadFile(filepath.Join("..", "models", "testdata", "shelley.sql"))
	require.NoError(t, err)
	_, err = db.Exec(string(script))
	require.NoError(t, err)
	return models.ShelleySource{StateDir: dir, Name: "source"}, db
}

func TestImportShelleyUsesOfflineLifecycleAndKeepsFailuresUnchanged(t *testing.T) {
	for _, scenario := range []string{"new", "retained", "active", "conflict", "invalid source"} {
		t.Run(scenario, func(t *testing.T) {
			source, db := importSource(t)
			var actions []string
			a := testApplication(t, func(action string) error { actions = append(actions, action); return nil })
			if scenario != "new" {
				seed(t, a, 19123, scenario == "active")
			}
			home := filepath.Join(a.opts.StateDir, "instances", "demo", "home")
			configPath := filepath.Join(home, ".config", "moltis", "moltis.toml")
			if scenario == "conflict" {
				configs, err := models.ReadShelley(t.Context(), source)
				require.NoError(t, err)
				configs[2].APIKey = "destination-secret"
				require.NoError(t, os.MkdirAll(filepath.Dir(home), 0o700))
				require.NoError(t, models.Configure(home, configs[2]))
			}
			if scenario == "invalid source" {
				_, err := db.Exec(`UPDATE models SET provider_type='gemini' WHERE model_id='e'`)
				require.NoError(t, err)
			}
			stateBefore, _ := os.ReadFile(filepath.Join(a.opts.StateDir, "state.json"))
			configBefore, _ := os.ReadFile(configPath)
			err := a.ImportShelleyModels(t.Context(), "demo", source)
			if scenario == "new" || scenario == "retained" {
				require.NoError(t, err)
				r := diskRecord(t, a)
				require.False(t, r.Desired)
				if scenario == "retained" {
					require.Equal(t, 19123, r.Port)
				}
				data, err := os.ReadFile(configPath)
				require.NoError(t, err)
				var doc struct{ Providers map[string]map[string]any }
				require.NoError(t, toml.Unmarshal(data, &doc))
				require.Len(t, doc.Providers, 3)
				t.Setenv("MOLTISCTL_RESUME", "1")
				before := len(actions)
				require.NoError(t, a.Create(t.Context(), "demo"))
				require.Len(t, actions, before)
			} else {
				require.Error(t, err)
				if scenario == "conflict" {
					require.ErrorContains(t, err, "conflicts")
				}
				if scenario == "active" {
					require.ErrorContains(t, err, "delete the instance")
				}
				if scenario == "invalid source" {
					require.ErrorContains(t, err, "unsupported")
				}
				stateAfter, _ := os.ReadFile(filepath.Join(a.opts.StateDir, "state.json"))
				configAfter, _ := os.ReadFile(configPath)
				require.Equal(t, stateBefore, stateAfter)
				require.Equal(t, configBefore, configAfter)
			}
			stateAfter, _ := os.ReadFile(filepath.Join(a.opts.StateDir, "state.json"))
			for _, key := range []string{"chat-fixture-key", "responses-fixture-key", "other-fixture-key", "destination-secret"} {
				require.NotContains(t, string(stateAfter), key)
			}
			for _, action := range actions {
				require.Contains(t, []string{"check container", "inspect container"}, action)
			}
		})
	}
}
