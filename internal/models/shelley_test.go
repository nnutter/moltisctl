package models_test

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/moltisctl/internal/models"
)

func shelleyFixture(t *testing.T) (models.ShelleySource, *sql.DB, string) {
	t.Helper()
	return shelleyFixtureAt(t, filepath.Join(".local", "state", "shelley", "shelley.db"))
}

func shelleyFixtureAt(t *testing.T, layout string) (models.ShelleySource, *sql.DB, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "source ?# space")
	path := filepath.Join(dir, "instances", "alice", "home", layout)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"version":1,"instances":{"alice":{"name":"alice","image":"original","port":19000,"desired":true}}}`), 0o600))
	uri := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", uri.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0;")
	require.NoError(t, err)
	script, err := os.ReadFile("testdata/shelley.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(script))
	require.NoError(t, err)
	return models.ShelleySource{StateDir: dir, Name: "alice"}, db, path
}

func TestReadShelleyGroupsCommittedWALModelsWithoutChangingSource(t *testing.T) {
	for _, layout := range []string{filepath.Join(".local", "state", "shelley", "shelley.db"), "shelley.db"} {
		t.Run(layout, func(t *testing.T) {
			source, _, path := shelleyFixtureAt(t, layout)
			originals := map[string][]byte{}
			for _, file := range []string{path, path + "-wal", filepath.Join(source.StateDir, "state.json")} {
				data, err := os.ReadFile(file)
				require.NoError(t, err)
				require.NotEmpty(t, data)
				originals[file] = data
			}
			configs, err := models.ReadShelley(t.Context(), source)
			require.NoError(t, err)
			require.Len(t, configs, 3)
			for i, expected := range []struct {
				kind, key string
				names     []string
			}{
				{"openai", "chat-fixture-key", []string{"model-a", "model-b"}},
				{"openai-responses", "responses-fixture-key", []string{"model-r"}},
				{"openai", "other-fixture-key", []string{"model-c"}},
			} {
				require.Equal(t, "https://meta.example/v1", configs[i].APIURL)
				require.Equal(t, expected.kind, configs[i].APIType)
				require.Equal(t, expected.key, configs[i].APIKey)
				require.Equal(t, expected.names, configs[i].Names)
				require.Regexp(t, `^custom-shelleyctl-[0-9a-f]{16}$`, configs[i].Provider)
			}
			require.NotEqual(t, configs[0].Provider, configs[1].Provider)
			require.NotEqual(t, configs[0].Provider, configs[2].Provider)
			// Environment-based discovery must use the same state and stable names.
			t.Setenv("SHELLEYCTL_STATE_DIR", source.StateDir)
			source.StateDir = ""
			repeated, err := models.ReadShelley(t.Context(), source)
			require.NoError(t, err)
			require.Equal(t, configs, repeated)
			for file, want := range originals {
				got, err := os.ReadFile(file)
				require.NoError(t, err)
				require.Equal(t, want, got, "source file changed: %s", file)
			}
		})
	}
}

func TestReadShelleyRejectsUnconvertibleSourcesWithoutSecrets(t *testing.T) {
	for _, scenario := range []string{"unsupported", "empty key", "credential URL", "empty database", "wrong schema", "missing database", "unknown instance", "invalid state", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			source, db, path := shelleyFixture(t)
			ctx := t.Context()
			var statement string
			switch scenario {
			case "unsupported":
				statement = `UPDATE models SET provider_type='anthropic' WHERE model_id='e'`
			case "empty key":
				statement = `UPDATE models SET api_key='' WHERE model_id='e'`
			case "credential URL":
				statement = `UPDATE models SET endpoint='https://user:do-not-print@meta.example/v1' WHERE model_id='e'`
			case "empty database":
				statement = `DELETE FROM models`
			case "wrong schema":
				statement = `DROP TABLE models`
			case "missing database":
				require.NoError(t, os.Rename(path, path+".retained"))
			case "unknown instance":
				source.Name = "missing"
			case "invalid state":
				require.NoError(t, os.WriteFile(filepath.Join(source.StateDir, "state.json"), []byte(`{"version":2}`), 0o600))
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if statement != "" {
				_, err := db.Exec(statement)
				require.NoError(t, err)
			}
			configs, err := models.ReadShelley(ctx, source)
			require.Error(t, err)
			if scenario == "unsupported" || scenario == "empty key" || scenario == "credential URL" {
				require.ErrorContains(t, err, "unsupported or incomplete model")
			}
			require.Nil(t, configs)
			for _, secret := range []string{"do-not-print", "chat-fixture-key", "responses-fixture-key"} {
				require.NotContains(t, err.Error(), secret)
			}
			if scenario == "cancelled" {
				require.ErrorIs(t, err, context.Canceled)
			}
			if scenario == "missing database" {
				require.NoFileExists(t, path)
			}
		})
	}
}
