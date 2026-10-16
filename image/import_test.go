package image_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/moltisctl/internal/models"
)

// Add imported providers to the home exercised by TestNativeEmbeddedContext.
// Its real Moltis process and config check validate the conversion contract.
func importShelleyFixture(t *testing.T, home string) {
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
	configs, err := models.ReadShelley(t.Context(), models.ShelleySource{StateDir: dir, Name: "native"})
	require.NoError(t, err)
	require.Len(t, configs, 3)
	require.NoError(t, models.Import(home, configs))
}
