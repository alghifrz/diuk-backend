package payment

type paymentJSON struct {
	ID            string  `json:"id"`
	ReservationID string  `json:"reservation_id"`
	Status        string  `json:"status"`
	PaymentType   string  `json:"payment_type"`
	Amount        any     `json:"amount"`
	Currency      string  `json:"currency"`
	Notes         *string `json:"notes"`
	ConfirmedAt   *string `json:"confirmed_at"`
	ConfirmedBy   *string `json:"confirmed_by"`
	RefundedAt    *string `json:"refunded_at"`
	RefundedBy    *string `json:"refunded_by"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

func paymentResponse(p *Payment) paymentJSON {
	out := paymentJSON{
		ID:            p.ID.String(),
		ReservationID: p.ReservationID.String(),
		Status:        p.Status,
		PaymentType:   p.PaymentType,
		Amount:        p.Amount,
		Currency:      p.Currency,
		Notes:         p.Notes,
		CreatedAt:     p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if p.ConfirmedAt != nil {
		s := p.ConfirmedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		out.ConfirmedAt = &s
	}
	if p.ConfirmedBy != nil {
		s := p.ConfirmedBy.String()
		out.ConfirmedBy = &s
	}
	if p.RefundedAt != nil {
		s := p.RefundedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		out.RefundedAt = &s
	}
	if p.RefundedBy != nil {
		s := p.RefundedBy.String()
		out.RefundedBy = &s
	}
	return out
}

func paymentsResponse(items []Payment) []paymentJSON {
	out := make([]paymentJSON, 0, len(items))
	for i := range items {
		out = append(out, paymentResponse(&items[i]))
	}
	return out
}
