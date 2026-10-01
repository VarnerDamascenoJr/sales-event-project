package messaging

import (
	"context"
	"errors"
	"fmt"
)

func (r *RabbitMQ) waitForPublishOutcome(ctx context.Context, routingKey string) error {
	if err := r.pollReturnedPublish(ctx, routingKey); err != nil {
		return err
	}

	select {
	case returned := <-r.publishReturns:
		returnErr := publishReturnError(returned)
		if err := r.waitForPublishConfirmation(ctx, routingKey); err != nil {
			return errors.Join(returnErr, err)
		}
		return returnErr
	case confirmation := <-r.publishConfirmations:
		if !confirmation.Ack {
			return fmt.Errorf("%w: exchange=%q routing_key=%q delivery_tag=%d", ErrPublishNacked, r.exchange, routingKey, confirmation.DeliveryTag)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for rabbitmq publish confirmation: %w", ctx.Err())
	}
}

func (r *RabbitMQ) pollReturnedPublish(ctx context.Context, routingKey string) error {
	select {
	case returned := <-r.publishReturns:
		returnErr := publishReturnError(returned)
		if err := r.waitForPublishConfirmation(ctx, routingKey); err != nil {
			return errors.Join(returnErr, err)
		}
		return returnErr
	default:
		return nil
	}
}

func (r *RabbitMQ) waitForPublishConfirmation(ctx context.Context, routingKey string) error {
	select {
	case confirmation := <-r.publishConfirmations:
		if !confirmation.Ack {
			return fmt.Errorf("%w: exchange=%q routing_key=%q delivery_tag=%d", ErrPublishNacked, r.exchange, routingKey, confirmation.DeliveryTag)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for rabbitmq publish confirmation after return: %w", ctx.Err())
	}
}

func (r *RabbitMQ) drainPublishNotifications() {
	for {
		select {
		case <-r.publishConfirmations:
		case <-r.publishReturns:
		default:
			return
		}
	}
}
