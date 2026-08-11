package cmd

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/suifei/molex/internal/webui"
)

const webPasswordEnvironment = "MOLEX_WEB_PASSWORD"

func newWebCommand() *cobra.Command {
	var configPath, listen, passwordFile string
	var autoStart bool
	command := &cobra.Command{
		Use:   "web",
		Short: "Run the authenticated browser management console",
		RunE: func(cmd *cobra.Command, _ []string) error {
			password, err := loadWebPassword(passwordFile)
			if err != nil {
				return err
			}
			logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelInfo}))
			server, err := webui.New(webui.Options{
				Listen:     listen,
				ConfigPath: configPath,
				Password:   password,
				AutoStart:  autoStart,
				Logger:     logger,
			})
			if err != nil {
				return err
			}
			return server.Run(cmd.Context())
		},
	}
	command.Flags().StringVarP(&configPath, "config", "c", "molex.json", "configuration file managed by the web console")
	command.Flags().StringVar(&listen, "listen", "127.0.0.1:9090", "loopback address for the web console")
	command.Flags().StringVar(&passwordFile, "password-file", "", "file containing the web login password")
	command.Flags().BoolVar(&autoStart, "autostart", false, "start the configured relay or client with the web console")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return fmt.Errorf("web flags: %w", err) })
	return command
}

func loadWebPassword(path string) (string, error) {
	if path == "" {
		password := strings.TrimRight(os.Getenv(webPasswordEnvironment), "\r\n")
		if password == "" {
			return "", fmt.Errorf("web login password is required; set %s or use --password-file", webPasswordEnvironment)
		}
		return password, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open web password file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 2049))
	if err != nil {
		return "", fmt.Errorf("read web password file: %w", err)
	}
	if len(data) > 2048 {
		return "", errors.New("web password file is too large")
	}
	password := strings.TrimRight(string(data), "\r\n")
	if password == "" {
		return "", errors.New("web password file is empty")
	}
	return password, nil
}
