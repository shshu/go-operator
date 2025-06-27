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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Mock NATS Server", func() {
	var mockServer *MockNATSServer

	BeforeEach(func() {
		mockServer = NewMockNATSServer()
	})

	AfterEach(func() {
		if mockServer != nil {
			mockServer.Close()
		}
	})

	It("Should handle subsz endpoint correctly", func() {
		By("Setting up test data")
		mockServer.SetPendingMessages("test.subject.1", 5)
		mockServer.SetPendingMessages("test.subject.2", 10)

		By("Making request to subsz endpoint")
		resp, err := http.Get(fmt.Sprintf("%s/subsz", mockServer.URL()))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		By("Verifying response structure")
		var subsInfo SubsInfo
		err = json.NewDecoder(resp.Body).Decode(&subsInfo)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(subsInfo.Subscriptions)).To(Equal(2))

		// Verify the subjects and pending counts
		subjectCounts := make(map[string]int32)
		for _, sub := range subsInfo.Subscriptions {
			subjectCounts[sub.Subject] = sub.Pending
		}

		Expect(subjectCounts["test.subject.1"]).To(Equal(int32(5)))
		Expect(subjectCounts["test.subject.2"]).To(Equal(int32(10)))
	})

	It("Should handle jsz endpoint correctly", func() {
		By("Setting up test data")
		mockServer.SetPendingMessages("stream.1", 3)
		mockServer.SetPendingMessages("stream.2", 7)

		By("Making request to jsz endpoint")
		resp, err := http.Get(fmt.Sprintf("%s/jsz", mockServer.URL()))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		By("Verifying response structure")
		var jsInfo JSInfo
		err = json.NewDecoder(resp.Body).Decode(&jsInfo)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(jsInfo.Streams)).To(Equal(2))

		// Verify message counts
		totalMessages := int32(0)
		for _, stream := range jsInfo.Streams {
			totalMessages += stream.State.Messages
		}

		Expect(totalMessages).To(Equal(int32(10))) // 3 + 7
	})

	It("Should handle concurrent access correctly", func() {
		By("Setting up initial data")
		mockServer.SetPendingMessages("concurrent.test", 1)

		By("Making concurrent requests")
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func(id int) {
				defer GinkgoRecover()

				// Update data
				mockServer.SetPendingMessages(fmt.Sprintf("concurrent.test.%d", id), int32(id))

				// Read data
				resp, err := http.Get(fmt.Sprintf("%s/subsz", mockServer.URL()))
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				done <- true
			}(i)
		}

		By("Waiting for all goroutines to complete")
		for i := 0; i < 10; i++ {
			Eventually(done).Should(Receive())
		}

		By("Verifying final state")
		resp, err := http.Get(fmt.Sprintf("%s/subsz", mockServer.URL()))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		var subsInfo SubsInfo
		err = json.NewDecoder(resp.Body).Decode(&subsInfo)
		Expect(err).NotTo(HaveOccurred())

		// Should have 11 subjects (1 initial + 10 from goroutines)
		Expect(len(subsInfo.Subscriptions)).To(Equal(11))
	})

	It("Should return empty data for unknown endpoints", func() {
		By("Making request to unknown endpoint")
		resp, err := http.Get(fmt.Sprintf("%s/unknown", mockServer.URL()))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})
