package image_test

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/moltisctl/image"
	cr "github.com/nnutter/moltisctl/internal/runtime"
)

// TestNativeEmbeddedContext builds the distributed context and replaces a real
// container, without changing host startup files or Tailscale configuration.
func TestNativeEmbeddedContext(t *testing.T) {
	if os.Getenv("MOLTISCTL_TEST_NATIVE") != "1" {
		t.Skip("set MOLTISCTL_TEST_NATIVE=1 to build and exercise the native image")
	}
	adapter, err := cr.New(runtime.GOOS)
	require.NoError(t, err)
	dir, err := image.Materialize()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	id := fmt.Sprintf("native-%d", os.Getpid())
	tag := "localhost/moltisctl:" + id
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Cleanup must still run after the test context is cancelled.
		require.NoError(t, adapter.Remove(ctx, id))
		output, err := exec.CommandContext(ctx, adapter.Binary(), "image", "rm", tag).CombinedOutput()
		require.NoError(t, err, "%s", output)
	})
	require.NoError(t, adapter.Build(t.Context(), tag, dir))
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	home := t.TempDir()
	spec := cr.Spec{Name: id, Image: tag, Home: home, Port: port}
	for generation := range 2 {
		require.NoError(t, adapter.Ensure(t.Context(), spec))
		require.NoError(t, adapter.Start(t.Context(), id))
		waitHealthy(t, port)
		output, err := exec.CommandContext(t.Context(), adapter.Binary(), "exec", "moltisctl-"+id,
			"/usr/local/bin/moltis-entrypoint", "sh", "-ec", `
			test "$(id -u)" != 0
			getent passwd "$(id -u)" >/dev/null
			test ! -S /var/run/docker.sock
			! command -v sudo
			moltis --version
			moltis config check
		`).CombinedOutput()
		require.NoError(t, err, "%s", output)
		config := filepath.Join(home, ".config", "moltis", "moltis.toml")
		data, err := os.ReadFile(config)
		require.NoError(t, err)
		require.Contains(t, string(data), `backend = "restricted-host"`)
		if generation == 0 {
			// User-owned config and data must survive container replacement.
			require.NoError(t, os.WriteFile(filepath.Join(home, ".moltis", "retention-check"), []byte("retained"), 0o600))
			require.NoError(t, os.WriteFile(config, append(data, []byte("\n# user configuration\n")...), 0o600))
		} else {
			require.Contains(t, string(data), "# user configuration")
			retained, err := os.ReadFile(filepath.Join(home, ".moltis", "retention-check"))
			require.NoError(t, err)
			require.Equal(t, "retained", string(retained))
		}
		require.NoError(t, adapter.Remove(t.Context(), id))
	}
}

func waitHealthy(t *testing.T, port int) {
	t.Helper()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	url := "http://127.0.0.1:" + strconv.Itoa(port)
	require.Eventually(t, func() bool {
		resp, err := client.Get(url + "/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 60*time.Second, 250*time.Millisecond)
	resp, err := client.Get(url + "/api/auth/status")
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	// Proxy connections must not receive Moltis's local unauthenticated access.
	var status struct {
		SetupRequired bool `json:"setup_required"`
		Authenticated bool `json:"authenticated"`
		AuthDisabled  bool `json:"auth_disabled"`
	}
	require.NoError(t, json.Unmarshal(data, &status))
	require.True(t, status.SetupRequired)
	require.False(t, status.Authenticated)
	require.False(t, status.AuthDisabled)
}
