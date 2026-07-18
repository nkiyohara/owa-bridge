package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nkiyohara/owa-bridge/internal/application"
	"github.com/nkiyohara/owa-bridge/internal/approval"
	"github.com/nkiyohara/owa-bridge/internal/audit"
	"github.com/nkiyohara/owa-bridge/internal/config"
	"github.com/nkiyohara/owa-bridge/internal/daemonapi"
	"github.com/nkiyohara/owa-bridge/internal/domain"
	"github.com/nkiyohara/owa-bridge/internal/owa"
	"github.com/nkiyohara/owa-bridge/internal/paths"
)

type sessionAccount struct {
	handle   browserHandle
	mail     *application.MailService
	calendar *application.CalendarService
	captured time.Time
}

type sessionPreview struct {
	account   domain.AccountID
	expiresAt time.Time
}

// sessionBackend lazily opens one dedicated browser per configured account and
// keeps it for the lifetime of its owning server. Every adapter call passes
// through the same application guard and content-free audit recorder.
type sessionBackend struct {
	app           *runtime
	configuration config.Config
	guard         *application.Guard
	recorder      *audit.FileRecorder

	mu       sync.Mutex
	accounts map[domain.AccountID]sessionAccount
	previews map[string]sessionPreview
	closed   bool
	active   sync.WaitGroup
	close    sync.Once
	closeErr error
}

func newSessionBackend(app *runtime) (*sessionBackend, error) {
	configuration, _, err := app.loadConfig()
	if err != nil {
		return nil, err
	}
	auditPath, err := paths.AuditPath()
	if err != nil {
		return nil, err
	}
	recorder, err := audit.NewFileRecorder(auditPath, audit.Options{})
	if err != nil {
		return nil, err
	}
	approvals, err := approval.NewStore(approval.Options{})
	if err != nil {
		_ = recorder.Close()
		return nil, err
	}
	guard, err := application.NewGuard(configuration.Policy.Rules(), approvals, recorder)
	if err != nil {
		_ = recorder.Close()
		return nil, err
	}
	return &sessionBackend{
		app:           app,
		configuration: configuration,
		guard:         guard,
		recorder:      recorder,
		accounts:      make(map[domain.AccountID]sessionAccount),
		previews:      make(map[string]sessionPreview),
	}, nil
}

func (backend *sessionBackend) DefaultAccount() domain.AccountID {
	return domain.AccountID(backend.configuration.DefaultAccount)
}

func (backend *sessionBackend) Login(
	ctx context.Context,
	accountID domain.AccountID,
	_ domain.Caller,
) (daemonapi.LoginResult, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return daemonapi.LoginResult{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	account, err := backend.accountServices(ctx, accountID)
	if err != nil {
		return daemonapi.LoginResult{}, err
	}
	return daemonapi.LoginResult{
		Account: accountID, Authenticated: true, CapturedAt: account.captured,
	}, nil
}

func (backend *sessionBackend) ListMail(
	ctx context.Context,
	input application.MailListInput,
	caller domain.Caller,
) (application.MailPage, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailPage{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.MailPage{}, err
	}
	return services.mail.List(ctx, input, caller)
}

func (backend *sessionBackend) SearchMail(
	ctx context.Context,
	input application.MailSearchInput,
	caller domain.Caller,
) (application.MailPage, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailPage{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.MailPage{}, err
	}
	return services.mail.Search(ctx, input, caller)
}

func (backend *sessionBackend) ListMailFolders(
	ctx context.Context,
	input application.MailFolderListInput,
	caller domain.Caller,
) (application.MailFolderPage, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailFolderPage{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.MailFolderPage{}, err
	}
	return services.mail.ListFolders(ctx, input, caller)
}

func (backend *sessionBackend) GetMailBody(
	ctx context.Context,
	input application.MailBodyInput,
	caller domain.Caller,
) (application.MailBodyAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailBodyAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.MailBodyAccess{}, err
	}
	access, err := services.mail.GetBody(ctx, input, caller)
	if err == nil && access.Preview != nil {
		backend.rememberPreview(access.Preview.Token, input.Account, access.Preview.ExpiresAt)
	}
	return access, err
}

func (backend *sessionBackend) CreateMailDraft(
	ctx context.Context,
	input application.MailDraftInput,
	caller domain.Caller,
) (application.MailDraftAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailDraftAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.MailDraftAccess{}, err
	}
	access, err := services.mail.CreateDraft(ctx, input, caller)
	if err == nil && access.Preview != nil {
		backend.rememberPreview(access.Preview.Token, input.Account, access.Preview.ExpiresAt)
	}
	return access, err
}

