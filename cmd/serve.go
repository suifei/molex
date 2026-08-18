package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/relay"
	"github.com/suifei/molex/internal/telemetry"
)

func newServeCommand() *cobra.Command {
	var configPath, listen string
	command := &cobra.Command{
		Use:   "serve",
		Short: "Run the public WebSocket relay",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := config.Config{Mode: config.ModeRelay, Listen: "127.0.0.1:8080"}
			loaded, err := config.Load(configPath)
			if err == nil {
				cfg = loaded
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if cfg.Mode != config.ModeRelay {
				return fmt.Errorf("%s is a %q configuration; `molex serve` requires mode \"relay\"", configPath, cfg.Mode)
			}
			if cmd.Flags().Changed("listen") {
				cfg.Listen = listen
			}
			cfg = cfg.Normalized()
			if err := cfg.Validate(); err != nil {
				return err
			}

			logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelInfo}))
			logReporter := telemetry.ReporterFunc(func(event telemetry.Event) {
				if event.Transient {
					return
				}
				logger.Info(event.Message, "state", event.State, "listen", event.Listen)
			})
			// Relay lifecycle events are audited to a durable JSONL file
			// beside the configuration.
			reporter := telemetry.MultiReporter(logReporter, telemetry.NewAuditWriter(telemetry.DefaultAuditPath(configPath)))
			credentials := make([]relay.Credential, 0, len(cfg.Tokens))
			for _, token := range cfg.Tokens {
				credentials = append(credentials, relay.Credential{
					ID:              token.ID,
					Token:           token.Token,
					Disabled:        token.Disabled,
					ExpiresAt:       token.ExpiresAt,
					Previous:        token.PreviousToken,
					PreviousExpires: token.PreviousExpiresAt,
				})
			}
			if len(credentials) == 0 {
				logger.Warn("No access tokens are configured; every client will be rejected. Create tokens in the relay web console or configuration file.")
			}
			server := relay.New(relay.Options{Listen: cfg.Listen, Tokens: credentials, Logger: logger, Reporter: reporter})
			return server.Run(cmd.Context())
		},
	}
	command.Flags().StringVarP(&configPath, "config", "c", "molex.json", "configuration file")
	command.Flags().StringVar(&listen, "listen", "", "relay listen address")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return fmt.Errorf("serve flags: %w", err) })
	return command
}
