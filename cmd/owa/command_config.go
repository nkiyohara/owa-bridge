package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/nkiyohara/owa-bridge/internal/config"
)

type configCommand struct {
	Init     configInitCommand     `cmd:"" help:"Create a safe default configuration."`
	Path     configPathCommand     `cmd:"" help:"Print the effective configuration path."`
	Validate configValidateCommand `cmd:"" help:"Strictly validate configuration."`
}

type configInitCommand struct {
	Force bool `help:"Replace an existing regular config file."`
	JSON  bool `help:"Write machine-readable JSON."`
}

func (command *configInitCommand) Run(app *runtime) error {
	path, err := app.resolvedConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil && !command.Force {
		return fmt.Errorf("config already exists at %s; use --force to replace it", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect config path: %w", err)
	}
	if err := config.Save(path, config.Default()); err != nil {
		return err
	}
	if command.JSON {
		return writeJSON(app.stdout, map[string]any{"created": true, "path": path})
	}
	_, err = fmt.Fprintf(app.stdout, "Created %s\n", path)
	return err
}

type configPathCommand struct {
	JSON bool `help:"Write machine-readable JSON."`
}

func (command *configPathCommand) Run(app *runtime) error {
	path, err := app.resolvedConfigPath()
	if err != nil {
		return err
	}
	if command.JSON {
		return writeJSON(app.stdout, map[string]string{"path": path})
	}
	_, err = fmt.Fprintln(app.stdout, path)
	return err
}

type configValidateCommand struct {
	JSON bool `help:"Write machine-readable JSON."`
}

func (command *configValidateCommand) Run(app *runtime) error {
	_, path, err := app.loadConfig()
	if err != nil {
		return err
	}
	if command.JSON {
		return writeJSON(app.stdout, map[string]any{"path": path, "valid": true})
	}
	_, err = fmt.Fprintf(app.stdout, "Valid configuration: %s\n", path)
	return err
}
