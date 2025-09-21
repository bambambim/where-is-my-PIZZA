package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"where-is-my-PIZZA/internal/config"
	"where-is-my-PIZZA/internal/domain"
	"where-is-my-PIZZA/internal/logger"

	"github.com/rabbitmq/amqp091-go"
)

const (
	OrdersTopicExchange         = "orders_topic"
	NotificationsFanoutExchange = "notifications_fanout"
	KitchenQueue                = "kitchen_queue"
	NotificationsQueue          = "notifications_queue"
	KitchenDLX                  = "kitchen_dlx"
	KitchenDLQ                  = "kitchen_dlq"
)

type Client struct {
	conn           *amqp091.Connection
	log            *logger.Logger
	cfg            *config.Config
	mu             sync.RWMutex
	connected      bool
	reconnectDelay time.Duration
	maxReconnects  int
	closeCh        chan struct{}
	doneCh         chan struct{}
}

func NewClient(cfg *config.Config, log *logger.Logger) (*Client, error) {
	client := &Client{
		cfg:            cfg,
		log:            log,
		reconnectDelay: 5 * time.Second,
		maxReconnects:  10,
		closeCh:        make(chan struct{}),
		doneCh:         make(chan struct{}),
	}

	if err := client.connect(); err != nil {
		return nil, err
	}

	go client.handleReconnection()
	return client, nil
}

func (c *Client) connect() error {
	dsn := fmt.Sprintf("amqp://%s:%s@%s:%d/",
		c.cfg.RabbitMQ.User,
		c.cfg.RabbitMQ.Password,
		c.cfg.RabbitMQ.Host,
		c.cfg.RabbitMQ.Port,
	)

	var conn *amqp091.Connection
	var err error

	for i := 0; i < 5; i++ {
		conn, err = amqp091.Dial(dsn)
		if err == nil {
			c.mu.Lock()
			c.conn = conn
			c.connected = true
			c.mu.Unlock()

			c.log.Info("rabbitmq_connected", "Successfully connected to RabbitMQ")

			if err := c.setupInfra(); err != nil {
				conn.Close()
				c.mu.Lock()
				c.connected = false
				c.mu.Unlock()
				return fmt.Errorf("failed to setup RabbitMQ infrastructure: %w", err)
			}

			return nil
		}
		c.log.Debug("rabbitmq_retry", fmt.Sprintf("Failed to connect to RabbitMQ, retrying in 2 seconds... (%v)", err))
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("failed to connect to RabbitMQ after several retries: %w", err)
}

func (c *Client) handleReconnection() {
	defer close(c.doneCh)

	for {
		select {
		case <-c.closeCh:
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			time.Sleep(c.reconnectDelay)
			continue
		}

		// Wait for connection to close
		notify := conn.NotifyClose(make(chan *amqp091.Error, 1))

		select {
		case <-c.closeCh:
			return
		case err := <-notify:
			if err != nil {
				c.log.Error(err, "rabbitmq_disconnected", "RabbitMQ connection lost")
				c.mu.Lock()
				c.connected = false
				c.mu.Unlock()

				// Attempt reconnection
				c.attemptReconnect()
			}
		}
	}
}

func (c *Client) attemptReconnect() {
	for attempt := 1; attempt <= c.maxReconnects; attempt++ {
		select {
		case <-c.closeCh:
			return
		default:
		}

		c.log.Info("rabbitmq_reconnect_attempt", fmt.Sprintf("Attempting to reconnect to RabbitMQ (attempt %d/%d)", attempt, c.maxReconnects))

		if err := c.connect(); err != nil {
			c.log.Error(err, "rabbitmq_reconnect_failed", fmt.Sprintf("Reconnection attempt %d failed", attempt))

			if attempt < c.maxReconnects {
				time.Sleep(c.reconnectDelay)
				continue
			} else {
				c.log.Error(err, "rabbitmq_reconnect_exhausted", "All reconnection attempts failed")
				return
			}
		}

		c.log.Info("rabbitmq_reconnected", "Successfully reconnected to RabbitMQ")
		return
	}
}

func (c *Client) isConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.conn != nil && !c.conn.IsClosed()
}

