package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"unicode"
	"unicode/utf8"

	"github.com/nkiyohara/owa-bridge/internal/application"
)

type mailCommand struct {
	Folders mailFoldersCommand `cmd:"" help:"Discover mail folders and their opaque IDs."`
	List    mailListCommand    `cmd:"" help:"List message metadata in a folder."`
	Search  mailSearchCommand  `cmd:"" help:"Search message metadata in one folder with Outlook AQS."`
	Body    mailBodyCommand    `cmd:"" help:"Review and read plain text for one explicit message ID."`
	Move    mailMoveCommand    `cmd:"" help:"Review and move one versioned message to one folder."`
	Mark    mailMarkCommand    `cmd:"" help:"Review and mark one versioned message read or unread."`
	Draft   mailDraftCommand   `cmd:"" help:"Review and save one plain-text draft without sending."`
	Send    mailSendCommand    `cmd:"" help:"Review and send one new plain-text message."`
}

type mailFoldersCommand struct {
	Account   string `help:"Configured account alias; defaults to default_account."`
	Parent    string `default:"msgfolderroot" help:"Well-known parent folder name."`
	ParentID  string `name:"parent-id" help:"Opaque parent folder ID; takes precedence over parent."`
	Traversal string `default:"deep" enum:"shallow,deep" help:"Folder hierarchy traversal."`
	Offset    int    `default:"0" help:"Zero-based page offset."`
	Limit     int    `default:"100" help:"Folders to return (1-100)."`
	TimeZone  string `name:"time-zone" default:"UTC" help:"OWA time-zone identifier."`
	JSON      bool   `help:"Write the stable machine-readable schema."`
}

type mailSendCommand struct {
	Account  string   `help:"Configured account alias; defaults to default_account."`
	To       []string `help:"Bare To recipient; repeat for multiple recipients."`
	CC       []string `name:"cc" help:"Bare Cc recipient; repeat for multiple recipients."`
	BCC      []string `name:"bcc" help:"Bare Bcc recipient; repeat for multiple recipients."`
	Subject  string   `help:"Message subject; CR/LF are rejected."`
	BodyFile string   `name:"body-file" help:"Plain-text body file, or - for stdin."`
	Approve  bool     `help:"Send the exact preview generated from these arguments."`
	JSON     bool     `help:"Write the stable machine-readable schema."`
}

type mailDraftCommand struct {
	Account  string   `help:"Configured account alias; defaults to default_account."`
	To       []string `help:"Bare To recipient; repeat for multiple recipients."`
	CC       []string `name:"cc" help:"Bare Cc recipient; repeat for multiple recipients."`
	BCC      []string `name:"bcc" help:"Bare Bcc recipient; repeat for multiple recipients."`
	Subject  string   `help:"Draft subject; CR/LF are rejected."`
	BodyFile string   `name:"body-file" help:"Plain-text body file, or - for stdin."`
	Approve  bool     `help:"Save the exact preview generated from these arguments when policy requires approval."`
	JSON     bool     `help:"Write the stable machine-readable schema."`
}

type mailBodyCommand struct {
	Account   string `help:"Configured account alias; defaults to default_account."`
	MessageID string `name:"message-id" help:"Exact message ID returned by mail list (required)."`
	Approve   bool   `help:"Commit an in-process preview when sensitive reads require approval."`
	JSON      bool   `help:"Write the stable machine-readable schema."`
}

type mailListCommand struct {
	Account  string `help:"Configured account alias; defaults to default_account."`
	Folder   string `default:"inbox" help:"Well-known folder name."`
	FolderID string `name:"folder-id" help:"Opaque folder ID from folder discovery."`
	Offset   int    `default:"0" help:"Zero-based page offset."`
	Limit    int    `default:"25" help:"Messages to return (1-100)."`
	TimeZone string `name:"time-zone" default:"UTC" help:"OWA time-zone identifier."`
	JSON     bool   `help:"Write the stable machine-readable schema."`
}

