package instance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/moltisctl/internal/models"
	cr "github.com/nnutter/moltisctl/internal/runtime"
)

type existingModelRuntime struct{ *runtimeStub }

func (r *existingModelRuntime) Inspect(ctx context.Context, name string) (cr.Status, error) {
	return cr.Status{Exists: true}, nil
}

func TestConfigureModelsRequiresOfflineUnownedResourcesToBeAbsent(t *testing.T) {
	for _, scenario := range []string{"new", "retained", "active", "leftover container", "foreign container", "query failed"} {
		t.Run(scenario, func(t *testing.T) {
			var actions []string
			a := testApplication(t, func(action string) error {
				actions = append(actions, action)
				if scenario == "foreign container" && action == "check container" {
					return errors.New("foreign container")
				}
				if scenario == "query failed" && action == "inspect container" {
					return errors.New("runtime unavailable")
				}
				return nil
			})
			if scenario != "new" {
				seed(t, a, 19123, scenario == "active")
			}
			if scenario == "leftover container" {
				a.containers = &existingModelRuntime{a.containers.(*runtimeStub)}
			}
			config := models.Config{Provider: "custom-test", APIURL: "https://provider.example/v1", APIType: "openai", APIKey: "fixture-secret", Names: []string{"one"}}
			err := a.ConfigureModels(t.Context(), "demo", config)
			path := filepath.Join(a.opts.StateDir, "instances", "demo", "home", ".config", "moltis", "moltis.toml")
			if scenario == "new" || scenario == "retained" {
				require.NoError(t, err)
				r := diskRecord(t, a)
				require.False(t, r.Desired)
				if scenario == "retained" {
					require.Equal(t, 19123, r.Port)
				}
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Contains(t, string(data), "fixture-secret")
				state, err := os.ReadFile(filepath.Join(a.opts.StateDir, "state.json"))
				require.NoError(t, err)
				require.NotContains(t, string(state), "fixture-secret")
				// Scheduled recovery must not start a preconfigured instance.
				t.Setenv("MOLTISCTL_RESUME", "1")
				before := len(actions)
				require.NoError(t, a.Create(t.Context(), "demo"))
				require.Len(t, actions, before)
			} else {
				require.Error(t, err)
				require.NoFileExists(t, path)
			}
			for _, action := range actions {
				require.Contains(t, []string{"check container", "inspect container"}, action)
			}
		})
	}
}
