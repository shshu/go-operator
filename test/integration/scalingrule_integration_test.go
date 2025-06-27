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
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	scalingv1 "github.com/example/scaling-operator/api/v1"
)

var _ = Describe("ScalingRule Integration", func() {
	Context("End-to-end scaling workflow", func() {
		var (
			scalingRuleName string
			deploymentName  string
			namespace       string
			scalingRule     *scalingv1.ScalingRule
			deployment      *appsv1.Deployment
			mockNATSServer  *MockNATSServer
			ctx             context.Context
			timeout         = time.Second * 60
			interval        = time.Second * 2
		)

		BeforeEach(func() {
			ctx = context.Background()

			// Generate unique names for this test
			testID := fmt.Sprintf("%d", time.Now().UnixNano())
			scalingRuleName = fmt.Sprintf("test-scaling-rule-%s", testID)
			deploymentName = fmt.Sprintf("test-deployment-%s", testID)
			namespace = "default" // Use default namespace for simplicity

			// Start mock NATS server
			mockNATSServer = NewMockNATSServer()
		})

		AfterEach(func() {
			// Clean up resources in reverse order
			if scalingRule != nil {
				By("Cleaning up ScalingRule")
				err := k8sClient.Delete(ctx, scalingRule)
				if err != nil {
					GinkgoLogr.Error(err, "Failed to delete ScalingRule")
				}

				// Wait for deletion to complete
				Eventually(func() bool {
					var sr scalingv1.ScalingRule
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      scalingRuleName,
						Namespace: namespace,
					}, &sr)
					return err != nil // Should return error when deleted
				}, time.Second*30, time.Second*1).Should(BeTrue())
			}

			if deployment != nil {
				By("Cleaning up Deployment")
				err := k8sClient.Delete(ctx, deployment)
				if err != nil {
					GinkgoLogr.Error(err, "Failed to delete Deployment")
				}
			}

			if mockNATSServer != nil {
				mockNATSServer.Close()
			}
		})

		It("Should perform complete scaling lifecycle", func() {
			By("Creating a test deployment")
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploymentName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To[int32](2), // Start with 2 replicas
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-integration-app",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "test-integration-app",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

			// Wait for deployment to be created and verify replica count
			By("Waiting for deployment to be ready")
			Eventually(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					GinkgoLogr.Error(err, "Failed to get deployment")
					return 0
				}
				GinkgoLogr.Info("Deployment status", "replicas", *dep.Spec.Replicas, "readyReplicas", dep.Status.ReadyReplicas)
				return *dep.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))

			By("Setting up mock NATS data")
			subject := "integration.test.subject"
			mockNATSServer.SetPendingMessages(subject, 0) // Start with no pending messages

			By("Creating a ScalingRule")
			scalingRule = &scalingv1.ScalingRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scalingRuleName,
					Namespace: namespace,
				},
				Spec: scalingv1.ScalingRuleSpec{

					DeploymentName:      deploymentName,
					Namespace:           namespace,
					MinReplicas:         1,
					MaxReplicas:         5,
					NatsMonitoringURL:   mockNATSServer.URL(),
					Subject:             subject,
					ScaleUpThreshold:    3,
					ScaleDownThreshold:  1,
					PollIntervalSeconds: 3, // Shorter interval for faster testing
				},
			}

			Expect(k8sClient.Create(ctx, scalingRule)).To(Succeed())

			By("Waiting for initial ScalingRule reconciliation")
			Eventually(func() bool {
				var sr scalingv1.ScalingRule
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      scalingRuleName,
					Namespace: namespace,
				}, &sr)
				if err != nil {
					GinkgoLogr.Error(err, "Failed to get ScalingRule")
					return false
				}

				GinkgoLogr.Info("ScalingRule status", "currentReplicas", sr.Status.CurrentReplicas, "conditions", len(sr.Status.Conditions))

				// Check if status has been updated
				return sr.Status.CurrentReplicas > 0
			}, timeout, interval).Should(BeTrue())

			By("Verifying initial status")
			var sr scalingv1.ScalingRule
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      scalingRuleName,
				Namespace: namespace,
			}, &sr)).To(Succeed())

			// Debug: Print actual values

			GinkgoLogr.Info("Final status check",
				"currentReplicas", sr.Status.CurrentReplicas,
				"lastMessageCount", sr.Status.LastMessageCount,
				"conditions", sr.Status.Conditions)

			// Verify deployment still has 2 replicas
			var dep appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      deploymentName,
				Namespace: namespace,
			}, &dep)).To(Succeed())

			GinkgoLogr.Info("Deployment check", "specReplicas", *dep.Spec.Replicas)

			// The current replicas should match the deployment's replica count

			Expect(sr.Status.CurrentReplicas).To(Equal(*dep.Spec.Replicas),
				fmt.Sprintf("ScalingRule CurrentReplicas (%d) should match Deployment replicas (%d)",
					sr.Status.CurrentReplicas, *dep.Spec.Replicas))

			// Should have some conditions set
			Expect(len(sr.Status.Conditions)).To(BeNumerically(">", 0), "Should have at least one condition")

			By("Testing scale up scenario")
			// Set high pending messages to trigger scale up
			mockNATSServer.SetPendingMessages(subject, 5) // Above threshold of 3

			// Wait for scale up
			Eventually(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return 0
				}
				GinkgoLogr.Info("Scale up check", "replicas", *dep.Spec.Replicas)
				return *dep.Spec.Replicas
			}, timeout, interval).Should(BeNumerically(">", 2)) // Should scale up from 2

			By("Testing scale down scenario")
			// Set low pending messages to trigger scale down
			mockNATSServer.SetPendingMessages(subject, 0) // Below threshold of 1

			// Wait for scale down
			Eventually(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return 0
				}
				GinkgoLogr.Info("Scale down check", "replicas", *dep.Spec.Replicas)
				return *dep.Spec.Replicas
			}, timeout, interval).Should(BeNumerically("<", 5)) // Should scale down

			By("Verifying final ScalingRule status")
			Eventually(func() bool {
				var sr scalingv1.ScalingRule
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      scalingRuleName,
					Namespace: namespace,
				}, &sr)
				if err != nil {
					return false
				}

				var dep appsv1.Deployment
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return false
				}

				return sr.Status.CurrentReplicas == *dep.Spec.Replicas
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle NATS server unavailability gracefully", func() {
			By("Creating a test deployment")
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploymentName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To[int32](2),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-nats-error-app",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "test-nats-error-app",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

			// Wait for deployment to be ready
			Eventually(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return 0
				}
				return *dep.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))

			By("Creating a ScalingRule with invalid NATS URL")
			scalingRule = &scalingv1.ScalingRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scalingRuleName,
					Namespace: namespace,
				},
				Spec: scalingv1.ScalingRuleSpec{

					DeploymentName:      deploymentName,
					Namespace:           namespace,
					MinReplicas:         1,
					MaxReplicas:         5,
					NatsMonitoringURL:   "http://invalid-nats-server:8222", // Invalid URL
					Subject:             "error.test.subject",
					ScaleUpThreshold:    3,
					ScaleDownThreshold:  1,
					PollIntervalSeconds: 3,
				},
			}

			Expect(k8sClient.Create(ctx, scalingRule)).To(Succeed())

			By("Verifying ScalingRule handles NATS errors gracefully")
			Eventually(func() bool {
				var sr scalingv1.ScalingRule
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      scalingRuleName,
					Namespace: namespace,
				}, &sr)
				if err != nil {
					return false
				}

				GinkgoLogr.Info("NATS error test", "currentReplicas", sr.Status.CurrentReplicas, "conditions", sr.Status.Conditions)

				// Should still update current replicas even with NATS errors
				return sr.Status.CurrentReplicas == 2
			}, timeout, interval).Should(BeTrue())

			By("Verifying deployment is not modified during NATS errors")
			Consistently(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return 0
				}
				return *dep.Spec.Replicas
			}, time.Second*15, time.Second*2).Should(Equal(int32(2)))
		})

		It("Should handle min/max replica boundaries correctly", func() {
			By("Creating a test deployment")
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploymentName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To[int32](2), // Start with 2 replicas
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-boundary-app",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "test-boundary-app",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

			// Wait for deployment to be ready
			Eventually(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return 0
				}
				return *dep.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))

			By("Setting up mock NATS with boundary test data")
			subject := "boundary.test.subject"
			mockNATSServer.SetPendingMessages(subject, 1) // Within normal range

			By("Creating a ScalingRule with tight boundaries")
			scalingRule = &scalingv1.ScalingRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scalingRuleName,
					Namespace: namespace,
				},
				Spec: scalingv1.ScalingRuleSpec{
					DeploymentName:      deploymentName,
					Namespace:           namespace,
					MinReplicas:         1,
					MaxReplicas:         3,
					NatsMonitoringURL:   mockNATSServer.URL(),
					Subject:             subject,
					ScaleUpThreshold:    2,
					ScaleDownThreshold:  1,
					PollIntervalSeconds: 2,
				},
			}

			Expect(k8sClient.Create(ctx, scalingRule)).To(Succeed())

			By("Waiting for initial reconciliation")
			Eventually(func() bool {
				var sr scalingv1.ScalingRule
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      scalingRuleName,
					Namespace: namespace,
				}, &sr)
				return err == nil && sr.Status.CurrentReplicas == 2
			}, timeout, interval).Should(BeTrue())

			By("Testing scale down to minimum")
			// Set very low pending messages
			mockNATSServer.SetPendingMessages(subject, 0)

			// Should scale down to min replicas (1)
			Eventually(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return 0
				}
				return *dep.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(1)))

			By("Verifying it doesn't scale below minimum")
			// Keep messages at 0 for a while
			Consistently(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return 0
				}
				return *dep.Spec.Replicas
			}, time.Second*10, time.Second*1).Should(Equal(int32(1)))

			By("Testing scale up to maximum")
			// Set very high pending messages
			mockNATSServer.SetPendingMessages(subject, 10)

			// Should scale up to max replicas (3)
			Eventually(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return 0
				}
				return *dep.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(3)))

			By("Verifying it doesn't scale above maximum")
			// Keep messages high for a while
			Consistently(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return 0
				}
				return *dep.Spec.Replicas
			}, time.Second*10, time.Second*1).Should(Equal(int32(3)))

			By("Verifying ScalingRule tracks the final state")
			Eventually(func() int32 {
				var sr scalingv1.ScalingRule
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      scalingRuleName,
					Namespace: namespace,
				}, &sr)
				if err != nil {
					return 0
				}
				return sr.Status.CurrentReplicas
			}, timeout, interval).Should(Equal(int32(3)))
		})

		It("Should handle rapid configuration changes", func() {
			By("Creating a test deployment")
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploymentName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To[int32](2),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-rapid-changes",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "test-rapid-changes",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

			// Wait for deployment to be ready
			Eventually(func() int32 {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return 0
				}
				return *dep.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))

			By("Creating a ScalingRule with rapid polling")
			subject := "rapid.test.subject"
			mockNATSServer.SetPendingMessages(subject, 1)

			scalingRule = &scalingv1.ScalingRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scalingRuleName,
					Namespace: namespace,
				},
				Spec: scalingv1.ScalingRuleSpec{
					DeploymentName:      deploymentName,
					Namespace:           namespace,
					MinReplicas:         1,
					MaxReplicas:         4,
					NatsMonitoringURL:   mockNATSServer.URL(),
					Subject:             subject,
					ScaleUpThreshold:    2,
					ScaleDownThreshold:  1,
					PollIntervalSeconds: 1, // Very fast polling
				},
			}

			Expect(k8sClient.Create(ctx, scalingRule)).To(Succeed())

			By("Waiting for initial reconciliation")
			Eventually(func() bool {
				var sr scalingv1.ScalingRule
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      scalingRuleName,
					Namespace: namespace,
				}, &sr)
				return err == nil && sr.Status.CurrentReplicas == 2
			}, timeout, interval).Should(BeTrue())

			By("Rapidly changing NATS message counts")
			// Simulate rapid changes in message count
			for i := 0; i < 3; i++ {
				mockNATSServer.SetPendingMessages(subject, 5) // Scale up trigger
				time.Sleep(time.Second * 2)
				mockNATSServer.SetPendingMessages(subject, 0) // Scale down trigger
				time.Sleep(time.Second * 2)
			}

			By("Verifying system remains stable after rapid changes")
			// After rapid changes, the system should stabilize
			Eventually(func() bool {
				var dep appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      deploymentName,
					Namespace: namespace,
				}, &dep)
				if err != nil {
					return false
				}

				var sr scalingv1.ScalingRule
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      scalingRuleName,
					Namespace: namespace,
				}, &sr)
				if err != nil {
					return false
				}

				GinkgoLogr.Info("Stability check",
					"deploymentReplicas", *dep.Spec.Replicas,
					"scalingRuleReplicas", sr.Status.CurrentReplicas)

				// Should be stable and tracking correctly
				return *dep.Spec.Replicas == sr.Status.CurrentReplicas
			}, timeout, interval).Should(BeTrue())
		})
	})
})
