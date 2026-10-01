package messaging

import "github.com/rabbitmq/amqp091-go"

func (r *RabbitMQ) Consume(queue string) (<-chan amqp091.Delivery, error) {
	if err := r.channel.Qos(10, 0, false); err != nil {
		return nil, err
	}

	return r.channel.Consume(queue, "", false, false, false, false, nil)
}
