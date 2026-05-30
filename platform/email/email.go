// Package email provides a thin interface over transactional email sending.
// The only implementation today is SES; swap it by injecting a different Sender.
package email

import "context"

// Sender is the interface every email backend must satisfy.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Message is a single outbound email.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string // plain-text fallback
}
