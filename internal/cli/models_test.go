package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelCredentialsFromFileOrStdin(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "provider-key")
	require.NoError(t, os.WriteFile(keyPath, []byte("fixture-secret\n"), 0o600))
	for _, source := range []string{keyPath, "-"} {
		t.Run(source, func(t *testing.T) {
			app := &recordingApp{}
			root := New(app)
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetIn(strings.NewReader("fixture-secret\n"))
			root.SetArgs([]string{"models", "configure", "alice", "--api-url", "https://api.example/v1", "--api-key-file", source, "--model", "one", "--model", "two"})
			require.NoError(t, root.ExecuteContext(t.Context()))
			require.Equal(t, []string{"models configure alice"}, app.calls)
			require.Equal(t, "fixture-secret", app.modelConfig.APIKey)
			require.Equal(t, "custom-moltisctl", app.modelConfig.Provider)
			require.Equal(t, "openai-responses", app.modelConfig.APIType)
			require.Equal(t, []string{"one", "two"}, app.modelConfig.Names)
			require.NotContains(t, output.String(), "fixture-secret")
		})
	}
}

func TestInvalidProviderInputDoesNotDispatch(t *testing.T) {
	for _, scenario := range []string{"missing endpoint", "credential URL", "missing key", "missing model", "invalid format", "invalid provider", "empty", "multiline", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			args := []string{"models", "configure", "alice", "--api-url", "https://api.example/v1", "--api-key-file", "-", "--model", "one"}
			key := "fixture-secret\n"
			switch scenario {
			case "missing endpoint":
				args[4] = ""
			case "credential URL":
				args[4] = "https://user:fixture-secret@api.example/v1"
			case "missing key":
				args[6] = ""
			case "missing model":
				args = args[:7]
			case "invalid format":
				args = append(args, "--api-type", "anthropic")
			case "invalid provider":
				args = append(args, "--provider", "openai")
			case "empty":
				key = "\n"
			case "multiline":
				key = "fixture-secret\nsecond-line"
			case "oversized":
				key = strings.Repeat("x", (64<<10)+1)
			}
			app := &recordingApp{}
			root := New(app)
			root.SetIn(strings.NewReader(key))
			root.SetArgs(args)
			err := root.ExecuteContext(t.Context())
			require.Error(t, err)
			require.Empty(t, app.calls)
			require.NotContains(t, err.Error(), "fixture-secret")
		})
	}
}
