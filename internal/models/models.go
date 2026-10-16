// Package models writes custom providers to an offline Moltis home.
package models

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config supplies a custom OpenAI-compatible provider. Keys must not appear in
// logs, lifecycle records, or launcher arguments.
type Config struct {
	Provider, APIURL, APIType, APIKey string
	Names                             []string
}

var providerName = regexp.MustCompile(`^custom-[a-z][a-z0-9-]{0,39}$`)

// Validate rejects incomplete input before any instance side effects.
func (c Config) Validate() error {
	if !providerName.MatchString(c.Provider) {
		return errors.New("provider must start with custom- followed by 1–40 lowercase letters, digits, or hyphens, starting with a letter")
	}
	u, err := url.Parse(c.APIURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("API URL must be an absolute HTTP or HTTPS URL without credentials, query, or fragment")
	}
	if c.APIType != "openai" && c.APIType != "openai-responses" {
		return errors.New("API type must be openai or openai-responses; configure other providers in Moltis")
	}
	if strings.TrimSpace(c.APIKey) == "" || strings.ContainsAny(c.APIKey, "\r\n\x00") {
		return errors.New("API key must be a nonempty single line")
	}
	if len(c.Names) == 0 {
		return errors.New("at least one model name is required")
	}
	for _, name := range c.Names {
		if name == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, "\r\n\x00") {
			return errors.New("model names must be nonempty without surrounding whitespace or line breaks")
		}
	}
	return nil
}

// Configure preserves unrelated TOML values, but not comments or formatting.
// The caller must prevent concurrent execution and disable scheduled recovery.
func Configure(home string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	return configure(home, func(providers map[string]any) error {
		return configureProvider(providers, c)
	})
}

func configure(home string, update func(map[string]any) error) error {
	dir := home
	for _, part := range []string{"", ".config", "moltis"} {
		dir = filepath.Join(dir, part)
		if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return errors.New("cannot create Moltis configuration directory")
		}
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() {
			return errors.New("configuration directories must not be symbolic links")
		}
	}
	path := filepath.Join(dir, "moltis.toml")
	doc, err := readConfig(path)
	if err != nil {
		return err
	}
	providers, err := table(doc, "providers")
	if err != nil {
		return err
	}
	if err := update(providers); err != nil {
		return err
	}
	data, err := toml.Marshal(doc)
	if err != nil {
		return errors.New("cannot encode Moltis configuration")
	}
	return publish(dir, path, data)
}

func configureProvider(providers map[string]any, c Config) error {
	provider, err := table(providers, c.Provider)
	if err != nil {
		return err
	}
	delete(provider, "url") // Moltis treats url as an alias of base_url.
	provider["enabled"], provider["api_key"], provider["base_url"] = true, c.APIKey, c.APIURL
	provider["models"] = c.Names
	if _, exists := provider["fetch_models"]; !exists {
		provider["fetch_models"] = false
	}
	provider["wire_api"] = "chat-completions"
	if c.APIType == "openai-responses" {
		provider["wire_api"] = "responses"
	}
	// Moltis's optional allowlist must not hide the configured provider.
	if offered, ok := providers["offered"].([]any); ok && len(offered) > 0 {
		found := false
		for _, name := range offered {
			if name == c.Provider {
				found = true
			}
		}
		if !found {
			providers["offered"] = append(offered, c.Provider)
		}
	}
	return nil
}

func table(doc map[string]any, name string) (map[string]any, error) {
	if value, exists := doc[name]; exists {
		if t, ok := value.(map[string]any); ok {
			return t, nil
		}
		return nil, errors.New("provider configuration must contain TOML tables")
	}
	t := map[string]any{}
	doc[name] = t
	return t, nil
}

func readConfig(path string) (map[string]any, error) {
	doc := map[string]any{}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		doc["tools"] = map[string]any{"exec": map[string]any{"sandbox": map[string]any{"backend": "restricted-host"}}}
		return doc, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("configuration must be a regular file, not a symbolic link")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("cannot read Moltis configuration")
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		// TOML diagnostics can contain secret values from the source document.
		return nil, errors.New("cannot parse Moltis configuration; repair it before configuring models")
	}
	return doc, nil
}

func publish(dir, path string, data []byte) (result error) {
	file, err := os.CreateTemp(dir, ".moltisctl-*")
	if err != nil {
		return errors.New("cannot create temporary Moltis configuration")
	}
	defer func() {
		if err := os.Remove(file.Name()); !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