type mailSearchCommand struct {
	Account  string `help:"Configured account alias; defaults to default_account."`
	Folder   string `default:"inbox" help:"Well-known folder name."`
	FolderID string `name:"folder-id" help:"Opaque folder ID from folder discovery."`
	Query    string `help:"Outlook AQS query (required; 1-1024 UTF-8 bytes)."`
	Offset   int    `default:"0" help:"Zero-based page offset."`
	Limit    int    `default:"25" help:"Messages to return (1-50)."`
	TimeZone string `name:"time-zone" default:"UTC" help:"OWA time-zone identifier."`
	JSON     bool   `help:"Write the stable machine-readable schema."`
}

type mailMoveCommand struct {
	Account       string `help:"Configured account alias; defaults to default_account."`
	MessageID     string `name:"message-id" help:"Exact message ID returned by mail list or search."`
	ChangeKey     string `name:"change-key" help:"Exact change key returned with the message ID."`
	Destination   string `default:"archive" help:"Well-known destination folder."`
	DestinationID string `name:"destination-id" help:"Opaque destination folder ID; takes precedence."`
	Approve       bool   `help:"Move the exact preview generated from these arguments when policy requires approval."`
	JSON          bool   `help:"Write the stable machine-readable schema."`
}

type mailMarkCommand struct {
	Account   string `help:"Configured account alias; defaults to default_account."`
	MessageID string `name:"message-id" help:"Exact message ID returned by mail list or search."`
	ChangeKey string `name:"change-key" help:"Exact change key returned with the message ID."`
	State     string `help:"Required target state: read or unread."`
	Approve   bool   `help:"Apply the exact preview generated from these arguments when policy requires approval."`
	JSON      bool   `help:"Write the stable machine-readable schema."`
}

func (command *mailFoldersCommand) Run(app *runtime) (returnErr error) {
	configuration, _, err := app.loadConfig()
	if err != nil {
		return err
	}
	accountID, err := app.account(configuration, command.Account)
	if err != nil {
		return err
	}
	parent := application.MailFolder{Kind: application.MailFolderDistinguished, ID: command.Parent}
	if command.ParentID != "" {
		parent = application.MailFolder{Kind: application.MailFolderOpaque, ID: command.ParentID}
	}
	input := application.MailFolderListInput{
		Account: accountID, Parent: parent,
		Traversal: application.MailFolderTraversal(command.Traversal),
		Offset:    command.Offset, Limit: command.Limit, TimeZone: command.TimeZone,
	}
	if err := input.Validate(); err != nil {
		return err
	}
	client, _, err := app.openDaemon(app.context)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, client.Close()) }()
	page, err := client.ListMailFolders(app.context, input, app.caller())
	if err != nil {
		return err
	}
	if command.JSON {
		return writeJSON(app.stdout, page)
	}
	return writeMailFolderTable(app, page)
}

func (command *mailListCommand) Run(app *runtime) (returnErr error) {
	configuration, _, err := app.loadConfig()
	if err != nil {
		return err
	}
	accountID, err := app.account(configuration, command.Account)
	if err != nil {
		return err
	}
	folder := application.MailFolder{Kind: application.MailFolderDistinguished, ID: command.Folder}
	if command.FolderID != "" {
		folder = application.MailFolder{Kind: application.MailFolderOpaque, ID: command.FolderID}
	}
	input := application.MailListInput{
		Account:  accountID,
		Folder:   folder,
		Offset:   command.Offset,
		Limit:    command.Limit,
		TimeZone: command.TimeZone,
	}
	if err := input.Validate(); err != nil {
		return err
	}

	client, _, err := app.openDaemon(app.context)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, client.Close()) }()
	page, err := client.ListMail(app.context, input, app.caller())
	if err != nil {
		return err
	}
	if command.JSON {
		return writeJSON(app.stdout, page)
	}
	return writeMailTable(app, page)
}

