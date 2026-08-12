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
	var configPath, remote, secret, token, role, listen, local, channel, name string
	var pool int
	command := &cobra.Command{
		Use:   "connect",
		Short: "Connect an edge or target to the relay",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := config.Default()
			loaded, err := config.Load(configPath)
			if err == nil {
				cfg = loaded
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			cfg.Mode = config.ModePunch
			applyStringFlag(cmd, "remote", &cfg.Remote, remote)
			applyStringFlag(cmd, "secret", &cfg.Secret, secret)
			applyStringFlag(cmd, "token", &cfg.Token, token)
			applyStringFlag(cmd, "role", &cfg.Role, role)
			applyStringFlag(cmd, "listen", &cfg.Listen, listen)
			applyStringFlag(cmd, "local", &cfg.Tunnel.Local, local)
			applyStringFlag(cmd, "channel", &cfg.Tunnel.Remote, channel)
			applyStringFlag(cmd, "name", &cfg.Tunnel.Name, name)
			if cmd.Flags().Changed("pool") {
				cfg.Tunnel.Pool = pool
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
			return client.Run(cmd.Context(), cfg, reporter)
		},
	}
	command.Flags().StringVarP(&configPath, "config", "c", "molex.json", "configuration file")
	command.Flags().StringVar(&remote, "remote", "", "relay ws:// or wss:// endpoint")
	command.Flags().StringVar(&secret, "secret", "", "end-to-end shared secret")
	command.Flags().StringVar(&token, "token", "", "optional relay admission token")
	command.Flags().StringVar(&role, "role", "", "client role: edge or target")
	command.Flags().StringVar(&listen, "listen", "", "local edge listen address")
	command.Flags().StringVar(&local, "local", "", "target service address")
	command.Flags().StringVar(&channel, "channel", "", "shared rendezvous channel")
	command.Flags().StringVar(&name, "name", "", "client name shown in the Relay console")
	command.Flags().IntVar(&pool, "pool", 0, "target session pool size (1-64)")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return fmt.Errorf("connect flags: %w", err) })
	return command
}

func applyStringFlag(command *cobra.Command, name string, destination *string, value string) {
	if command.Flags().Changed(name) {
		*destination = value
	}
}
