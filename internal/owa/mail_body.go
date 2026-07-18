package owa

import (
	"context"
	"errors"
	"fmt"

	"github.com/nkiyohara/owa-bridge/internal/application"
)

type getItemEnvelope struct {
	Type   string         `json:"__type"`
	Header requestHeader  `json:"Header"`
	Body   getItemRequest `json:"Body"`
}

type getItemRequest struct {
	Type      string            `json:"__type"`
	ItemShape itemResponseShape `json:"ItemShape"`
	ItemIDs   []itemID          `json:"ItemIds"`
}

type getItemResponseBody struct {
	ResponseMessages responseMessages[getItemResponseMessage] `json:"ResponseMessages"`
}

type getItemResponseMessage struct {
	ResponseClass string           `json:"ResponseClass"`
	ResponseCode  string           `json:"ResponseCode"`
	Items         []getItemMessage `json:"Items"`
}

type getItemMessage struct {
	ItemID itemID      `json:"ItemId"`
	Body   bodyContent `json:"Body"`
}

type bodyContent struct {
	Type     string `json:"__type,omitempty"`
	BodyType string `json:"BodyType"`
	Value    string `json:"Value"`
}

// GetMessageBody fetches plain text for one explicit item ID.
func (client *Client) GetMessageBody(
	ctx context.Context,
	input application.MailBodyInput,
) (application.MailBody, error) {
	if err := input.Validate(); err != nil {
		return application.MailBody{}, err
	}
	payload := buildGetItemBodyEnvelope(input)
	var response responseEnvelope[getItemResponseBody]
	if err := client.Call(ctx, GetItem, payload, &response); err != nil {
		return application.MailBody{}, err
	}
	if len(response.Body.ResponseMessages.Items) != 1 {
		return application.MailBody{}, errors.New("OWA GetItem returned an unexpected response count")
	}
	message := response.Body.ResponseMessages.Items[0]
	if err := checkResponse(GetItem.Name(), message.ResponseClass, message.ResponseCode); err != nil {
		return application.MailBody{}, err
	}
	if len(message.Items) != 1 {
		return application.MailBody{}, errors.New("OWA GetItem did not return exactly one message")
	}
	item := message.Items[0]
	if err := validateOpaqueID("message", item.ItemID.ID); err != nil {
		return application.MailBody{}, fmt.Errorf("invalid message in OWA response: %w", err)
	}
	if item.Body.BodyType != "Text" {
		return application.MailBody{}, errors.New("OWA GetItem did not return a plain text body")
	}
	if len(item.Body.Value) > application.MaxMailBodyBytes {
		return application.MailBody{}, fmt.Errorf("mail body exceeds %d bytes", application.MaxMailBodyBytes)
	}
	return application.MailBody{
		ID:        item.ItemID.ID,
		ChangeKey: item.ItemID.ChangeKey,
		Text:      item.Body.Value,
	}, nil
}

func buildGetItemBodyEnvelope(input application.MailBodyInput) getItemEnvelope {
	return getItemEnvelope{
		Type:   "GetItemJsonRequest:#Exchange",
		Header: newRequestHeader(defaultZone),
		Body: getItemRequest{
			Type: "GetItemRequest:#Exchange",
			ItemShape: itemResponseShape{
				Type:      "ItemResponseShape:#Exchange",
				BaseShape: "IdOnly",
				BodyType:  "Text",
				AdditionalProperties: []propertyURI{{
					Type:     "PropertyUri:#Exchange",
					FieldURI: "item:Body",
				}},
			},
			ItemIDs: []itemID{{Type: "ItemId:#Exchange", ID: input.MessageID}},
		},
	}
}
