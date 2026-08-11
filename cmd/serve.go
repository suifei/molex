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
	var configPath, listen, token string
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
			cfg.Mode = config.ModeRelay
			if cmd.Flags().Changed("listen") {
				cfg.Listen = listen
			}
			if cmd.Flags().Changed("token") {
				cfg.Token = token
			}
			cfg = cfg.Normalized()
			if err := cfg.Validate(); err != nil {
				return err
			}

			logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelInfo}))
			reporter := telemetry.ReporterFunc(func(event telemetry.Event) {
				if event.Transient {
					return
				}
				logger.Info(event.Message, "state", event.State, "listen", event.Listen)
			})
			server := relay.New(relay.Options{Listen: cfg.Listen, Token: cfg.Token, Logger: logger, Reporter: reporter})
			return server.Run(cmd.Context())
		},
	}
	command.Flags().StringVarP(&configPath, "config", "c", "molex.json", "configuration file")
	command.Flags().StringVar(&listen, "listen", "", "relay listen address")
	command.Flags().StringVar(&token, "token", "", "optional relay admission token")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return fmt.Errorf("serve flags: %w", err) })
	return command
}
