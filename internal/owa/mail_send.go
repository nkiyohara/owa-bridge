package owa

import (
	"context"
	"errors"
	"fmt"

	"github.com/nkiyohara/owa-bridge/internal/application"
)

// SendMail creates one plain-text message, sends it, and saves the copy in Sent
// Items. CreateItem is transport-classified as external and never retried.
func (client *Client) SendMail(
	ctx context.Context,
	input application.MailSendInput,
) (application.MailSendResult, error) {
	if err := input.Validate(application.MaxMailRecipients); err != nil {
		return application.MailSendResult{}, err
	}
	var response responseEnvelope[createItemResponseBody]
	if err := client.Call(ctx, CreateItem, buildSendEnvelope(input), &response); err != nil {
		return application.MailSendResult{}, err
	}
	if len(response.Body.ResponseMessages.Items) != 1 {
		return application.MailSendResult{}, classifyPostRequestError(
			CreateItem, errors.New("OWA CreateItem send returned an unexpected response count"),
		)
	}
	message := response.Body.ResponseMessages.Items[0]
	if err := checkWriteResponse(CreateItem, message.ResponseClass, message.ResponseCode); err != nil {
		return application.MailSendResult{}, err
	}
	if len(message.Items) > 1 {
		return application.MailSendResult{}, classifyPostRequestError(
			CreateItem, errors.New("OWA CreateItem send returned multiple sent copies"),
		)
	}
	if len(message.Items) == 0 {
		return application.MailSendResult{}, nil
	}
	item := message.Items[0]
	if item.ItemID.ID == "" {
		if item.ItemID.ChangeKey != "" {
			return application.MailSendResult{}, classifyPostRequestError(
				CreateItem, errors.New("OWA CreateItem send returned a change key without a sent-copy ID"),
			)
		}
		return application.MailSendResult{}, nil
	}
	if err := validateOpaqueID("sent copy", item.ItemID.ID); err != nil {
		return application.MailSendResult{}, classifyPostRequestError(
			CreateItem, fmt.Errorf("invalid sent copy in OWA response: %w", err),
		)
	}
	if item.ItemID.ChangeKey != "" {
		if err := validateOpaqueID("sent copy change key", item.ItemID.ChangeKey); err != nil {
			return application.MailSendResult{}, classifyPostRequestError(
				CreateItem, fmt.Errorf("invalid sent copy in OWA response: %w", err),
			)
		}
	}
	return application.MailSendResult{
		ID: item.ItemID.ID, ChangeKey: item.ItemID.ChangeKey,
	}, nil
}

func buildSendEnvelope(input application.MailSendInput) createItemEnvelope {
	envelope := buildCreateDraftEnvelope(application.MailDraftInput(input))
	envelope.Body.MessageDisposition = "SendAndSaveCopy"
	envelope.Body.MessageDispositionString = "SendAndSaveCopy"
	envelope.Body.SavedItemFolderID.BaseFolderID.ID = "sentitems"
	return envelope
}
