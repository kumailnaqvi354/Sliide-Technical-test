package queue

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Publisher puts messages on the article status change queue.
type Publisher struct {
	client   *sqs.Client
	queueURL string
}

func NewPublisher(endpoint, queueURL string) *Publisher {
	cfg := aws.Config{
		Region:       "eu-west-1",
		Credentials:  credentials.NewStaticCredentialsProvider("local", "local", ""),
		BaseEndpoint: aws.String(endpoint),
	}

	return &Publisher{
		client:   sqs.NewFromConfig(cfg),
		queueURL: queueURL,
	}
}

func (p *Publisher) QueueURL() string {
	return p.queueURL
}

// Publish sends a message to the queue. The queue is drained by a service we do
// not own, so a nil error means the queue accepted the message and nothing more:
// it does not mean the change has been applied.
func (p *Publisher) Publish(ctx context.Context, body string) error {
	_, err := p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(p.queueURL),
		MessageBody: aws.String(body),
	})

	return err
}
