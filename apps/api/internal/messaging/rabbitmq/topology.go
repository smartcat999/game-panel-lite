package rabbitmq

import amqp "github.com/rabbitmq/amqp091-go"

func declareTopology(channel *amqp.Channel, options Options) error {
	// Parked messages have no automatic consumer or expiry. Explicit recovery
	// must not accidentally exhaust a second delivery limit and discard them.
	if _, err := channel.QueueDeclare(options.DeadLetterQueue, true, false, false, false, amqp.Table{
		"x-queue-type": "quorum", "x-overflow": "reject-publish", "x-delivery-limit": int32(-1),
	}); err != nil {
		return err
	}
	_, err := channel.QueueDeclare(options.Queue, true, false, false, false, amqp.Table{
		"x-queue-type": "quorum", "x-overflow": "reject-publish",
		"x-delivery-limit":       int32(options.DeliveryLimit),
		"x-dead-letter-exchange": "", "x-dead-letter-routing-key": options.DeadLetterQueue,
		"x-dead-letter-strategy": "at-least-once",
	})
	return err
}
