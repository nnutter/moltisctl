package models

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"

	_ "modernc.org/sqlite" // Read SQLite without a host sqlite3 executable or CGO.
)

// ShelleySource identifies an instance in a shelleyctl state directory.
// An empty StateDir uses SHELLEYCTL_STATE_DIR or shelleyctl's platform default.
type ShelleySource struct{ StateDir, Name string }

var shelleyName = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,38}[a-z0-9])?$`)

// ReadShelley reads one SQLite snapshot, including committed WAL data. It does
// not invoke Shelley, migrate its database, or contact a model provider.
func ReadShelley(ctx context.Context, source ShelleySource) ([]Config, error) {
	path, err := source.databasePath()
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	query := url.Values{"mode": {"ro"}, "_pragma": {"query_only(1)", "busy_timeout(5000)"}}
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, errors.New("cannot open Shelley database read-only")
	}
	defer func() { _ = db.Close() }()
	// A single SELECT has a consistent SQLite read transaction, even while
	// Shelley writes. Do not use immutable=1: that can ignore committed WAL data.
	rows, err := db.QueryContext(ctx, `SELECT model_id, provider_type, endpoint, api_key, model_name FROM models ORDER BY model_id LIMIT 10001`)
	if err != nil {
		return nil, sourceQueryError(ctx)
	}
	defer func() { _ = rows.Close() }()
	type connection struct{ apiType, endpoint, key string }
	groups := map[connection]int{}
	var configs []Config
	count := 0
	for rows.Next() {
		count++
		if count > 10000 {
			return nil, errors.New("source has more than 10000 custom models; import refused")
		}
		var id, name string
		var key connection
		if err := rows.Scan(&id, &key.apiType, &key.endpoint, &key.key, &name); err != nil {
			return nil, errors.New("cannot read Shelley model fields; import refused")
		}
		index, exists := groups[key]
		if !exists {
			// Names depend on source identity, not on API-key hashes. Never expose
			// a secret-derived fingerprint in the provider name or diagnostics.
			hash := sha256.Sum256([]byte(source.Name + "\x00" + id))
			index = len(configs)
			configs = append(configs, Config{Provider: "custom-shelleyctl-" + hex.EncodeToString(hash[:8]), APIURL: key.endpoint, APIType: key.apiType, APIKey: key.key})
			groups[key] = index
		}
		if !slices.Contains(configs[index].Names, name) {
			configs[index].Names = append(configs[index].Names, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, sourceQueryError(ctx)
	}
	if len(configs) == 0 {
		return nil, errors.New("source Shelley instance has no saved custom models")
	}
	for _, config := range configs {
		if err := config.Validate(); err != nil {
			// Do not include database values or SQL diagnostics in the error.
			return nil, errors.New("source contains an unsupported or incomplete model; import requires OpenAI Chat Completions or Responses with explicit endpoints and keys")
		}
	}
	return configs, nil
}

func sourceQueryError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("cannot read Shelley models; check database permissions, schema, and WAL availability")
}

func (source ShelleySource) databasePath() (string, error) {
	if !shelleyName.MatchString(source.Name) {
		return "", errors.New("invalid Shelley instance name")
	}
	dir := source.StateDir
	if dir == "" {
		dir = os.Getenv("SHELLEYCTL_STATE_DIR")
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base := os.Getenv("XDG_STATE_HOME")
		if base == "" {
			base = filepath.Join(home, ".local", "state")
		}
		if runtime.GOOS == "darwin" {
			base = filepath.Join(home, "Library", "Application Support")
		}
		dir = filepath.Join(base, "shelleyctl")
	}
	if !filepath.IsAbs(dir) {
		return "", errors.New("source Shelley state directory must be absolute")
	}
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return "", errors.New("cannot read shelleyctl state; select its directory with --shelley-state-dir")
	}
	var state struct {
		Version   int `json:"version"`
		Instances map[string]struct {
			Name string `json:"name"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(data, &state); err != nil || state.Version != 1 {
		return "", errors.New("unsupported or invalid shelleyctl state")
	}
	if record, ok := state.Instances[source.Name]; !ok || record.Name != source.Name {
		return "", errors.New("source Shelley instance is not recorded in the selected state directory")
	}
	home := filepath.Join(dir, "instances", source.Name, "home")
	var selected string
	for _, layout := range []string{filepath.Join(".local", "state", "shelley", "shelley.db"), "shelley.db"} {
		path := filepath.Join(home, layout)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return "", errors.New("source Shelley database must be a readable regular file, not a symbolic link")
		}
		if selected != "" {
			return "", errors.New("both Shelley database layouts exist. Identify the authoritative database before importing")
		}
		selected = path
	}
	if selected == "" {
		return "", errors.New("no Shelley database found at home/.local/state/shelley/shelley.db or home/shelley.db. Check --from and --shelley-state-dir")
	}
	return selected, nil
}
