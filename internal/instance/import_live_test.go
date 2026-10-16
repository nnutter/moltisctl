package instance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/moltisctl/internal/models"
	cr "github.com/nnutter/moltisctl/internal/runtime"
)

type importRuntime struct {
	*runtimeStub
	status     cr.Status
	transition func(context.Context, string) error
}

func (r *importRuntime) Inspect(context.Context, string) (cr.Status, error) {
	return r.status, r.action("inspect container")
}
func (r *importRuntime) Stop(ctx context.Context, _ string) error {
	return r.transition(ctx, "stop container")
}
func (r *importRuntime) Start(ctx context.Context, _ string) error {
	return r.transition(ctx, "start container")
}

func TestImportRestoresRunningInstancesOnSuccessFailureAndCancellation(t *testing.T) {
	for _, scenario := range []string{"running", "stopped", "stop failed", "cancelled", "conflict", "restart failed", "conflict and restart failed"} {
		t.Run(scenario, func(t *testing.T) {
			source, _ := importSource(t)
			port := backend(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
			var actions []string
			a := testApplication(t, func(action string) error { actions = append(actions, action); return nil })
			seed(t, a, port, true)
			home := filepath.Join(a.opts.StateDir, "instances", "demo", "home")
			path := filepath.Join(home, ".config", "moltis", "moltis.toml")
			require.NoError(t, os.MkdirAll(filepath.Dir(home), 0o700))
			configs, err := models.ReadShelley(t.Context(), source)
			require.NoError(t, err)
			conflict := scenario == "conflict" || scenario == "conflict and restart failed"
			if conflict {
				c := configs[2]
				c.APIKey = "destination-secret"
				require.NoError(t, models.Configure(home, c))
			} else {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				require.NoError(t, os.WriteFile(path, []byte("# original config\n"), 0o600))
			}
			configBefore, err := os.ReadFile(path)
			require.NoError(t, err)
			stateBefore, err := os.ReadFile(filepath.Join(a.opts.StateDir, "state.json"))
			require.NoError(t, err)
			stopFailure, restartFailure := errors.New("stop failed"), errors.New("restart failed")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			a.containers = &importRuntime{runtimeStub: a.containers.(*runtimeStub), status: cr.Status{Exists: true, Running: scenario != "stopped", ID: "original"}, transition: func(cleanup context.Context, action string) error {
				actions = append(actions, action)
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				if action == "stop container" {
					require.Equal(t, configBefore, data, "settings must not change before stop")
					if scenario == "cancelled" {
						cancel()
					}
					if scenario == "stop failed" {
						return stopFailure
					}
					return nil
				}
				require.NoError(t, cleanup.Err(), "restoration must not inherit cancellation")
				if conflict || scenario == "stop failed" || scenario == "cancelled" {
					require.Equal(t, configBefore, data)
				} else {
					var doc struct{ Providers map[string]any }
					require.NoError(t, toml.Unmarshal(data, &doc))
					require.Len(t, doc.Providers, 3, "publish the entire batch before restart")
				}
				if scenario == "running" {
					// A scheduled reconciler must still be blocked during restart.
					wait, done := context.WithTimeout(t.Context(), 40*time.Millisecond)
					defer done()
					require.ErrorIs(t, a.Status(wait, io.Discard), context.DeadlineExceeded)
				}
				if scenario == "restart failed" || scenario == "conflict and restart failed" {
					return restartFailure
				}
				return nil
			}}
			err = a.ImportShelleyModels(ctx, "demo", source)
			switch scenario {
			case "running", "stopped":
				require.NoError(t, err)
			case "stop failed":
				require.ErrorIs(t, err, stopFailure)
			case "cancelled":
				require.ErrorIs(t, err, context.Canceled)
			case "conflict":
				require.ErrorContains(t, err, "conflicts")
			case "restart failed":
				require.ErrorIs(t, err, restartFailure)
			case "conflict and restart failed":
				require.ErrorContains(t, err, "conflicts")
				require.ErrorIs(t, err, restartFailure)
			}
			if scenario == "stopped" {
				require.Equal(t, []string{"check container", "inspect container"}, actions)
			} else {
				require.Equal(t, []string{"check container", "inspect container", "stop container", "check container", "start container"}, actions)
			}
			stateAfter, err := os.ReadFile(filepath.Join(a.opts.StateDir, "state.json"))
			require.NoError(t, err)
			require.Equal(t, stateBefore, stateAfter, "do not withdraw or replace lifecycle intent")
		})
	}
}

func TestImportRefusesPendingUpdatesAndForeignContainersBeforeStopping(t *testing.T) {
	for _, scenario := range []string{"pending update", "foreign container"} {
		t.Run(scenario, func(t *testing.T) {
			source, _ := importSource(t)
			var actions []string
			a := testApplication(t, func(action string) error {
				actions = append(actions, action)
				return errors.New("foreign container")
			})
			r := record{Name: "demo", Image: "original-image", Port: 19123, Desired: true}
			if scenario == "pending update" {
				r.PendingImage, r.Replacing = "replacement-image", true
			}
			require.NoError(t, a.save(state{Version: 1, Instances: map[string]record{"demo": r}}))
			err := a.ImportShelleyModels(t.Context(), "demo", source)
			require.Error(t, err)
			if scenario == "pending update" {
				require.ErrorContains(t, err, "pending instance update")
				require.Empty(t, actions)
			} else {
				require.ErrorContains(t, err, "foreign container")
				require.Equal(t, []string{"check container"}, actions)
			}
			require.NoDirExists(t, filepath.Join(a.opts.StateDir, "instances", "demo", "home"))
		})
	}
}