func (c *Client) getChannel() (*amqp091.Channel, error) {
	if !c.isConnected() {
		return nil, fmt.Errorf("not connected to RabbitMQ")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.conn.Channel()
}

func (c *Client) setupInfra() error {
	ch, err := c.getChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(OrdersTopicExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}

	if err := ch.ExchangeDeclare(NotificationsFanoutExchange, "fanout", true, false, false, false, nil); err != nil {
		return err
	}

	// Dead Letter Exchange setup
	if err := ch.ExchangeDeclare(KitchenDLX, "fanout", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(KitchenDLQ, true, false, false, false, nil); err != nil {
		return err
	}
	return ch.QueueBind(KitchenDLQ, "", KitchenDLX, false, nil)
}

func (c *Client) PublishOrder(ctx context.Context, msg domain.OrderMessage) error {
	return c.publishWithRetry(ctx, func(ch *amqp091.Channel) error {
		body, err := json.Marshal(msg)
		if err != nil {
			return err
		}

		routingKey := fmt.Sprintf("kitchen.%s.%d", msg.OrderType, msg.Priority)

		return ch.PublishWithContext(ctx,
			OrdersTopicExchange,
			routingKey,
			false, // mandatory
			false, // immediate
			amqp091.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp091.Persistent,
				Priority:     uint8(msg.Priority),
				Body:         body,
			},
		)
	})
}

func (c *Client) PublishStatusUpdate(ctx context.Context, msg domain.StatusUpdateMessage) error {
	return c.publishWithRetry(ctx, func(ch *amqp091.Channel) error {
		body, err := json.Marshal(msg)
		if err != nil {
			return err
		}

		return ch.PublishWithContext(ctx,
			NotificationsFanoutExchange,
			"", // fanout ignores routing key
			false,
			false,
			amqp091.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp091.Transient, // Notifications can be transient
				Body:         body,
			},
		)
	})
}

func (c *Client) publishWithRetry(ctx context.Context, publishFunc func(*amqp091.Channel) error) error {
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ch, err := c.getChannel()
		if err != nil {
			if attempt < maxRetries {
				c.log.Debug("publish_retry", fmt.Sprintf("Failed to get channel, retrying... (attempt %d/%d)", attempt, maxRetries))
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			return fmt.Errorf("failed to get channel after %d attempts: %w", maxRetries, err)
		}

		err = publishFunc(ch)
		ch.Close()

		if err != nil {
			if attempt < maxRetries {
				c.log.Debug("publish_retry", fmt.Sprintf("Publish failed, retrying... (attempt %d/%d): %v", attempt, maxRetries, err))
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			return fmt.Errorf("failed to publish after %d attempts: %w", maxRetries, err)
		}

		return nil
	}

	return fmt.Errorf("publish failed after %d attempts", maxRetries)
}

func (c *Client) ConsumeOrders(prefetchCount int, routingKeys []string) (<-chan amqp091.Delivery, error) {
	ch, err := c.getChannel()
	if err != nil {
		return nil, err
	}

	if err := ch.Qos(prefetchCount, 0, false); err != nil {
		ch.Close()
		return nil, err
	}

	queue, err := ch.QueueDeclare(
		KitchenQueue,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		amqp091.Table{
			"x-dead-letter-exchange": KitchenDLX,
		},
	)
	if err != nil {
		ch.Close()
		return nil, err
	}

	for _, key := range routingKeys {
		if err := ch.QueueBind(queue.Name, key, OrdersTopicExchange, false, nil); err != nil {
			ch.Close()
			return nil, err
		}
	}

	deliveries, err := ch.Consume(
		queue.Name,
		"",    // consumer
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		ch.Close()
		return nil, err
	}

	// Monitor channel for closure and handle reconnection
	go c.monitorChannel(ch, "kitchen_consumer")

	return deliveries, nil
}

func (c *Client) ConsumeNotifications() (<-chan amqp091.Delivery, error) {
	ch, err := c.getChannel()
	if err != nil {
		return nil, err
	}

	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		ch.Close()
		return nil, err
	}

	if err := ch.QueueBind(q.Name, "", NotificationsFanoutExchange, false, nil); err != nil {
		ch.Close()
		return nil, err
	}

	deliveries, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		ch.Close()
		return nil, err
	}

	// Monitor channel for closure
	go c.monitorChannel(ch, "notification_consumer")

	return deliveries, nil
}

func (c *Client) monitorChannel(ch *amqp091.Channel, consumerType string) {
	notify := ch.NotifyClose(make(chan *amqp091.Error, 1))
	select {
	case <-c.closeCh:
		return
	case err := <-notify:
		if err != nil {
			c.log.Error(err, "channel_closed", fmt.Sprintf("Channel closed for %s", consumerType))
		}
	}
}

func (c *Client) Close() {
	close(c.closeCh)

	// Wait for reconnection goroutine to finish
	select {
	case <-c.doneCh:
	case <-time.After(5 * time.Second):
		c.log.Debug("close_timeout", "Timeout waiting for reconnection goroutine to finish")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.connected = false
	}
}
