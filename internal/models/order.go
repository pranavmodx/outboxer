package models

import "time"

// Order represents a customer order.
type Order struct {
	ID           string    `json:"id"`
	CustomerName string    `json:"customer_name"`
	Product      string    `json:"product"`
	Quantity     int       `json:"quantity"`
	CreatedAt    time.Time `json:"created_at"`
}
