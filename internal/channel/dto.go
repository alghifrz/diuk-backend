package channel

import "time"

type channelJSON struct {
	ID                    string  `json:"id"`
	Channel               string  `json:"channel"`
	Status                string  `json:"status"`
	DisplayName           *string `json:"display_name"`
	ExternalAccountID     *string `json:"external_account_id"`
	ExternalPhoneNumberID *string `json:"external_phone_number_id"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
}

type createChannelRequest struct {
	Channel               string  `json:"channel"`
	DisplayName           *string `json:"display_name"`
	ExternalAccountID     *string `json:"external_account_id"`
	ExternalPhoneNumberID *string `json:"external_phone_number_id"`
	Status                string  `json:"status"`
}

type updateChannelRequest struct {
	DisplayName           *string `json:"display_name"`
	ExternalAccountID     *string `json:"external_account_id"`
	ExternalPhoneNumberID *string `json:"external_phone_number_id"`
	Status                *string `json:"status"`
}

func channelResponse(item *BusinessChannel) channelJSON {
	return channelJSON{
		ID:                    item.ID.String(),
		Channel:               item.Channel,
		Status:                item.Status,
		DisplayName:           item.DisplayName,
		ExternalAccountID:     item.ExternalAccountID,
		ExternalPhoneNumberID: item.ExternalPhoneNumberID,
		CreatedAt:             item.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:             item.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func channelsResponse(items []BusinessChannel) []channelJSON {
	out := make([]channelJSON, 0, len(items))
	for i := range items {
		out = append(out, channelResponse(&items[i]))
	}
	return out
}
