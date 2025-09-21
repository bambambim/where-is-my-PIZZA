package notificationservice

import (
	"context"
	"encoding/json"
	"fmt"

	"where-is-my-PIZZA/internal/domain"
	"where-is-my-PIZZA/internal/logger"

	"github.com/rabbitmq/amqp091-go"
)

type Broker interface {
	ConsumeNotifications() (<-chan amqp091.Delivery, error)
}

type Subscriber struct {
	broker Broker
	log    *logger.Logger
}

func NewSubscriber(broker Broker, log *logger.Logger) *Subscriber {
	return &Subscriber{broker: broker, log: log}
}

func (s *Subscriber) Run(ctx context.Context) error {
	msgs, err := s.broker.ConsumeNotifications()
	if err != nil {
		s.log.Error(err, "rabbitmq_consume_failed", "Failed to start consuming notifications")
		return err
	}
	s.log.Info("subscriber_started", "Notification subscriber waiting for status updates...")

	for {
		select {
		case <-ctx.Done():
			s.log.Info("shutdown_signal", "Context canceled, stopping subscriber")
			return nil
		case msg, ok := <-msgs:
			if !ok {
				s.log.Info("channel_closed", "Notification channel closed by broker")
				return nil
			}
			s.handleMessage(msg)
		}
	}
}

func (s *Subscriber) handleMessage(msg amqp091.Delivery) {
	var update domain.StatusUpdateMessage
	if err := json.Unmarshal(msg.Body, &update); err != nil {
		s.log.Error(err, "message_unmarshal_failed", "Cannot parse status update message")
		return
	}

	s.log.Info("notification_received",
		fmt.Sprintf("Received status update for order %s", update.OrderNumber),
		update.OrderNumber)

	fmt.Printf("🔔 Notification for order %s: Status changed from '%s' to '%s' by %s.\n",
		update.OrderNumber,
		update.OldStatus,
		update.NewStatus,
		update.ChangedBy,
	)
}
