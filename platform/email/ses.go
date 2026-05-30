package email

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
)

// SESSender sends email via Amazon SES.
// Authentication uses the standard AWS credential chain:
//   - IAM instance role (EC2 production)
//   - AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY env vars (local dev / CI)
type SESSender struct {
	client *ses.Client
	from   string // e.g. "Atmos <noreply@atmosapp.dev>"
}

func NewSESSender(ctx context.Context, region, from string) (*SESSender, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("ses: load aws config: %w", err)
	}
	return &SESSender{
		client: ses.NewFromConfig(cfg),
		from:   from,
	}, nil
}

func (s *SESSender) Send(ctx context.Context, msg Message) error {
	input := &ses.SendEmailInput{
		Source: aws.String(s.from),
		Destination: &types.Destination{
			ToAddresses: []string{msg.To},
		},
		Message: &types.Message{
			Subject: &types.Content{
				Data:    aws.String(msg.Subject),
				Charset: aws.String("UTF-8"),
			},
			Body: &types.Body{
				Html: &types.Content{
					Data:    aws.String(msg.HTML),
					Charset: aws.String("UTF-8"),
				},
				Text: &types.Content{
					Data:    aws.String(msg.Text),
					Charset: aws.String("UTF-8"),
				},
			},
		},
	}
	_, err := s.client.SendEmail(ctx, input)
	if err != nil {
		return fmt.Errorf("ses: send email to %s: %w", msg.To, err)
	}
	return nil
}
