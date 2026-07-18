package main

import (
	"github.com/nkiyohara/owa-bridge/internal/daemonapi"
	"github.com/nkiyohara/owa-bridge/internal/domain"
)

// daemonMCPBackend forwards the MCP application boundary to the sole local
// session owner. Outlook credentials never enter the MCP stdio process.
type daemonMCPBackend struct {
	*daemonapi.Client
	defaultAccount domain.AccountID
}

func newDaemonMCPBackend(app *runtime) (*daemonMCPBackend, error) {
	client, status, err := app.openDaemon(app.context)
	if err != nil {
		return nil, err
	}
	return &daemonMCPBackend{
		Client: client, defaultAccount: status.DefaultAccount,
	}, nil
}

func (backend *daemonMCPBackend) DefaultAccount() domain.AccountID {
	return backend.defaultAccount
}
