package queue

import (
	"context"

	"github.com/rabbitmq/amqp091-go"
)

type Publisher struct {
	Conn     *amqp091.Connection
	Channel  *amqp091.Channel
	Exchange string
}

func NewPublisher(url string, exchange string) (*Publisher, error) {
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	err = ch.ExchangeDeclare(
		exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return &Publisher{
		Conn:     conn,
		Channel:  ch,
		Exchange: exchange,
	}, nil
}

func (p *Publisher) Publish(ctx context.Context, routingKey string, body []byte) error {
	return p.Channel.PublishWithContext(
		ctx,
		p.Exchange,
		routingKey,
		false,
		false,
		amqp091.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp091.Persistent,
			Body:         body,
		},
	)
}

func (p *Publisher) Close() {
	if p.Channel != nil {
		p.Channel.Close()
	}
	if p.Conn != nil {
		p.Conn.Close()
	}
}
