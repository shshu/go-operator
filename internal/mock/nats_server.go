/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mock

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
)

// MockNATSServer provides a simple mock NATS monitoring server
type MockNATSServer struct {
	port            int
	pendingMessages map[string]int32 // subject -> pending count
	mutex           sync.RWMutex
	server          *http.Server
}

// Subscription represents a mock NATS subscription
type Subscription struct {
	Subject       string `json:"subject"`
	Queue         string `json:"queue,omitempty"`
	PendingMsgs   int32  `json:"pending_msgs"`
	DeliveredMsgs int64  `json:"delivered_msgs"`
	DroppedMsgs   int64  `json:"dropped_msgs"`
	MaxPending    int32  `json:"max_pending"`
}

// SubsResponse represents the mock /subsz response
type SubsResponse struct {
	Subscriptions []Subscription `json:"subscriptions"`
}

// NewMockNATSServer creates a new mock NATS server
func NewMockNATSServer(port int) *MockNATSServer {
	return &MockNATSServer{
		port:            port,
		pendingMessages: make(map[string]int32),
	}
}

// SetPendingMessages sets the pending message count for a subject
func (m *MockNATSServer) SetPendingMessages(subject string, count int32) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.pendingMessages[subject] = count
	log.Printf("Mock NATS: Set pending messages for subject '%s' to %d", subject, count)
}

// GetPendingMessages gets the pending message count for a subject
func (m *MockNATSServer) GetPendingMessages(subject string) int32 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.pendingMessages[subject]
}

// Start starts the mock NATS server
func (m *MockNATSServer) Start() error {
	mux := http.NewServeMux()

	// Handle /subsz endpoint
	mux.HandleFunc("/subsz", m.handleSubsz)

	// Handle control endpoints for testing
	mux.HandleFunc("/mock/set", m.handleSetPending)
	mux.HandleFunc("/mock/get", m.handleGetPending)
	mux.HandleFunc("/mock/increment", m.handleIncrement)
	mux.HandleFunc("/mock/decrement", m.handleDecrement)

	m.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", m.port),
		Handler: mux,
	}

	log.Printf("Starting Mock NATS server on port %d", m.port)
	return m.server.ListenAndServe()
}

// Stop stops the mock NATS server
func (m *MockNATSServer) Stop() error {
	if m.server != nil {
		return m.server.Close()
	}
	return nil
}

// handleSubsz handles the /subsz endpoint (NATS monitoring format)
func (m *MockNATSServer) handleSubsz(w http.ResponseWriter, r *http.Request) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var subscriptions []Subscription
	for subject, pending := range m.pendingMessages {
		subscriptions = append(subscriptions, Subscription{
			Subject:       subject,
			PendingMsgs:   pending,
			DeliveredMsgs: 100, // Mock value
			DroppedMsgs:   0,
			MaxPending:    1000,
		})
	}

	response := SubsResponse{
		Subscriptions: subscriptions,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSetPending handles setting pending messages via HTTP
// Usage: POST /mock/set?subject=my.subject&count=10
func (m *MockNATSServer) handleSetPending(w http.ResponseWriter, r *http.Request) {
	subject := r.URL.Query().Get("subject")
	countStr := r.URL.Query().Get("count")

	if subject == "" || countStr == "" {
		http.Error(w, "Missing subject or count parameter", http.StatusBadRequest)
		return
	}

	count, err := strconv.ParseInt(countStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid count parameter", http.StatusBadRequest)
		return
	}

	m.SetPendingMessages(subject, int32(count))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"subject": subject,
		"count":   count,
		"message": "Pending messages updated",
	})
}

// handleGetPending handles getting pending messages via HTTP
// Usage: GET /mock/get?subject=my.subject
func (m *MockNATSServer) handleGetPending(w http.ResponseWriter, r *http.Request) {
	subject := r.URL.Query().Get("subject")
	if subject == "" {
		http.Error(w, "Missing subject parameter", http.StatusBadRequest)
		return
	}

	count := m.GetPendingMessages(subject)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"subject": subject,
		"count":   count,
	})
}

// handleIncrement handles incrementing pending messages
// Usage: POST /mock/increment?subject=my.subject&amount=5
func (m *MockNATSServer) handleIncrement(w http.ResponseWriter, r *http.Request) {
	subject := r.URL.Query().Get("subject")
	amountStr := r.URL.Query().Get("amount")

	if subject == "" {
		http.Error(w, "Missing subject parameter", http.StatusBadRequest)
		return
	}

	amount := int32(1) // default increment
	if amountStr != "" {
		if parsed, err := strconv.ParseInt(amountStr, 10, 32); err == nil {
			amount = int32(parsed)
		}
	}

	m.mutex.Lock()
	m.pendingMessages[subject] += amount
	newCount := m.pendingMessages[subject]
	m.mutex.Unlock()

	log.Printf("Mock NATS: Incremented subject '%s' by %d, new count: %d", subject, amount, newCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"subject":   subject,
		"increment": amount,
		"newCount":  newCount,
	})
}

// handleDecrement handles decrementing pending messages
// Usage: POST /mock/decrement?subject=my.subject&amount=3
func (m *MockNATSServer) handleDecrement(w http.ResponseWriter, r *http.Request) {
	subject := r.URL.Query().Get("subject")
	amountStr := r.URL.Query().Get("amount")

	if subject == "" {
		http.Error(w, "Missing subject parameter", http.StatusBadRequest)
		return
	}

	amount := int32(1) // default decrement
	if amountStr != "" {
		if parsed, err := strconv.ParseInt(amountStr, 10, 32); err == nil {
			amount = int32(parsed)
		}
	}

	m.mutex.Lock()
	m.pendingMessages[subject] -= amount
	if m.pendingMessages[subject] < 0 {
		m.pendingMessages[subject] = 0
	}
	newCount := m.pendingMessages[subject]
	m.mutex.Unlock()

	log.Printf("Mock NATS: Decremented subject '%s' by %d, new count: %d", subject, amount, newCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"subject":   subject,
		"decrement": amount,
		"newCount":  newCount,
	})
}
