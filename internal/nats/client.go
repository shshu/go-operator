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

package nats

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client interface for NATS operations (for testing)
type Client interface {
	GetPendingMessages(monitoringURL, subject string) (int32, error)
}

// HTTPClient implements the Client interface using HTTP requests
type HTTPClient struct {
	httpClient *http.Client
}

// NewHTTPClient creates a new HTTP-based NATS client
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Subscription represents a NATS subscription from monitoring API
type Subscription struct {
	Subject     string `json:"subject"`
	Queue       string `json:"queue,omitempty"`
	PendingMsgs int32  `json:"pending_msgs"`
}

// SubsResponse represents the response from /subsz endpoint
type SubsResponse struct {
	Subscriptions []Subscription `json:"subscriptions"`
}

// GetPendingMessages retrieves pending message count for a subject
func (c *HTTPClient) GetPendingMessages(monitoringURL, subject string) (int32, error) {
	url := fmt.Sprintf("%s/subsz", monitoringURL)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to get NATS monitoring data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("NATS monitoring endpoint returned status %d", resp.StatusCode)
	}

	var subsResponse SubsResponse
	if err := json.NewDecoder(resp.Body).Decode(&subsResponse); err != nil {
		return 0, fmt.Errorf("failed to decode NATS monitoring response: %w", err)
	}

	// Find the subscription for our subject
	for _, sub := range subsResponse.Subscriptions {
		if sub.Subject == subject {
			return sub.PendingMsgs, nil
		}
	}

	// Subject not found, return 0 pending messages
	return 0, nil
}
