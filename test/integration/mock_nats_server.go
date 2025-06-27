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

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// MockNATSServer provides a mock NATS monitoring server for integration tests
type MockNATSServer struct {
	server          *httptest.Server
	mu              sync.RWMutex
	pendingMessages map[string]int32
	requestLog      []string
}

// SubsInfo represents NATS subscription information
type SubsInfo struct {
	Subscriptions []struct {
		Subject string `json:"subject"`
		Queue   string `json:"queue,omitempty"`
		Pending int32  `json:"pending"`
	} `json:"subscriptions"`
}

// StreamInfo represents NATS stream information
type StreamInfo struct {
	Name  string `json:"name"`
	State struct {
		Messages int32 `json:"messages"`
	} `json:"state"`
}

// JSInfo represents JetStream information
type JSInfo struct {
	Streams []StreamInfo `json:"streams"`
}

// ConsumerInfo represents NATS consumer information
type ConsumerInfo struct {
	StreamName    string `json:"stream_name"`
	Name          string `json:"name"`
	NumPending    int32  `json:"num_pending"`
	NumAckPending int32  `json:"num_ack_pending"`
}

// NewMockNATSServer creates a new mock NATS monitoring server
func NewMockNATSServer() *MockNATSServer {
	mock := &MockNATSServer{
		pendingMessages: make(map[string]int32),
		requestLog:      make([]string, 0),
	}

	mux := http.NewServeMux()
	// Handle all possible NATS monitoring endpoints
	mux.HandleFunc("/subsz", mock.handleSubsZ)
	mux.HandleFunc("/jsz", mock.handleJSZ)
	mux.HandleFunc("/connz", mock.handleConnZ)
	mux.HandleFunc("/routez", mock.handleRouteZ)
	mux.HandleFunc("/gatewayz", mock.handleGatewayZ)
	mux.HandleFunc("/leafz", mock.handleLeafZ)
	mux.HandleFunc("/varz", mock.handleVarZ)
	mux.HandleFunc("/", mock.handleDefault)

	mock.server = httptest.NewServer(mux)
	return mock
}

// URL returns the mock server URL
func (m *MockNATSServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *MockNATSServer) Close() {
	m.server.Close()
}

// SetPendingMessages sets the pending message count for a subject
func (m *MockNATSServer) SetPendingMessages(subject string, count int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pendingMessages[subject] = count
}

// GetPendingMessages gets the pending message count for a subject
func (m *MockNATSServer) GetPendingMessages(subject string) int32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pendingMessages[subject]
}

// GetRequestLog returns the log of requests made to the server
func (m *MockNATSServer) GetRequestLog() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	logCopy := make([]string, len(m.requestLog))
	copy(logCopy, m.requestLog)
	return logCopy
}

// logRequest logs an incoming request
func (m *MockNATSServer) logRequest(r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	logEntry := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	if r.URL.RawQuery != "" {
		logEntry += "?" + r.URL.RawQuery
	}
	m.requestLog = append(m.requestLog, logEntry)
	fmt.Printf("Mock NATS Server: %s\n", logEntry)
}

// handleSubsZ handles subscription monitoring requests
func (m *MockNATSServer) handleSubsZ(w http.ResponseWriter, r *http.Request) {
	// m.logRequest(r)
	m.mu.RLock()
	defer m.mu.RUnlock()

	subsInfo := SubsInfo{
		Subscriptions: []struct {
			Subject string `json:"subject"`
			Queue   string `json:"queue,omitempty"`
			Pending int32  `json:"pending"`
		}{},
	}

	// Add subscriptions for each subject we're tracking
	for subject, count := range m.pendingMessages {
		sub := struct {
			Subject string `json:"subject"`
			Queue   string `json:"queue,omitempty"`
			Pending int32  `json:"pending"`
		}{
			Subject: subject,
			Pending: count,
		}
		subsInfo.Subscriptions = append(subsInfo.Subscriptions, sub)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(subsInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleJSZ handles JetStream monitoring requests
func (m *MockNATSServer) handleJSZ(w http.ResponseWriter, r *http.Request) {
	// m.logRequest(r)
	m.mu.RLock()
	defer m.mu.RUnlock()

	jsInfo := JSInfo{
		Streams: []StreamInfo{},
	}

	i := 0
	for subject, count := range m.pendingMessages {
		streamInfo := StreamInfo{
			Name: fmt.Sprintf("stream-%s", subject),
			State: struct {
				Messages int32 `json:"messages"`
			}{
				Messages: count,
			},
		}
		jsInfo.Streams = append(jsInfo.Streams, streamInfo)
		i++
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(jsInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleConnZ handles connection monitoring requests
func (m *MockNATSServer) handleConnZ(w http.ResponseWriter, r *http.Request) {
	// m.logRequest(r)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"connections": []}`)
}

// handleRouteZ handles route monitoring requests
func (m *MockNATSServer) handleRouteZ(w http.ResponseWriter, r *http.Request) {
	// m.logRequest(r)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"routes": []}`)
}

// handleGatewayZ handles gateway monitoring requests
func (m *MockNATSServer) handleGatewayZ(w http.ResponseWriter, r *http.Request) {
	// m.logRequest(r)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"gateways": []}`)
}

// handleLeafZ handles leaf node monitoring requests
func (m *MockNATSServer) handleLeafZ(w http.ResponseWriter, r *http.Request) {
	// m.logRequest(r)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"leafnodes": []}`)
}

// handleVarZ handles general server info requests
func (m *MockNATSServer) handleVarZ(w http.ResponseWriter, r *http.Request) {
	// m.logRequest(r)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"server_id": "mock-nats", "version": "2.9.0", "go": "go1.19"}`)
}

// handleDefault handles other requests
func (m *MockNATSServer) handleDefault(w http.ResponseWriter, r *http.Request) {
	// m.logRequest(r)

	// Check if this is a stream-specific request
	if strings.Contains(r.URL.Path, "/streams/") {
		m.handleStreamInfo(w, r)
		return
	}

	// Check if this is a consumer-specific request
	if strings.Contains(r.URL.Path, "/consumers/") {
		m.handleConsumerInfo(w, r)
		return
	}

	// For any other endpoint, return a basic JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"message": "Mock NATS Server", "path": "%s"}`, r.URL.Path)
}

// handleStreamInfo handles individual stream info requests
func (m *MockNATSServer) handleStreamInfo(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Extract stream name from path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	streamName := parts[len(parts)-1]
	count, exists := m.pendingMessages[streamName]
	if !exists {
		count = 0
	}

	streamInfo := StreamInfo{
		Name: streamName,
		State: struct {
			Messages int32 `json:"messages"`
		}{
			Messages: count,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(streamInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleConsumerInfo handles individual consumer info requests
func (m *MockNATSServer) handleConsumerInfo(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Extract consumer name from path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	consumerName := parts[len(parts)-1]

	// Find a matching subject for this consumer
	var count int32 = 0
	for subject, pendingCount := range m.pendingMessages {
		if strings.Contains(consumerName, subject) || strings.Contains(subject, consumerName) {
			count = pendingCount
			break
		}
	}

	consumerInfo := ConsumerInfo{
		StreamName:    "mock-stream",
		Name:          consumerName,
		NumPending:    count,
		NumAckPending: 0,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(consumerInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
