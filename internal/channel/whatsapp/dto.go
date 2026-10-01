package whatsapp

type WebhookPayload struct {
	Object string  `json:"object"`
	Entry  []Entry `json:"entry"`
}

type Entry struct {
	ID      string   `json:"id"`
	Changes []Change `json:"changes"`
}

type Change struct {
	Field string `json:"field"`
	Value Value  `json:"value"`
}

type Value struct {
	MessagingProduct string    `json:"messaging_product"`
	Metadata         Metadata  `json:"metadata"`
	Contacts         []Contact `json:"contacts"`
	Messages         []Message `json:"messages"`
	Statuses         []Status  `json:"statuses"`
}

type Metadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type Contact struct {
	WaID    string `json:"wa_id"`
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
}

type Message struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      *struct {
		Body string `json:"body"`
	} `json:"text"`
}

type Status struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Timestamp   string `json:"timestamp"`
	RecipientID string `json:"recipient_id"`
}

type sendTextRequest struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Text             struct {
		Body string `json:"body"`
	} `json:"text"`
}

type sendTemplateRequest struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Template         struct {
		Name       string                  `json:"name"`
		Language   struct{ Code string }   `json:"language"`
		Components []sendTemplateComponent `json:"components,omitempty"`
	} `json:"template"`
}

type sendTemplateComponent struct {
	Type       string              `json:"type"`
	Parameters []sendTemplateParam `json:"parameters"`
}

type sendTemplateParam struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type sendTextResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

type markReadRequest struct {
	MessagingProduct string `json:"messaging_product"`
	Status           string `json:"status"`
	MessageID        string `json:"message_id"`
}

type markReadResponse struct {
	Success bool `json:"success"`
}

type providerError struct {
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}