func (command *mailSearchCommand) Run(app *runtime) (returnErr error) {
	configuration, _, err := app.loadConfig()
	if err != nil {
		return err
	}
	accountID, err := app.account(configuration, command.Account)
	if err != nil {
		return err
	}
	folder := application.MailFolder{Kind: application.MailFolderDistinguished, ID: command.Folder}
	if command.FolderID != "" {
		folder = application.MailFolder{Kind: application.MailFolderOpaque, ID: command.FolderID}
	}
	input := application.MailSearchInput{
		Account: accountID, Folder: folder, Query: command.Query,
		Offset: command.Offset, Limit: command.Limit, TimeZone: command.TimeZone,
	}
	if err := input.Validate(); err != nil {
		return err
	}

	client, _, err := app.openDaemon(app.context)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, client.Close()) }()
	page, err := client.SearchMail(app.context, input, app.caller())
	if err != nil {
		return err
	}
	if command.JSON {
		return writeJSON(app.stdout, page)
	}
	return writeMailTable(app, page)
}

func (command *mailMoveCommand) Run(app *runtime) (returnErr error) {
	configuration, _, err := app.loadConfig()
	if err != nil {
		return err
	}
	accountID, err := app.account(configuration, command.Account)
	if err != nil {
		return err
	}
	destination := application.MailFolder{
		Kind: application.MailFolderDistinguished, ID: command.Destination,
	}
	if command.DestinationID != "" {
		destination = application.MailFolder{Kind: application.MailFolderOpaque, ID: command.DestinationID}
	}
	input := application.MailMoveInput{
		Account: accountID, MessageID: command.MessageID, ChangeKey: command.ChangeKey,
		Destination: destination,
	}
	if err := input.Validate(); err != nil {
		return err
	}

	client, _, err := app.openDaemon(app.context)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, client.Close()) }()
	access, err := client.MoveMail(app.context, input, app.caller())
	if err != nil {
		return err
	}
	if access.Status == "approval_required" {
		if access.Preview == nil {
			return errors.New("mail move required approval without returning a preview")
		}
		if !command.Approve {
			if command.JSON {
				return writeJSON(app.stdout, access)
			}
			return writeMailMoveReview(app.stdout, access.Review, false)
		}
		if err := writeMailMoveReview(app.stderr, access.Review, true); err != nil {
			return err
		}
		access, err = client.CommitMailMove(app.context, access.Preview.Token, app.caller())
		if err != nil {
			return err
		}
	}
	if access.Moved == nil {
		return errors.New("mail move completed without a result")
	}
	if command.JSON {
		return writeJSON(app.stdout, access)
	}
	if access.Moved.ID == "" {
		_, err = fmt.Fprintf(
			app.stdout, "Moved message to %s; Outlook returned no new ID, so list the destination to refresh it.\n",
			sanitizeCell(moveDestinationLabel(access.Review.Destination), 120),
		)
		return err
	}
	_, err = fmt.Fprintf(
		app.stdout, "Moved message to %s; new ID: %s\n",
		sanitizeCell(moveDestinationLabel(access.Review.Destination), 120),
		sanitizeCell(access.Moved.ID, 4096),
	)
	return err
}

func moveDestinationLabel(folder application.MailFolder) string {
	if folder.Kind == application.MailFolderOpaque {
		return "folder-id:" + folder.ID
	}
	return folder.ID
}

func (command *mailMarkCommand) Run(app *runtime) (returnErr error) {
	configuration, _, err := app.loadConfig()
	if err != nil {
		return err
	}
	accountID, err := app.account(configuration, command.Account)
	if err != nil {
		return err
	}
	input := application.MailReadStateInput{
		Account: accountID, MessageID: command.MessageID, ChangeKey: command.ChangeKey,
		State: application.MailReadState(command.State),
	}
	if err := input.Validate(); err != nil {
		return err
	}
	client, _, err := app.openDaemon(app.context)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, client.Close()) }()
	access, err := client.SetMailReadState(app.context, input, app.caller())
	if err != nil {
		return err
	}
	if access.Status == "approval_required" {
		if access.Preview == nil {
			return errors.New("mail read-state update required approval without returning a preview")
		}
		if !command.Approve {
			if command.JSON {
				return writeJSON(app.stdout, access)
			}
			return writeMailReadStateReview(app.stdout, access.Review, false)
		}
		if err := writeMailReadStateReview(app.stderr, access.Review, true); err != nil {
			return err
		}
		access, err = client.CommitMailReadState(app.context, access.Preview.Token, app.caller())
		if err != nil {
			return err
		}
	}
	if access.Updated == nil {
		return errors.New("mail read-state update completed without a result")
	}
	if command.JSON {
		return writeJSON(app.stdout, access)
	}
	_, err = fmt.Fprintf(
		app.stdout, "Marked message %s.\n", sanitizeCell(string(access.Updated.State), 16),
	)
	return err
}

