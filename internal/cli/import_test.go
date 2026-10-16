package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImportShelleyCommandSelectsSourceWithoutCredentials(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "explicit"}[custom], func(t *testing.T) {
			app := &recordingApp{}
			root := New(app)
			args := []string{"models", "import-shelleyctl", "alice"}
			if custom {
				args = append(args, "--from", "christy", "--shelley-state-dir", "/source/state")
			}
			root.SetArgs(args)
			require.NoError(t, root.ExecuteContext(t.Context()))
			require.Equal(t, []string{"models import-shelleyctl alice"}, app.calls)
			if custom {
				require.Equal(t, "christy", app.shelleySource.Name)
				require.Equal(t, "/source/state", app.shelleySource.StateDir)
			} else {
				require.Equal(t, "alice", app.shelleySource.Name)
				require.Empty(t, app.shelleySource.StateDir)
			}
		})
	}
}

func TestImportShelleyArgumentsAndHelpDoNotDispatch(t *testing.T) {
	for _, args := range [][]string{{}, {"alice", "extra"}, {"alice", "--api-key-file", "-"}, {"--help"}} {
		app := &recordingApp{}
		root := New(app)
		root.SetArgs(append([]string{"models", "import-shelleyctl"}, args...))
		err := root.ExecuteContext(t.Context())
		if len(args) == 1 && args[0] == "--help" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
		require.Empty(t, app.calls)
	}
}
