package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/suifei/molex/internal/client"
	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/telemetry"
)

func newConnectCommand() *cobra.Command {
	var configPath, remote, token, name string
	command := &cobra.Command{
		Use:   "connect",
		Short: "Connect a target or edge to the relay",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := config.Default()
			loaded, err := config.Load(configPath)
			if err == nil {
				cfg = loaded
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if cfg.Mode == config.ModeRelay {
				return fmt.Errorf("%s is a relay configuration; use `molex serve` for the relay and mode \"target\" or \"edge\" for clients", configPath)
			}
			applyStringFlag(cmd, "remote", &cfg.Remote, remote)
			applyStringFlag(cmd, "token", &cfg.Token, token)
			applyStringFlag(cmd, "name", &cfg.Name, name)
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
			return client.Run(cmd.Context(), cfg, reporter)
		},
	}
	command.Flags().StringVarP(&configPath, "config", "c", "molex.json", "configuration file")
	command.Flags().StringVar(&remote, "remote", "", "relay ws:// or wss:// endpoint")
	command.Flags().StringVar(&token, "token", "", "relay access token")
	command.Flags().StringVar(&name, "name", "", "client name shown in the Relay console")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return fmt.Errorf("connect flags: %w", err) })
	return command
}

func applyStringFlag(command *cobra.Command, name string, destination *string, value string) {
	if command.Flags().Changed(name) {
		*destination = value
	}
}
