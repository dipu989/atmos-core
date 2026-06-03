package push

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// FCMSender delivers push notifications via Firebase Cloud Messaging HTTP v1 API.
// Authentication uses a service account JSON file pointed to by serviceAccountPath.
type FCMSender struct {
	client *messaging.Client
}

// NewFCMSender initialises a Firebase messaging client from a service account JSON file.
// serviceAccountPath is the absolute path to the downloaded service account key.
func NewFCMSender(ctx context.Context, serviceAccountPath string) (*FCMSender, error) {
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile(serviceAccountPath))
	if err != nil {
		return nil, fmt.Errorf("fcm: init firebase app: %w", err)
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("fcm: get messaging client: %w", err)
	}
	return &FCMSender{client: client}, nil
}

// Send delivers a single push notification to the given FCM registration token.
func (s *FCMSender) Send(ctx context.Context, msg Message) error {
	fcmMsg := &messaging.Message{
		Token: msg.Token,
		Notification: &messaging.Notification{
			Title: msg.Title,
			Body:  msg.Body,
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				Title: msg.Title,
				Body:  msg.Body,
				Sound: "default",
			},
		},
	}
	if len(msg.Data) > 0 {
		fcmMsg.Data = msg.Data
	}

	_, err := s.client.Send(ctx, fcmMsg)
	if err != nil {
		return fmt.Errorf("fcm: send to %s: %w", msg.Token[:min(len(msg.Token), 10)], err)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
