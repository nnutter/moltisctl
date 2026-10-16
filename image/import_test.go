package image_test

import (
	"database/sql"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/moltisctl/internal/instance"
	"github.com/nnutter/moltisctl/internal/models"
	cr "github.com/nnutter/moltisctl/internal/runtime"
)

// Build a source with synthetic credentials, never a real user's database.
func shelleyFixtureSource(t *testing.T) models.ShelleySource {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "instances", "native", "home", ".local", "state", "shelley", "shelley.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"version":1,"instances":{"native":{"name":"native"}}}`), 0o600))
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	script, err := os.ReadFile(filepath.Join("..", "internal", "models", "testdata", "shelley.sql"))
	require.NoError(t, err)
	_, err = db.Exec(string(script))
	require.NoError(t, err)
	return models.ShelleySource{StateDir: dir, Name: "native"}
}

// Exercise the real application stop/import/restart path without installing
// launchers or configuring Tailscale. The existing native test checks health,
// authentication, and Moltis's config parser after this operation returns.
func importIntoRunningInstance(t *testing.T, app *instance.Application, adapter *cr.Adapter, stateDir string, spec cr.Spec) {
	t.Helper()
	statePath := filepath.Join(stateDir, "state.json")
	// Seed active intent directly: Create would modify host startup and Serve.
	stateData, err := json.Marshal(map[string]any{
		"version": 1,
		"instances": map[string]any{spec.Name: map[string]any{
			"name": spec.Name, "image": spec.Image, "port": spec.Port, "desired": true,
		}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(statePath, stateData, 0o600))
	before, err := adapter.Inspect(t.Context(), spec.Name)
	require.NoError(t, err)
	require.True(t, before.Running)
	require.NoError(t, app.ImportShelleyModels(t.Context(), spec.Name, shelleyFixtureSource(t)))
	after, err := adapter.Inspect(t.Context(), spec.Name)
	require.NoError(t, err)
	require.True(t, after.Running)
	require.Equal(t, before.ID, after.ID, "import must retain the container")
	unchanged, err := os.ReadFile(statePath)
	require.NoError(t, err)
	require.Equal(t, stateData, unchanged, "retain active intent, image, and port")
	data, err := os.ReadFile(filepath.Join(spec.Home, ".config", "moltis", "moltis.toml"))
	require.NoError(t, err)
	var doc struct{ Providers map[string]any }
	require.NoError(t, toml.Unmarshal(data, &doc))
	require.Len(t, doc.Providers, 4, "import three groups and preserve the original provider")
}
