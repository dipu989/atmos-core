// Package push provides a thin interface over mobile push notification delivery.
// Current implementations: FCM (Android). APNs (iOS/iPadOS) can be added later.
package push

import "context"

// Message is a single push notification to one device.
type Message struct {
	// Token is the FCM registration token or APNs device token.
	Token string
	Title string
	Body  string
	// Data carries arbitrary key-value pairs for the app to act on (e.g. deep-link target).
	Data map[string]string
}

// Sender delivers push notifications.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}
