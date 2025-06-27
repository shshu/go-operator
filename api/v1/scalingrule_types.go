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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScalingRuleSpec defines the desired state of ScalingRule
type ScalingRuleSpec struct {
	// DeploymentName is the name of the deployment to scale
	DeploymentName string `json:"deploymentName"`

	// Namespace is the namespace of the deployment
	Namespace string `json:"namespace"`

	// MinReplicas is the minimum number of replicas
	// +kubebuilder:validation:Minimum=0
	MinReplicas int32 `json:"minReplicas"`

	// MaxReplicas is the maximum number of replicas
	// +kubebuilder:validation:Minimum=1
	MaxReplicas int32 `json:"maxReplicas"`

	// NatsMonitoringURL is the NATS monitoring endpoint URL
	NatsMonitoringURL string `json:"natsMonitoringURL"`

	// Subject is the NATS queue subject to monitor
	Subject string `json:"subject"`

	// ScaleUpThreshold is the message count threshold to scale up
	// +kubebuilder:validation:Minimum=1
	ScaleUpThreshold int32 `json:"scaleUpThreshold"`

	// ScaleDownThreshold is the message count threshold to scale down
	// +kubebuilder:validation:Minimum=0
	ScaleDownThreshold int32 `json:"scaleDownThreshold"`

	// PollIntervalSeconds is the interval between NATS monitoring checks
	// +kubebuilder:validation:Minimum=1
	PollIntervalSeconds int32 `json:"pollIntervalSeconds"`
}

// ScalingRuleStatus defines the observed state of ScalingRule
type ScalingRuleStatus struct {
	// CurrentReplicas is the current number of replicas
	CurrentReplicas int32 `json:"currentReplicas,omitempty"`

	// LastScaleTime is the last time the deployment was scaled
	LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`

	// LastMessageCount is the last observed message count
	LastMessageCount int32 `json:"lastMessageCount,omitempty"`

	// Conditions represent the latest available observations of the ScalingRule's current state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Deployment",type="string",JSONPath=".spec.deploymentName"
// +kubebuilder:printcolumn:name="Subject",type="string",JSONPath=".spec.subject"
// +kubebuilder:printcolumn:name="Current Replicas",type="integer",JSONPath=".status.currentReplicas"
// +kubebuilder:printcolumn:name="Min",type="integer",JSONPath=".spec.minReplicas"
// +kubebuilder:printcolumn:name="Max",type="integer",JSONPath=".spec.maxReplicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ScalingRule is the Schema for the scalingrules API
type ScalingRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScalingRuleSpec   `json:"spec,omitempty"`
	Status ScalingRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScalingRuleList contains a list of ScalingRule
type ScalingRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScalingRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScalingRule{}, &ScalingRuleList{})
}
