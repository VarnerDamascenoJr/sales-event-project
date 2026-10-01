package messaging

import (
	"errors"
	"fmt"

	"github.com/rabbitmq/amqp091-go"
)

var (
	ErrPublishNacked   = errors.New("rabbitmq publish was not acknowledged")
	ErrPublishReturned = errors.New("rabbitmq publish was returned")
)

type PublishReturnError struct {
	Exchange   string
	RoutingKey string
	ReplyCode  uint16
	ReplyText  string
}

func (e PublishReturnError) Error() string {
	return fmt.Sprintf("rabbitmq publish returned: exchange=%q routing_key=%q reply_code=%d reply_text=%q", e.Exchange, e.RoutingKey, e.ReplyCode, e.ReplyText)
}

func (e PublishReturnError) Unwrap() error {
	return ErrPublishReturned
}

func publishReturnError(returned amqp091.Return) error {
	return PublishReturnError{
		Exchange:   returned.Exchange,
		RoutingKey: returned.RoutingKey,
		ReplyCode:  returned.ReplyCode,
		ReplyText:  returned.ReplyText,
	}
}
