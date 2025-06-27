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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	scalingv1 "github.com/example/scaling-operator/api/v1"
)

// ScalingRuleReconciler reconciles a ScalingRule object
type ScalingRuleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// NATSSubsResponse represents the response from NATS /subsz endpoint
type NATSSubsResponse struct {
	Subscriptions []struct {
		Subject string `json:"subject"`
		Pending int32  `json:"pending"`
	} `json:"subscriptions"`
}

//+kubebuilder:rbac:groups=scaling.example.com,resources=scalingrules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=scaling.example.com,resources=scalingrules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=scaling.example.com,resources=scalingrules/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ScalingRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ScalingRule instance
	var scalingRule scalingv1.ScalingRule
	if err := r.Get(ctx, req.NamespacedName, &scalingRule); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ScalingRule resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ScalingRule")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling ScalingRule",
		"name", scalingRule.Name,
		"namespace", scalingRule.Namespace,
		"deploymentName", scalingRule.Spec.DeploymentName)

	// Get the target deployment
	var deployment appsv1.Deployment
	deploymentKey := types.NamespacedName{
		Name:      scalingRule.Spec.DeploymentName,
		Namespace: scalingRule.Spec.Namespace,
	}

	if err := r.Get(ctx, deploymentKey, &deployment); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "Target deployment not found", "deployment", deploymentKey)
			_, updateErr := r.updateStatus(ctx, &scalingRule, 0, 0, "DeploymentNotFound", "Target deployment not found")
			return ctrl.Result{}, updateErr
		}
		logger.Error(err, "Failed to get target deployment", "deployment", deploymentKey)
		return ctrl.Result{}, err
	}

	// Get current replica count from deployment
	currentReplicas := int32(0)
	if deployment.Spec.Replicas != nil {
		currentReplicas = *deployment.Spec.Replicas
	}

	logger.Info("Found target deployment",
		"deployment", deployment.Name,
		"currentReplicas", currentReplicas)

	// Get pending message count from NATS
	pendingMessages, err := r.getNATSPendingMessages(scalingRule.Spec.NatsMonitoringURL, scalingRule.Spec.Subject)
	if err != nil {
		logger.Error(err, "Failed to get NATS pending messages",
			"natsURL", scalingRule.Spec.NatsMonitoringURL,
			"subject", scalingRule.Spec.Subject)
		// Update status with current replica count but indicate NATS error
		_, updateErr := r.updateStatus(ctx, &scalingRule, currentReplicas, 0, "NATSError", fmt.Sprintf("Failed to query NATS: %v", err))
		return ctrl.Result{}, updateErr
	}

	logger.Info("Retrieved NATS metrics",
		"subject", scalingRule.Spec.Subject,
		"pendingMessages", pendingMessages)

	// Determine if scaling is needed
	var newReplicas int32 = currentReplicas
	var reason string = "NoScalingNeeded"
	var message string = "Current replica count is appropriate"

	if pendingMessages > scalingRule.Spec.ScaleUpThreshold {
		// Scale up
		newReplicas = currentReplicas + 1
		if newReplicas > scalingRule.Spec.MaxReplicas {
			newReplicas = scalingRule.Spec.MaxReplicas
		}
		reason = "ScaledUp"
		message = fmt.Sprintf("Scaled up due to high pending messages (%d > %d)", pendingMessages, scalingRule.Spec.ScaleUpThreshold)
	} else if pendingMessages < scalingRule.Spec.ScaleDownThreshold {
		// Scale down
		newReplicas = currentReplicas - 1
		if newReplicas < scalingRule.Spec.MinReplicas {
			newReplicas = scalingRule.Spec.MinReplicas
		}
		reason = "ScaledDown"
		message = fmt.Sprintf("Scaled down due to low pending messages (%d < %d)", pendingMessages, scalingRule.Spec.ScaleDownThreshold)
	}

	// Apply scaling if needed
	if newReplicas != currentReplicas {
		logger.Info("Scaling deployment",
			"from", currentReplicas,
			"to", newReplicas,
			"reason", reason)

		deployment.Spec.Replicas = &newReplicas
		if err := r.Update(ctx, &deployment); err != nil {
			logger.Error(err, "Failed to update deployment replica count")
			return ctrl.Result{}, err
		}
		currentReplicas = newReplicas
	}

	// Update ScalingRule status
	if _, err := r.updateStatus(ctx, &scalingRule, currentReplicas, pendingMessages, reason, message); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue after the specified poll interval
	requeueAfter := time.Duration(scalingRule.Spec.PollIntervalSeconds) * time.Second
	logger.Info("Reconciliation complete, requeuing", "after", requeueAfter)

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// getNATSPendingMessages queries NATS monitoring API for pending message count
func (r *ScalingRuleReconciler) getNATSPendingMessages(natsURL, subject string) (int32, error) {
	url := fmt.Sprintf("%s/subsz", natsURL)

	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to query NATS monitoring API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("NATS monitoring API returned status %d", resp.StatusCode)
	}

	var subsResponse NATSSubsResponse
	if err := json.NewDecoder(resp.Body).Decode(&subsResponse); err != nil {
		return 0, fmt.Errorf("failed to decode NATS response: %w", err)
	}

	// Find the subscription for our subject
	for _, sub := range subsResponse.Subscriptions {
		if sub.Subject == subject {
			return sub.Pending, nil
		}
	}

	// If subject not found, return 0 (no pending messages)
	return 0, nil
}

// updateStatus updates the ScalingRule status
func (r *ScalingRuleReconciler) updateStatus(ctx context.Context, scalingRule *scalingv1.ScalingRule, currentReplicas, lastMessageCount int32, reason, message string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Update status fields
	scalingRule.Status.CurrentReplicas = currentReplicas
	scalingRule.Status.LastMessageCount = lastMessageCount

	// Update or add condition
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	// If there was an error, mark as not ready
	if reason == "DeploymentNotFound" || reason == "NATSError" {
		condition.Status = metav1.ConditionFalse
	}

	// Find existing condition or add new one
	conditionUpdated := false
	for i, existingCondition := range scalingRule.Status.Conditions {
		if existingCondition.Type == condition.Type {
			scalingRule.Status.Conditions[i] = condition
			conditionUpdated = true
			break
		}
	}

	if !conditionUpdated {
		scalingRule.Status.Conditions = append(scalingRule.Status.Conditions, condition)
	}

	// Update the status
	if err := r.Status().Update(ctx, scalingRule); err != nil {
		logger.Error(err, "Failed to update ScalingRule status")
		return ctrl.Result{}, err
	}

	logger.Info("Updated ScalingRule status",
		"currentReplicas", currentReplicas,
		"lastMessageCount", lastMessageCount,
		"reason", reason)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScalingRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scalingv1.ScalingRule{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