func (command *mailBodyCommand) Run(app *runtime) (returnErr error) {
	configuration, _, err := app.loadConfig()
	if err != nil {
		return err
	}
	accountID, err := app.account(configuration, command.Account)
	if err != nil {
		return err
	}
	input := application.MailBodyInput{Account: accountID, MessageID: command.MessageID}
	if err := input.Validate(); err != nil {
		return err
	}
	client, _, err := app.openDaemon(app.context)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, client.Close()) }()
	access, err := client.GetMailBody(app.context, input, app.caller())
	if err != nil {
		return err
	}
	if access.Status == "approval_required" {
		if access.Preview == nil {
			return errors.New("mail body read required approval without returning a preview")
		}
		if !command.Approve {
			if command.JSON {
				return writeJSON(app.stdout, access)
			}
			return writeMailBodyReview(app.stdout, command.MessageID, false)
		}
		if err := writeMailBodyReview(app.stderr, command.MessageID, true); err != nil {
			return err
		}
		access, err = client.CommitMailBody(app.context, access.Preview.Token, app.caller())
		if err != nil {
			return err
		}
	}
	if access.Body == nil {
		return errors.New("mail body operation completed without a body")
	}
	if command.JSON {
		return writeJSON(app.stdout, access)
	}
	_, err = fmt.Fprintf(
		app.stdout,
		"Message %s (private, untrusted content):\n%s\n",
		sanitizeCell(access.Body.ID, 80),
		sanitizeTerminalText(access.Body.Text),
	)
	return err
}

func (command *mailDraftCommand) Run(app *runtime) (returnErr error) {
	configuration, _, err := app.loadConfig()
	if err != nil {
		return err
	}
	accountID, err := app.account(configuration, command.Account)
	if err != nil {
		return err
	}
	body, err := readDraftBody(app, command.BodyFile)
	if err != nil {
		return err
	}
	input := application.MailDraftInput{
		Account: accountID, To: command.To, CC: command.CC, BCC: command.BCC,
		Subject: command.Subject, Body: body,
	}
	if err := input.Validate(configuration.Policy.MaxRecipients); err != nil {
		return err
	}
	client, _, err := app.openDaemon(app.context)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, client.Close()) }()
	access, err := client.CreateMailDraft(app.context, input, app.caller())
	if err != nil {
		return err
	}
	if access.Status == "approval_required" {
		if access.Preview == nil {
			return errors.New("mail draft required approval without returning a preview")
		}
		if !command.Approve {
			if command.JSON {
				return writeJSON(app.stdout, access)
			}
			return writeDraftReview(app.stdout, access.Review, false)
		}
		if err := writeDraftReview(app.stderr, access.Review, true); err != nil {
			return err
		}
		access, err = client.CommitMailDraft(app.context, access.Preview.Token, app.caller())
		if err != nil {
			return err
		}
	}
	if access.Draft == nil {
		return errors.New("mail draft operation completed without a draft ID")
	}
	if command.JSON {
		return writeJSON(app.stdout, access)
	}
	_, err = fmt.Fprintf(
		app.stdout,
		"Saved draft %s for %d recipient(s); nothing was sent.\n",
		sanitizeCell(access.Draft.ID, 80),
		len(access.Review.To)+len(access.Review.CC)+len(access.Review.BCC),
	)
	return err
}

