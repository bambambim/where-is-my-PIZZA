package main

import (
	"encoding/json"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Order struct {
	Number string `json:"number"`
	Item   string `json:"item"`
	Qty    int    `json:"qty"`
}

func main() {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatal("channel:", err)
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"orders_min", // имя очереди
		true,         // durable
		false,        // autoDelete
		false,        // exclusive
		false,        // noWait
		nil,          // args
	)
	if err != nil {
		log.Fatal("queue declare:", err)
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatal("consume:", err)
	}

	fmt.Println("👨‍🍳 worker: жду заказы… (Ctrl+C для выхода)")
	for d := range msgs {
		var o Order
		if err := json.Unmarshal(d.Body, &o); err != nil {
			fmt.Println("bad json:", err)
			_ = d.Nack(false, false)
			continue
		}
		fmt.Printf("✅ приготовил: %s x%d (№ %s)\n", o.Item, o.Qty, o.Number)
		_ = d.Ack(false)
	}
}
