-- Enable UUID generation if needed, though not used in this schema
-- CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Orders Table: Stores the primary details of each customer order.
CREATE TABLE "orders" (
                          "id"                BIGSERIAL       PRIMARY KEY,
                          "created_at"        TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
                          "updated_at"        TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
                          "number"            TEXT            UNIQUE NOT NULL,
                          "customer_name"     TEXT            NOT NULL,
                          "type"              TEXT            NOT NULL CHECK (type IN ('dine_in', 'takeout', 'delivery')),
                          "table_number"      INTEGER,
                          "delivery_address"  TEXT,
                          "total_amount"      DECIMAL(10, 2)  NOT NULL,
                          "priority"          INTEGER         NOT NULL DEFAULT 1,
                          "status"            TEXT            NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'cooking', 'ready', 'completed', 'cancelled')),
                          "processed_by"      TEXT,
                          "completed_at"      TIMESTAMPTZ
);

-- Order Items Table: Stores individual items belonging to an order.
CREATE TABLE "order_items" (
                               "id"          BIGSERIAL       PRIMARY KEY,
                               "created_at"  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
                               "order_id"    BIGINT          NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
                               "name"        TEXT            NOT NULL,
                               "quantity"    INTEGER         NOT NULL,
                               "price"       DECIMAL(8, 2)   NOT NULL
);

-- Order Status Log Table: Provides an audit trail for an order's lifecycle.
CREATE TABLE "order_status_log" (
                                    "id"          BIGSERIAL       PRIMARY KEY,
                                    "created_at"  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
                                    "order_id"    BIGINT          NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
                                    "status"      TEXT            NOT NULL,
                                    "changed_by"  TEXT,
                                    "changed_at"  TIMESTAMPTZ     NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                    "notes"       TEXT
);

-- Workers Table: A registry for all kitchen workers and their status.
CREATE TABLE "workers" (
                           "id"                BIGSERIAL      PRIMARY KEY,
                           "created_at"        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
                           "name"              TEXT           UNIQUE NOT NULL,
                           "type"              TEXT           NOT NULL DEFAULT 'kitchen',
                           "status"            TEXT           NOT NULL DEFAULT 'offline' CHECK (status IN ('online', 'offline', 'busy')),
                           "last_seen"         TIMESTAMPTZ    NOT NULL DEFAULT CURRENT_TIMESTAMP,
                           "orders_processed"  INTEGER        NOT NULL DEFAULT 0
);
CREATE TABLE daily_order_sequences (
                                       sequence_date   DATE    NOT NULL PRIMARY KEY,
                                       last_value      INTEGER NOT NULL
);
CREATE OR REPLACE FUNCTION get_next_daily_order_sequence()
    RETURNS INT AS $$
DECLARE
    next_val INT;
BEGIN
    -- Atomically insert or update the sequence for the current day.
    -- This is safe from race conditions.
    INSERT INTO daily_order_sequences (sequence_date, last_value)
    VALUES (CURRENT_DATE, 1)
    ON CONFLICT (sequence_date)
        DO UPDATE SET last_value = daily_order_sequences.last_value + 1
    RETURNING last_value INTO next_val;

    RETURN next_val;
END;
$$ LANGUAGE plpgsql;

-- Indexes for performance
CREATE INDEX idx_orders_number ON orders(number);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_order_items_order_id ON order_items(order_id);
CREATE INDEX idx_order_status_log_order_id ON order_status_log(order_id);
CREATE INDEX idx_workers_name ON workers(name);