func (command *mailSendCommand) Run(app *runtime) (returnErr error) {
	configuration, _, err := app.loadConfig()
	if err != nil {
		return err
	}
	accountID, err := app.account(configuration, command.Account)
	if err != nil {
		return err
	}
	body, err := readDraftBody(app, command.BodyFile)
	if err != nil {
		return err
	}
	input := application.MailSendInput{
		Account: accountID, To: command.To, CC: command.CC, BCC: command.BCC,
		Subject: command.Subject, Body: body,
	}
	if err := input.Validate(configuration.Policy.MaxRecipients); err != nil {
		return err
	}
	client, _, err := app.openDaemon(app.context)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, client.Close()) }()
	access, err := client.SendMail(app.context, input, app.caller())
	if err != nil {
		return err
	}
	if access.Status != "approval_required" || access.Preview == nil {
		return errors.New("mail send did not produce its mandatory preview")
	}
	if !command.Approve {
		if command.JSON {
			return writeJSON(app.stdout, access)
		}
		return writeSendReview(app.stdout, access.Review, false)
	}
	if err := writeSendReview(app.stderr, access.Review, true); err != nil {
		return err
	}
	access, err = client.CommitMailSend(app.context, access.Preview.Token, app.caller())
	if err != nil {
		return err
	}
	if access.Status != "sent" || access.Sent == nil {
		return errors.New("mail send commit completed without sent status")
	}
	if command.JSON {
		return writeJSON(app.stdout, access)
	}
	_, err = fmt.Fprintf(
		app.stdout, "Sent message to %d recipient(s); the network request was attempted once.\n",
		len(access.Review.To)+len(access.Review.CC)+len(access.Review.BCC),
	)
	return err
}

func writeSendReview(writer io.Writer, review application.MailReview, committing bool) error {
	action := "Preview only; nothing was sent. Rerun with --approve to send this exact content."
	if committing {
		action = "Committing this exact send now."
	}
	return writeMailContentReview(writer, review, action)
}

func writeDraftReview(writer io.Writer, review application.MailReview, committing bool) error {
	action := "Preview only; no draft was saved. Rerun with --approve to save this exact draft."
	if committing {
		action = "Saving this exact draft now; no message will be sent."
	}
	return writeMailContentReview(writer, review, action)
}

func writeMailContentReview(
	writer io.Writer,
	review application.MailReview,
	action string,
) error {
	_, err := fmt.Fprintf(
		writer,
		"%s\nTo: %s\nCc: %s\nBcc: %s\nSubject: %s\nBody (%d bytes, SHA-256 %s):\n%s\n",
		action,
		sanitizeCell(strings.Join(review.To, ", "), 512),
		sanitizeCell(strings.Join(review.CC, ", "), 512),
		sanitizeCell(strings.Join(review.BCC, ", "), 512),
		sanitizeCell(review.Subject, 998), review.BodyBytes,
		sanitizeCell(review.BodySHA256, 64), sanitizeTerminalText(review.BodyPreview),
	)
	return err
}

func writeMailMoveReview(
	writer io.Writer,
	review application.MailMoveReview,
	committing bool,
) error {
	action := "Preview only; no message was moved. Rerun with --approve to move this exact version."
	if committing {
		action = "Moving this exact message version now."
	}
	_, err := fmt.Fprintf(
		writer, "%s\nMessage ID: %s\nChange key: %s\nDestination: %s\n",
		action,
		sanitizeCell(review.MessageID, 4096),
		sanitizeCell(review.ChangeKey, 4096),
		sanitizeCell(moveDestinationLabel(review.Destination), 4096),
	)
	return err
}

func writeMailReadStateReview(
	writer io.Writer,
	review application.MailReadStateReview,
	committing bool,
) error {
	action := "Preview only; read state was not changed. Rerun with --approve to change this exact version."
	if committing {
		action = "Changing this exact message version now."
	}
	_, err := fmt.Fprintf(
		writer, "%s\nMessage ID: %s\nChange key: %s\nTarget state: %s\n",
		action,
		sanitizeCell(review.MessageID, 4096),
		sanitizeCell(review.ChangeKey, 4096),
		sanitizeCell(string(review.State), 16),
	)
	return err
}