func (backend *sessionBackend) CommitMailDraft(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (application.MailDraftAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailDraftAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	account, exists := backend.accountForPreview(token)
	if !exists || account.mail == nil {
		return application.MailDraftAccess{}, errors.New("invalid or expired approval token")
	}
	access, err := account.mail.CommitDraft(ctx, token, caller)
	if err != nil {
		return application.MailDraftAccess{}, err
	}
	backend.forgetPreview(token)
	return access, nil
}

func (backend *sessionBackend) SendMail(
	ctx context.Context,
	input application.MailSendInput,
	caller domain.Caller,
) (application.MailSendAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailSendAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.MailSendAccess{}, err
	}
	access, err := services.mail.Send(ctx, input, caller)
	if err == nil && access.Preview != nil {
		backend.rememberPreview(access.Preview.Token, input.Account, access.Preview.ExpiresAt)
	}
	return access, err
}

func (backend *sessionBackend) CommitMailSend(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (application.MailSendAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailSendAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	account, exists := backend.accountForPreview(token)
	if !exists || account.mail == nil {
		return application.MailSendAccess{}, errors.New("invalid or expired approval token")
	}
	access, err := account.mail.CommitSend(ctx, token, caller)
	if err != nil {
		return application.MailSendAccess{}, err
	}
	backend.forgetPreview(token)
	return access, nil
}

func (backend *sessionBackend) MoveMail(
	ctx context.Context,
	input application.MailMoveInput,
	caller domain.Caller,
) (application.MailMoveAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailMoveAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.MailMoveAccess{}, err
	}
	access, err := services.mail.Move(ctx, input, caller)
	if err == nil && access.Preview != nil {
		backend.rememberPreview(access.Preview.Token, input.Account, access.Preview.ExpiresAt)
	}
	return access, err
}

func (backend *sessionBackend) CommitMailMove(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (application.MailMoveAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailMoveAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	account, exists := backend.accountForPreview(token)
	if !exists || account.mail == nil {
		return application.MailMoveAccess{}, errors.New("invalid or expired approval token")
	}
	access, err := account.mail.CommitMove(ctx, token, caller)
	if err != nil {
		return application.MailMoveAccess{}, err
	}
	backend.forgetPreview(token)
	return access, nil
}

func (backend *sessionBackend) SetMailReadState(
	ctx context.Context,
	input application.MailReadStateInput,
	caller domain.Caller,
) (application.MailReadStateAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailReadStateAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.MailReadStateAccess{}, err
	}
	access, err := services.mail.SetReadState(ctx, input, caller)
	if err == nil && access.Preview != nil {
		backend.rememberPreview(access.Preview.Token, input.Account, access.Preview.ExpiresAt)
	}
	return access, err
}

func (backend *sessionBackend) CommitMailReadState(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (application.MailReadStateAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailReadStateAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	account, exists := backend.accountForPreview(token)
	if !exists || account.mail == nil {
		return application.MailReadStateAccess{}, errors.New("invalid or expired approval token")
	}
	access, err := account.mail.CommitReadState(ctx, token, caller)
	if err != nil {
		return application.MailReadStateAccess{}, err
	}
	backend.forgetPreview(token)
	return access, nil
}

func (backend *sessionBackend) CommitMailBody(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (application.MailBodyAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.MailBodyAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	account, exists := backend.accountForPreview(token)
	if !exists || account.mail == nil {
		return application.MailBodyAccess{}, errors.New("invalid or expired approval token")
	}
	access, err := account.mail.CommitBody(ctx, token, caller)
	if err != nil {
		return application.MailBodyAccess{}, err
	}
	backend.forgetPreview(token)
	return access, nil
}

func (backend *sessionBackend) rememberPreview(
	token string,
	account domain.AccountID,
	expiresAt time.Time,
) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	now := time.Now()
	for pendingToken, preview := range backend.previews {
		if !now.Before(preview.expiresAt) {
			delete(backend.previews, pendingToken)
		}
	}
	backend.previews[token] = sessionPreview{account: account, expiresAt: expiresAt}
}

func (backend *sessionBackend) accountForPreview(token string) (sessionAccount, bool) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	preview, exists := backend.previews[token]
	if !exists {
		return sessionAccount{}, false
	}
	if !time.Now().Before(preview.expiresAt) {
		delete(backend.previews, token)
		return sessionAccount{}, false
	}
	account, exists := backend.accounts[preview.account]
	return account, exists
}

func (backend *sessionBackend) forgetPreview(token string) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	delete(backend.previews, token)
}

func (backend *sessionBackend) ListCalendar(
	ctx context.Context,
	input application.CalendarListInput,
	caller domain.Caller,
) (application.CalendarPage, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.CalendarPage{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.CalendarPage{}, err
	}
	return services.calendar.List(ctx, input, caller)
}

func (backend *sessionBackend) CreateCalendar(
	ctx context.Context,
	input application.CalendarCreateInput,
	caller domain.Caller,
) (application.CalendarCreateAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.CalendarCreateAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.CalendarCreateAccess{}, err
	}
	access, err := services.calendar.Create(ctx, input, caller)
	if err == nil && access.Preview != nil {
		backend.rememberPreview(access.Preview.Token, input.Account, access.Preview.ExpiresAt)
	}
	return access, err
}

