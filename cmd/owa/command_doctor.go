package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nkiyohara/owa-bridge/internal/application"
	"github.com/nkiyohara/owa-bridge/internal/browser"
)

type doctorCommand struct {
	Account string `help:"Configured account alias; defaults to default_account."`
	Online  bool   `help:"Interactively validate the live mail and calendar contracts."`
	JSON    bool   `help:"Write a content-free machine-readable report."`
}

type doctorReport struct {
	Healthy bool          `json:"healthy"`
	Online  bool          `json:"online"`
	Version string        `json:"version"`
	Account string        `json:"account,omitempty"`
	Checks  []doctorCheck `json:"checks"`
}

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func (command *doctorCommand) Run(app *runtime) error {
	report := doctorReport{
		Healthy: true,
		Online:  command.Online,
		Version: app.info.Version,
		Checks:  make([]doctorCheck, 0, 10),
	}

	configuration, configPath, err := app.loadConfig()
	if err != nil {
		report.add("config", "fail", doctorError(err))
		return command.finish(app, report)
	}
	report.add("config", "pass", "strict secret-free configuration is valid")

	accountID, err := app.account(configuration, command.Account)
	if err != nil {
		report.add("account", "fail", doctorError(err))
		return command.finish(app, report)
	}
	report.Account = string(accountID)
	report.add("account", "pass", "configured account alias and HTTPS origin are valid")

	executable, err := browser.ResolveExecutable(configuration.Browser.Executable)
	if err != nil {
		report.add("browser", "fail", doctorError(err))
	} else {
		report.add("browser", "pass", "resolved "+sanitizeCell(filepath.Base(executable), 80))
	}

	endpoint, err := app.endpoint(configPath)
	if err != nil || endpoint.ID == "" || endpoint.Address == "" || endpoint.CredentialPath == "" {
		if err == nil {
			err = errors.New("local IPC endpoint is incomplete")
		}
		report.add("local_ipc", "fail", doctorError(err))
	} else {
		report.add("local_ipc", "pass", "config-scoped no-TCP endpoint is available")
	}

	if !command.Online {
		report.add("live_owa", "skip", "run with --online to open the interactive browser")
		return command.finish(app, report)
	}
	if !report.Healthy {
		report.add("live_owa", "skip", "local prerequisites failed")
		return command.finish(app, report)
	}

	client, status, err := app.openDaemon(app.context)
	if err != nil {
		report.add("daemon", "fail", doctorError(err))
		report.add("live_owa", "skip", "session owner is unavailable")
		return command.finish(app, report)
	}
	report.add("daemon", "pass", fmt.Sprintf("protocol %d session owner is ready", status.ProtocolVersion))

	if _, err := client.Login(app.context, accountID, app.caller()); err != nil {
		report.add("session", "fail", doctorError(err))
		report.add("folder_contract", "skip", "interactive session was not captured")
		report.add("mail_contract", "skip", "interactive session was not captured")
		report.add("calendar_contract", "skip", "interactive session was not captured")
		closeErr := client.Close()
		if closeErr != nil {
			report.add("daemon_close", "fail", doctorError(closeErr))
		}
		return command.finish(app, report)
	}
	report.add("session", "pass", "browser-owned authorization was captured in daemon memory")

	_, folderErr := client.ListMailFolders(app.context, application.MailFolderListInput{
		Account: accountID,
		Parent: application.MailFolder{
			Kind: application.MailFolderDistinguished,
			ID:   "msgfolderroot",
		},
		Traversal: application.MailFolderTraversalDeep,
		Limit:     1,
		TimeZone:  "UTC",
	}, app.caller())
	if folderErr != nil {
		report.add("folder_contract", "fail", doctorError(folderErr))
	} else {
		report.add("folder_contract", "pass", "metadata response accepted; no folder data emitted")
	}

	_, mailErr := client.ListMail(app.context, application.MailListInput{
		Account: accountID,
		Folder: application.MailFolder{
			Kind: application.MailFolderDistinguished,
			ID:   "inbox",
		},
		Limit:    1,
		TimeZone: "UTC",
	}, app.caller())
	if mailErr != nil {
		report.add("mail_contract", "fail", doctorError(mailErr))
	} else {
		report.add("mail_contract", "pass", "metadata response accepted; no message data emitted")
	}

	start := time.Now().UTC().Truncate(time.Second)
	_, calendarErr := client.ListCalendar(app.context, application.CalendarListInput{
		Account: accountID,
		Calendar: application.CalendarFolder{
			Kind: application.CalendarFolderDistinguished,
			ID:   "calendar",
		},
		Start: start.Format(time.RFC3339),
		End:   start.Add(time.Hour).Format(time.RFC3339),
	}, app.caller())
	if calendarErr != nil {
		report.add("calendar_contract", "fail", doctorError(calendarErr))
	} else {
		report.add("calendar_contract", "pass", "metadata response accepted; no event data emitted")
	}
	if err := client.Close(); err != nil {
		report.add("daemon_close", "fail", doctorError(err))
	}
	return command.finish(app, report)
}

func (report *doctorReport) add(name, status, detail string) {
	report.Checks = append(report.Checks, doctorCheck{Name: name, Status: status, Detail: detail})
	if status == "fail" {
		report.Healthy = false
	}
}

func (command *doctorCommand) finish(app *runtime, report doctorReport) error {
	var writeErr error
	if command.JSON {
		writeErr = writeJSON(app.stdout, report)
	} else {
		state := "healthy"
		if !report.Healthy {
			state = "unhealthy"
		}
		_, writeErr = fmt.Fprintf(app.stdout, "owa doctor: %s\n", state)
		for _, check := range report.Checks {
			if _, err := fmt.Fprintf(
				app.stdout,
				"[%s] %s: %s\n",
				check.Status,
				sanitizeCell(check.Name, 40),
				sanitizeCell(check.Detail, 240),
			); err != nil {
				writeErr = errors.Join(writeErr, err)
			}
		}
	}
	if !report.Healthy {
		return errors.Join(writeErr, errors.New("doctor found one or more failing checks"))
	}
	return writeErr
}

func doctorError(err error) string {
	if err == nil {
		return "unknown failure"
	}
	return sanitizeCell(err.Error(), 240)
}
