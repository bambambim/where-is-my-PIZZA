package rabbitmq

import "github.com/rabbitmq/amqp091-go"

type Rabbit struct {
	conn *amqp091.Connection
	ch   *amqp091.Channel
}

func Connect(rabbitUrl string) (*amqp091.Connection, error) {
	conn, err := amqp091.Dial(rabbitUrl)
	if err != nil {
		return nil, err
		//log
	}
	return conn, nil
}

func CreateChannel(conn *amqp091.Connection) (*amqp091.Channel, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
		//log
	}
	return ch, nil
}

func New(conn *amqp091.Connection, ch *amqp091.Channel) *Rabbit {
	return &Rabbit{conn: conn, ch: ch}
}