func (backend *sessionBackend) CommitCalendarCreate(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (application.CalendarCreateAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.CalendarCreateAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	account, exists := backend.accountForPreview(token)
	if !exists || account.calendar == nil {
		return application.CalendarCreateAccess{}, errors.New("invalid or expired approval token")
	}
	access, err := account.calendar.CommitCreate(ctx, token, caller)
	if err != nil {
		return application.CalendarCreateAccess{}, err
	}
	backend.forgetPreview(token)
	return access, nil
}

func (backend *sessionBackend) UpdateCalendar(
	ctx context.Context,
	input application.CalendarUpdateInput,
	caller domain.Caller,
) (application.CalendarUpdateAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.CalendarUpdateAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.CalendarUpdateAccess{}, err
	}
	access, err := services.calendar.Update(ctx, input, caller)
	if err == nil && access.Preview != nil {
		backend.rememberPreview(access.Preview.Token, input.Account, access.Preview.ExpiresAt)
	}
	return access, err
}

func (backend *sessionBackend) CommitCalendarUpdate(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (application.CalendarUpdateAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.CalendarUpdateAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	account, exists := backend.accountForPreview(token)
	if !exists || account.calendar == nil {
		return application.CalendarUpdateAccess{}, errors.New("invalid or expired approval token")
	}
	access, err := account.calendar.CommitUpdate(ctx, token, caller)
	if err != nil {
		return application.CalendarUpdateAccess{}, err
	}
	backend.forgetPreview(token)
	return access, nil
}

func (backend *sessionBackend) CancelCalendar(
	ctx context.Context,
	input application.CalendarCancelInput,
	caller domain.Caller,
) (application.CalendarCancelAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.CalendarCancelAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	services, err := backend.accountServices(ctx, input.Account)
	if err != nil {
		return application.CalendarCancelAccess{}, err
	}
	access, err := services.calendar.Cancel(ctx, input, caller)
	if err == nil && access.Preview != nil {
		backend.rememberPreview(access.Preview.Token, input.Account, access.Preview.ExpiresAt)
	}
	return access, err
}

func (backend *sessionBackend) CommitCalendarCancel(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (application.CalendarCancelAccess, error) {
	backend.mu.Lock()
	if backend.closed {
		backend.mu.Unlock()
		return application.CalendarCancelAccess{}, errors.New("session backend is closed")
	}
	backend.active.Add(1)
	backend.mu.Unlock()
	defer backend.active.Done()

	account, exists := backend.accountForPreview(token)
	if !exists || account.calendar == nil {
		return application.CalendarCancelAccess{}, errors.New("invalid or expired approval token")
	}
	access, err := account.calendar.CommitCancel(ctx, token, caller)
	if err != nil {
		return application.CalendarCancelAccess{}, err
	}
	backend.forgetPreview(token)
	return access, nil
}

func (backend *sessionBackend) accountServices(
	ctx context.Context,
	accountID domain.AccountID,
) (sessionAccount, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if account, exists := backend.accounts[accountID]; exists {
		return account, nil
	}
	configured, exists := backend.configuration.Accounts[string(accountID)]
	if !exists {
		return sessionAccount{}, fmt.Errorf("account %q is not configured", accountID)
	}
	handle, credentials, err := backend.app.authenticate(ctx, backend.configuration, accountID, configured)
	if err != nil {
		return sessionAccount{}, err
	}
	client, err := owa.NewClient(owa.Options{
		Origin:     configured.Origin,
		Authorizer: handle,
		UserAgent:  "owa-bridge/" + backend.app.info.Version,
	})
	if err != nil {
		return sessionAccount{}, errors.Join(err, handle.Close())
	}
	mail, err := application.NewMailService(backend.guard, client, application.MailOptions{
		MaxRecipients: backend.configuration.Policy.MaxRecipients,
	})
	if err != nil {
		return sessionAccount{}, errors.Join(err, handle.Close())
	}
	calendar, err := application.NewCalendarService(
		backend.guard,
		client,
		application.CalendarOptions{MaxAttendees: backend.configuration.Policy.MaxAttendees},
	)
	if err != nil {
		return sessionAccount{}, errors.Join(err, handle.Close())
	}
	services := sessionAccount{
		handle: handle, mail: mail, calendar: calendar, captured: credentials.CapturedAt(),
	}
	backend.accounts[accountID] = services
	return services, nil
}

func (backend *sessionBackend) Close() error {
	backend.close.Do(func() {
		backend.mu.Lock()
		backend.closed = true
		backend.mu.Unlock()
		backend.active.Wait()

		backend.mu.Lock()
		defer backend.mu.Unlock()
		closeErrors := make([]error, 0, len(backend.accounts)+1)
		for _, account := range backend.accounts {
			closeErrors = append(closeErrors, account.handle.Close())
		}
		closeErrors = append(closeErrors, backend.recorder.Close())
		backend.closeErr = errors.Join(closeErrors...)
	})
	return backend.closeErr
}
