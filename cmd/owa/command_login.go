package main

import (
	"errors"
	"fmt"
)

type loginCommand struct {
	Account string `help:"Configured account alias; defaults to default_account."`
	JSON    bool   `help:"Write machine-readable JSON."`
}

func (command *loginCommand) Run(app *runtime) (returnErr error) {
	configuration, _, err := app.loadConfig()
	if err != nil {
		return err
	}
	accountID, err := app.account(configuration, command.Account)
	if err != nil {
		return err
	}
	client, _, err := app.openDaemon(app.context)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, client.Close()) }()
	result, err := client.Login(app.context, accountID, app.caller())
	if err != nil {
		return err
	}

	if command.JSON {
		return writeJSON(app.stdout, result)
	}
	_, err = fmt.Fprintf(app.stdout, "Authenticated Outlook Web account %q.\n", accountID)
	return err
}
