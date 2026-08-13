package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/suifei/molex/internal/webui"
)

const webPasswordEnvironment = "MOLEX_WEB_PASSWORD"

func newWebCommand() *cobra.Command {
	var configPath, listen, passwordFile string
	var autoStart, openBrowser bool
	command := &cobra.Command{
		Use:   "web",
		Short: "Run the authenticated browser management console",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWeb(cmd.Context(), webRunOptions{
				configPath:   configPath,
				passwordFile: passwordFile,
				listen:       listen,
				autoListen:   !cmd.Flags().Changed("listen"),
				autoStart:    autoStart,
				openBrowser:  openBrowser,
				loggerOutput: cmd.ErrOrStderr(),
			})
		},
	}
	command.Flags().StringVarP(&configPath, "config", "c", "molex.json", "configuration file managed by the web console")
	command.Flags().StringVar(&listen, "listen", "127.0.0.1:9090", "loopback address for the web console (automatically advances if the default port is busy)")
	command.Flags().StringVar(&passwordFile, "password-file", "", "file containing the web login password")
	command.Flags().BoolVar(&autoStart, "autostart", false, "start the configured relay or client with the web console")
	command.Flags().BoolVar(&openBrowser, "open-browser", true, "open the web console in the default browser after it starts")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return fmt.Errorf("web flags: %w", err) })
	return command
}

type webRunOptions struct {
	configPath   string
	passwordFile string
	listen       string
	autoListen   bool
	autoStart    bool
	openBrowser  bool
	loggerOutput io.Writer
}

func runWeb(ctx context.Context, options webRunOptions) error {
	password, setupPasswordPath, err := loadOrSetupWebPassword(options.passwordFile)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(options.loggerOutput, &slog.HandlerOptions{Level: slog.LevelInfo}))
	server, err := webui.New(webui.Options{
		Listen:            options.listen,
		AutoListen:        options.autoListen,
		ConfigPath:        options.configPath,
		Password:          password,
		SetupPasswordPath: setupPasswordPath,
		AutoStart:         options.autoStart,
		Logger:            logger,
		OnReady: func(address string) {
			fmt.Fprintf(options.loggerOutput, "MoleX Web: %s\n", address)
			if options.openBrowser {
				go func() {
					if err := openDefaultBrowser(address); err != nil {
						logger.Warn("Could not open the default browser", "url", address, "error", err)
					}
				}()
			}
		},
	})
	if err != nil {
		return err
	}
	return server.Run(ctx)
}

func loadOrSetupWebPassword(path string) (string, string, error) {
	if path == "" {
		if password := strings.TrimRight(os.Getenv(webPasswordEnvironment), "\r\n"); password != "" {
			return password, "", nil
		}
		stateDirectory, err := os.UserConfigDir()
		if err != nil {
			return "", "", fmt.Errorf("locate user configuration directory: %w", err)
		}
		path = filepath.Join(stateDirectory, "MoleX", "web-password")
	}
	password, err := loadWebPassword(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", path, nil
	}
	return password, "", err
}

func openDefaultBrowser(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	case "darwin":
		command = exec.Command("open", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		return nil
	}
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
