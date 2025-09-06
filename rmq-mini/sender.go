package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

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

	// гарантируем, что очередь существует
	q, err := ch.QueueDeclare("orders_min", true, false, false, false, nil)
	if err != nil {
		log.Fatal("queue declare:", err)
	}

	o := Order{
		Number: time.Now().UTC().Format("ORD_20060102_150405"),
		Item:   "Margherita Pizza",
		Qty:    1,
	}
	body, _ := json.Marshal(o)

	// Публикуем через дефолтный exchange "" с routingKey = имя очереди.
	err = ch.Publish("", q.Name, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent, // чтобы не потерялось при рестарте брокера
		Body:         body,
	})
	if err != nil {
		log.Fatal("publish:", err)
	}

	fmt.Printf("📦 отправил заказ: %s (%s x%d)\n", o.Number, o.Item, o.Qty)
}
