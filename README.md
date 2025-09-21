# 🍕 Where's My Pizza? - A Distributed Order Management System

## Project Overview

Have you ever ordered a pizza through a delivery app and watched its status change from "Order Placed" to "In the Kitchen" and finally "Out for Delivery"? What seems like a simple status tracker is actually a complex dance between multiple independent systems. The web app where you place your order isn't directly connected to the tablet in the kitchen.

This project is a distributed restaurant order management system designed to showcase the power of **microservices architecture** and **message queue systems** for building scalable and resilient applications. It simulates a real-world restaurant workflow, from a customer placing an order to a kitchen worker preparing it and a tracking service providing real-time status updates.

The system is composed of several independent services written in Go, which communicate asynchronously via a **RabbitMQ** message broker. Order data is persisted in a **PostgreSQL** database.

### Core Concepts Demonstrated

* **Microservices Architecture:** Decomposing a monolithic application into smaller, single-responsibility services.
* **Asynchronous Communication:** Using a message queue (RabbitMQ) to decouple services and handle high message volumes without a bottleneck.
* **Work Queues and Pub/Sub Patterns:** Implementing different messaging patterns for load distribution (`Work Queue`) and state synchronization (`Publish/Subscribe`).
* **Database Transactions:** Ensuring data integrity by grouping multiple database operations into a single atomic unit.
* **Graceful Shutdowns:** Handling signals to cleanly shut down services, preventing data loss and leaving the system in a consistent state.
* **Structured Logging:** Implementing consistent, machine-readable log formats for easier debugging and monitoring.

## System Architecture

```
                                +--------------------------------------------+
                                |               PostgreSQL DB                |
                                |             (Order Storage)                |
                                +--+-------------+---------------------------+
                                   ^             ^                    |
                  (Writes & Reads) |             | (Writes & Reads)   |
                                   v             v                    |
+------------+        +-----------+              +---------------+    |
| HTTP Client|------->|  Order    |              | Kitchen       |    |
| (e.g. curl)|        |  Service  |              | Service       |    |
+------------+        +---------- +              +-+-------------+    |
                         |                         ^                  |
                (Publishes New Order)    (Publishes Status Update)    |
                         v                         |                  |
                   +-----+-------------------------+---------+        |
                   |                                         |        |
                   |         RabbitMQ Message Broker         |        |
                   |                                         |        |
                   +-----------------------------------------+        |
                              |                                       |
                              | (Status Updates)                      | (Reads)
                              v                                       v
                        +-----+-----------+         +-----+------------------+
                        | Notification    |         | Tracking               |
                        | Subscriber      |         | Service                |
                        +-----------------+         +------------------------+
```

The system consists of the following components:

* **Order Service:** The Order Service is the public-facing entry point of the restaurant system. Its primary responsibility is to receive new orders from customers via an HTTP API, validate them, store them in the database, and publish them to a message queue for the kitchen staff to process. It acts as the gatekeeper, ensuring all incoming data is correct and formatted before entering the system.
* **Kitchen Worker:** The Kitchen Worker is a background service that simulates the kitchen staff. It consumes order messages from a queue, processes them, and updates their status in the database. It is the core processing engine of the restaurant. Multiple worker instances can run concurrently to handle high order volumes and can be specialized to process specific types of orders.
* **Tracking Service:** The Tracking Service provides visibility into the restaurant's operations. It offers a read-only HTTP API for external clients (like a customer-facing app or an internal dashboard) to query the current status of orders, view an order's history, and monitor the status of all kitchen workers. It directly queries the database and does not interact with RabbitMQ.
* **Notification Subscriber:** The Notification Service is a simple subscriber that demonstrates the fanout capabilities of the messaging system. It listens for all order status updates published by the Kitchen Workers and displays them. In a real-world scenario, this service could be extended to send push notifications, emails, or SMS messages to customers.
* **RabbitMQ:** The central message broker that handles all inter-service communication.
* **PostgreSQL:** The database used for persisting all order, item, and worker data.

!

## Getting Started

### Prerequisites

* **Go:** A recent version of Go must be installed.
* **Docker & Docker Compose:** Required to easily set up the RabbitMQ and PostgreSQL services.
* **Make:** The provided `Makefile` simplifies the setup and running process.

### Setup

1.  **Clone the repository:**
    ```sh
    git clone [https://github.com/your-username/wheres-my-pizza.git](https://github.com/your-username/wheres-my-pizza.git)
    cd wheres-my-pizza
    ```

2.  **Start the infrastructure:**
    Use Docker Compose to launch the PostgreSQL and RabbitMQ containers. The `docker-compose.yml` file is configured with the necessary environment variables and port mappings.
    ```sh
    docker-compose up -d
    ```

3.  **Build the application:**
    Compile the Go application into a single executable binary.
    ```sh
    make build
    ```

## Running the Services

All services are controlled by a single binary using the `--mode` flag. You should run each service in a separate terminal window.

First, copy the content of a `config_example.yaml` to a `config.yaml` file.


### 1\. Order Service

   ```sh
   # Run the Order Service on port 3000 with 50 maximum number of concurrent orders to process.
   ./restaurant-system --mode=order-service --port=3000 --max-concurrent=50
   ```

### 2\. Kitchen Worker

**General Worker (handles all order types):**

   ```sh
   # Run a general kitchen worker
   ./restaurant-system --mode=kitchen-worker --worker-name="chef_mario" --prefetch=1 --heartbeat-interval=30
   ```

**Specialized Worker (handles specific order types):**

***Available order types: dine_in,takeout,delivery***

   ```sh
   # This worker only processes dine-in orders
   ./restaurant-system --mode=kitchen-worker --worker-name="chef_anna" --order-types="dine_in"
   ```

### 3\. Tracking Service

   ```sh
   # Run the Tracking Service on port 3002
   ./restaurant-system --mode=tracking-service --port=3002
   ```

### 4\. Notification-subscriber service

   ```sh
   # Terminal 1
   ./restaurant-system --mode=notification-subscriber

   # Terminal 2
   ./restaurant-system --mode=notification-subscriber
   ```

## API Endpoints

### Order Service

#### Place a new order

`POST /orders`

**Request Body**

```json
{
  "customer_name": "Jane Doe",
  "order_type": "takeout",
  "items": [
    { "name": "Margherita Pizza", "quantity": 1, "price": 15.99 },
    { "name": "Caesar Salad", "quantity": 1, "price": 8.99 }
  ]
}
```

**Example `curl` command:**

```sh
curl -X POST http://localhost:3000/orders \
  -H "Content-Type: application/json" \
  -d '{
        "customer_name": "Jane Doe",
        "order_type": "takeout",
        "items": [
          {"name": "Margherita Pizza", "quantity": 1, "price": 15.99},
          {"name": "Caesar Salad", "quantity": 1, "price": 8.99}
        ]
      }'
```

-----

### Tracking Service

#### Get an order's current status

`GET /orders/{order_number}/status`

**Example:**
`GET /orders/ORD_20250816_001/status`

#### Get an order's full history

`GET /orders/{order_number}/history`

**Example:**
`GET /orders/ORD_20250816_001/history`

#### Get the status of all kitchen workers

`GET /workers/status`