func writeMailBodyReview(writer io.Writer, messageID string, committing bool) error {
	action := "Preview only; the private body was not read. Rerun with --approve to read this exact message."
	if committing {
		action = "Reading the private body of this exact message now."
	}
	_, err := fmt.Fprintf(
		writer, "%s\nMessage ID: %s\n",
		action, sanitizeCell(messageID, 4096),
	)
	return err
}

func readDraftBody(app *runtime, path string) (body string, returnErr error) {
	return readPlainTextBody(app, path, application.MaxMailDraftBodyBytes, "draft")
}

// The calendar and draft limits currently match, but remain separate typed
// contracts and may diverge without changing this reader.
//
//nolint:unparam
func readPlainTextBody(
	app *runtime,
	path string,
	maximumBytes int64,
	kind string,
) (body string, returnErr error) {
	if path == "" {
		return "", nil
	}
	reader := app.stdin
	var file *os.File
	if path != "-" {
		opened, err := os.Open(path) // #nosec G304 -- the path is explicit CLI input.
		if err != nil {
			return "", fmt.Errorf("open %s body: %w", kind, err)
		}
		file = opened
		reader = file
	}
	if file != nil {
		defer func() { returnErr = errors.Join(returnErr, file.Close()) }()
	}
	data, err := io.ReadAll(io.LimitReader(reader, maximumBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s body: %w", kind, err)
	}
	if int64(len(data)) > maximumBytes {
		return "", fmt.Errorf("%s body exceeds %d bytes", kind, maximumBytes)
	}
	return string(data), nil
}

func writeMailTable(app *runtime, page application.MailPage) error {
	if len(page.Messages) == 0 {
		_, err := fmt.Fprintln(app.stdout, "No messages.")
		return err
	}
	writer := tabwriter.NewWriter(app.stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "RECEIVED\tFROM\tSUBJECT\tFLAGS\tID"); err != nil {
		return err
	}
	for _, message := range page.Messages {
		from := message.From.Name
		if from == "" {
			from = message.From.Address
		}
		flags := ""
		if !message.IsRead {
			flags += "U"
		}
		if message.HasAttachments {
			flags += "A"
		}
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\t%s\n",
			sanitizeCell(message.ReceivedAt, 30),
			sanitizeCell(from, 32),
			sanitizeCell(message.Subject, 72),
			flags,
			sanitizeCell(message.ID, 4096),
		); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeMailFolderTable(app *runtime, page application.MailFolderPage) error {
	if len(page.Folders) == 0 {
		_, err := fmt.Fprintln(app.stdout, "No folders.")
		return err
	}
	writer := tabwriter.NewWriter(app.stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tCLASS\tITEMS\tUNREAD\tCHILDREN\tID"); err != nil {
		return err
	}
	for _, folder := range page.Folders {
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%s\t%d\t%d\t%d\t%s\n",
			sanitizeCell(folder.DisplayName, 64),
			sanitizeCell(folder.Class, 32),
			folder.TotalItemCount,
			folder.UnreadItemCount,
			folder.ChildFolderCount,
			sanitizeCell(folder.ID, 4096),
		); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func sanitizeCell(value string, maximumRunes int) string {
	value = stripTerminalSequences(value)
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= maximumRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maximumRunes-1]) + "…"
}

func stripTerminalSequences(value string) string {
	result := make([]byte, 0, len(value))
	for index := 0; index < len(value); {
		if value[index] != 0x1b {
			result = append(result, value[index])
			index++
			continue
		}
		index++
		if index >= len(value) {
			break
		}
		switch value[index] {
		case '[': // Control Sequence Introducer.
			index++
			for index < len(value) {
				final := value[index]
				index++
				if final >= 0x40 && final <= 0x7e {
					break
				}
			}
		case ']': // Operating System Command terminated by BEL or ST.
			index++
			for index < len(value) {
				if value[index] == 0x07 {
					index++
					break
				}
				if value[index] == 0x1b && index+1 < len(value) && value[index+1] == '\\' {
					index += 2
					break
				}
				index++
			}
		default:
			index++
		}
	}
	return string(result)
}

func sanitizeTerminalText(value string) string {
	value = stripTerminalSequences(value)
	return strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' {
			return character
		}
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
}
