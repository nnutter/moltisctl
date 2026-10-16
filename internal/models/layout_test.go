package models_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/moltisctl/internal/models"
)

func TestReadShelleyRefusesAmbiguousOrLinkedLayouts(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "two databases", true: "linked database"}[linked], func(t *testing.T) {
			source, _, current := shelleyFixture(t)
			legacy := filepath.Join(source.StateDir, "instances", source.Name, "home", "shelley.db")
			if linked {
				require.NoError(t, os.Symlink(current, legacy))
			} else {
				require.NoError(t, os.WriteFile(legacy, []byte("a second database must not be guessed"), 0o600))
			}
			configs, err := models.ReadShelley(t.Context(), source)
			require.Error(t, err)
			require.Nil(t, configs)
			if linked {
				require.ErrorContains(t, err, "symbolic link")
			} else {
				require.ErrorContains(t, err, "both Shelley database layouts")
			}
		})
	}
}
