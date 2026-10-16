package models

import (
	"errors"
	"slices"
)

// Import merges a batch without replacing a conflicting provider connection.
// Validation and conflict checks finish before the atomic TOML publication.
// The caller must prevent concurrent execution, including scheduled recovery.
func Import(home string, configs []Config) error {
	if len(configs) == 0 {
		return errors.New("no providers to import")
	}
	seen := map[string]bool{}
	for _, c := range configs {
		if err := c.Validate(); err != nil {
			return err
		}
		if seen[c.Provider] {
			return errors.New("duplicate provider names in import batch")
		}
		seen[c.Provider] = true
	}
	return configure(home, func(providers map[string]any) error {
		for _, c := range configs {
			if existing, ok := providers[c.Provider]; ok {
				p, ok := existing.(map[string]any)
				if !ok {
					return importConflict()
				}
				endpoint, ok := p["base_url"]
				if !ok {
					endpoint = p["url"]
				}
				wire, ok := p["wire_api"]
				if !ok {
					wire = "chat-completions"
				}
				wantWire := "chat-completions"
				if c.APIType == "openai-responses" {
					wantWire = "responses"
				}
				if endpoint != c.APIURL || p["api_key"] != c.APIKey || wire != wantWire {
					return importConflict()
				}
				if enabled, ok := p["enabled"]; ok && enabled != true {
					return importConflict()
				}
				var names []string
				if value, exists := p["models"]; exists {
					list, ok := value.([]any)
					if !ok {
						return importConflict()
					}
					for _, value := range list {
						name, ok := value.(string)
						if !ok {
							return importConflict()
						}
						names = append(names, name)
					}
				}
				for _, name := range c.Names {
					if !slices.Contains(names, name) {
						names = append(names, name)
					}
				}
				c.Names = names
			}
			if err := configureProvider(providers, c); err != nil {
				return err
			}
		}
		return nil
	})
}

func importConflict() error {
	return errors.New("an imported provider conflicts with existing Moltis settings; no providers were written")
}
