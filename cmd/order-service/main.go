package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/modx/outbox/internal/config"
	"github.com/modx/outbox/internal/db"
	"github.com/modx/outbox/internal/models"
)

func main() {
	cfg := config.Load()

	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("POST /orders", createOrderHandler(database))

	addr := ":" + cfg.HTTPPort
	log.Printf("🚀 Order Service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// createOrderRequest is the expected JSON body for POST /orders.
type createOrderRequest struct {
	CustomerName string `json:"customer_name"`
	Product      string `json:"product"`
	Quantity     int    `json:"quantity"`
}

func createOrderHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if req.CustomerName == "" || req.Product == "" || req.Quantity <= 0 {
			http.Error(w, `{"error":"customer_name, product, and quantity (>0) are required"}`, http.StatusBadRequest)
			return
		}

		order := models.Order{
			ID:           uuid.New().String(),
			CustomerName: req.CustomerName,
			Product:      req.Product,
			Quantity:     req.Quantity,
			CreatedAt:    time.Now().UTC(),
		}

		// Serialize the event payload.
		payload, err := json.Marshal(order)
		if err != nil {
			http.Error(w, `{"error":"failed to marshal order"}`, http.StatusInternalServerError)
			return
		}

		outboxEvent := models.OutboxEvent{
			ID:            uuid.New().String(),
			AggregateType: "order",
			AggregateID:   order.ID,
			EventType:     "order.created",
			Payload:       payload,
			CreatedAt:     order.CreatedAt,
			Status:        models.OutboxStatusPending,
		}

		// ── Transactional write: order + outbox event in the SAME transaction ──
		tx, err := database.Begin()
		if err != nil {
			http.Error(w, `{"error":"tx begin failed"}`, http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(
			`INSERT INTO orders (id, customer_name, product, quantity, created_at) VALUES ($1,$2,$3,$4,$5)`,
			order.ID, order.CustomerName, order.Product, order.Quantity, order.CreatedAt,
		)
		if err != nil {
			tx.Rollback()
			log.Printf("insert order: %v", err)
			http.Error(w, `{"error":"insert order failed"}`, http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(
			`INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, payload, created_at, status)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			outboxEvent.ID, outboxEvent.AggregateType, outboxEvent.AggregateID,
			outboxEvent.EventType, outboxEvent.Payload, outboxEvent.CreatedAt, outboxEvent.Status,
		)
		if err != nil {
			tx.Rollback()
			log.Printf("insert outbox: %v", err)
			http.Error(w, `{"error":"insert outbox event failed"}`, http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("tx commit: %v", err)
			http.Error(w, `{"error":"tx commit failed"}`, http.StatusInternalServerError)
			return
		}

		log.Printf("✅ Order %s created with outbox event %s", order.ID, outboxEvent.ID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(order)
	}
}
