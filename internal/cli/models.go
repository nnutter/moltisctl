package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nnutter/moltisctl/internal/models"
)

type modelOptions struct {
	provider, url, apiType, keyFile string
	names                           []string
}

func (o *modelOptions) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.provider, "provider", "custom-moltisctl", "Custom Moltis provider name (custom- prefix)")
	cmd.Flags().StringVar(&o.url, "api-url", "", "Provider endpoint (OpenAI: base URL including /v1)")
	cmd.Flags().StringVar(&o.apiType, "api-type", "openai-responses", "API format: openai-responses or openai (Chat Completions)")
	cmd.Flags().StringVar(&o.keyFile, "api-key-file", "", "Read the API key from a file, or - for stdin")
	cmd.Flags().StringArrayVar(&o.names, "model", nil, "Provider model name (repeat for multiple models)")
}

func (o *modelOptions) read(input io.Reader) (models.Config, error) {
	c := models.Config{Provider: o.provider, APIURL: o.url, APIType: o.apiType, Names: o.names}
	if o.keyFile == "" {
		return c, errors.New("--api-key-file is required with model configuration")
	}
	if o.keyFile == "-" {
		key, err := readKey(input)
		if err != nil {
			return c, err
		}
		c.APIKey = key
	} else {
		key, err := readKeyFile(o.keyFile)
		if err != nil {
			return c, err
		}
		c.APIKey = key
	}
	return c, c.Validate()
}

func readKeyFile(path string) (key string, result error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("cannot open API key file")
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	return readKey(file)
}

func readKey(input io.Reader) (string, error) {
	const limit = 64 << 10
	data, err := io.ReadAll(io.LimitReader(input, limit+1))
	if err != nil {
		return "", errors.New("cannot read API key")
	}
	if len(data) > limit {
		return "", errors.New("API key input exceeds 64 KiB")
	}
	return strings.TrimSpace(string(data)), nil
}

type commandRunner func(*cobra.Command, func(context.Context) error) error

func modelsCommand(app Application, run commandRunner) *cobra.Command {
	parent := &cobra.Command{Use: "models", Short: "Configure instance models"}
	var options modelOptions
	cmd := &cobra.Command{
		Use: "configure NAME", Short: "Configure a custom provider on a new or deleted instance", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := options.read(cmd.InOrStdin())
			if err != nil {
				return err
			}
			return run(cmd, func(ctx context.Context) error { return app.ConfigureModels(ctx, args[0], config) })
		},
	}
	options.bind(cmd)
	var source models.ShelleySource
	importer := &cobra.Command{
		Use:   "import-shelleyctl NAME",
		Short: "Import saved Shelley models into a new or deleted Moltis instance",
		Long:  "Import OpenAI-compatible custom models and API keys from a shelleyctl instance. The source is read-only. The destination must be new or deleted. Conversations, the default model, and Shelley-specific tuning are not imported.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selected := source
			if selected.Name == "" {
				selected.Name = args[0]
			}
			return run(cmd, func(ctx context.Context) error {
				return app.ImportShelleyModels(ctx, args[0], selected)
			})
		},
	}
	importer.Flags().StringVar(&source.Name, "from", "", "Source Shelley instance name (defaults to NAME)")
	importer.Flags().StringVar(&source.StateDir, "shelley-state-dir", "", "Absolute shelleyctl state directory (defaults to SHELLEYCTL_STATE_DIR or the platform default)")
	parent.AddCommand(cmd, importer)
	return parent
}
