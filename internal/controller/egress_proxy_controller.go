/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, aftware
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// EgressProxyReconciler reconciles Services with egress annotations
type EgressProxyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch;create;update;patch;delete

// Reconcile processes Service objects to create egress proxy pods
func (r *EgressProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Service
	service := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if errors.IsNotFound(err) {
			// Service deleted, clean up proxy pod
			return r.cleanupProxyPod(ctx, req.NamespacedName)
		}
		return ctrl.Result{}, err
	}

	// Check if egress is enabled
	if !isEgressEnabled(service) {
		// Egress not enabled, clean up any existing proxy pod
		return r.cleanupProxyPod(ctx, req.NamespacedName)
	}

	// Get egress configuration
	targetIP, targetDNS := getEgressTarget(service)
	if targetIP == "" && targetDNS == "" {
		logger.Info("Egress enabled but no target specified", "service", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// Create or update proxy pod
	if err := r.ensureProxyPod(ctx, service, targetIP, targetDNS); err != nil {
		logger.Error(err, "Failed to ensure proxy pod", "service", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// Update Service endpoints
	if err := r.updateServiceEndpoints(ctx, service); err != nil {
		logger.Error(err, "Failed to update service endpoints", "service", req.NamespacedName)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// isEgressEnabled checks if egress is enabled for the service
func isEgressEnabled(service *corev1.Service) bool {
	if service.Annotations == nil {
		return false
	}
	return service.Annotations["netmaker.io/egress"] == "enabled"
}

// getEgressTarget extracts egress target configuration from service annotations
// Target ports are read from Service spec's targetPort (standard Kubernetes way)
func getEgressTarget(service *corev1.Service) (targetIP, targetDNS string) {
	if service.Annotations == nil {
		return "", ""
	}

	targetIP = service.Annotations["netmaker.io/egress-target-ip"]
	targetDNS = service.Annotations["netmaker.io/egress-target-dns"]

	return targetIP, targetDNS
}

// ensureProxyPod creates or updates the egress proxy pod
func (r *EgressProxyReconciler) ensureProxyPod(ctx context.Context, service *corev1.Service, targetIP, targetDNS string) error {
	logger := log.FromContext(ctx)
	podName := fmt.Sprintf("%s-egress-proxy", service.Name)

	// Check if pod already exists
	existingPod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      podName,
		Namespace: service.Namespace,
	}, existingPod)

	if err == nil {
		// Pod exists, check if it needs update
		if needsUpdate(existingPod, targetIP, targetDNS) {
			logger.Info("Updating egress proxy pod", "pod", podName)
			return r.updateProxyPod(ctx, existingPod, service, targetIP, targetDNS)
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	// Create new pod
	logger.Info("Creating egress proxy pod", "pod", podName, "targetIP", targetIP, "targetDNS", targetDNS)

	pod := r.buildProxyPod(ctx, service, podName, targetIP, targetDNS)

	if err := r.Create(ctx, pod); err != nil {
		return fmt.Errorf("failed to create proxy pod: %w", err)
	}

	return nil
}

// buildProxyPod builds the egress proxy pod specification
// Target ports are read from Service spec's targetPort (standard Kubernetes way)
func (r *EgressProxyReconciler) buildProxyPod(ctx context.Context, service *corev1.Service, podName string, targetIP, targetDNS string) *corev1.Pod {
	// Get configuration from environment or use defaults
	netclientImage := getEnvOrDefault("NETCLIENT_IMAGE", "gravitl/netclient:v1.2.0")
	// Try to get token from secret first (checks Service annotations), fallback to environment variable
	netclientToken := r.getNetclientToken(ctx, service)
	// Use socat for simple TCP forwarding - much lighter than nginx
	proxyImage := getEnvOrDefault("EGRESS_PROXY_IMAGE", "alpine/socat:latest")

	// Build target address (used in pod labels, actual proxying handled by nginx config)

	// Build pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: service.Namespace,
			Labels: map[string]string{
				"app":                "netmaker-egress-proxy",
				"service-name":       service.Name,
				"managed-by":         "netmaker-k8s-ops",
				"netmaker.io/egress": "enabled",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       service.Name,
					UID:        service.UID,
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				// Netclient sidecar
				{
					Name:  "netclient",
					Image: netclientImage,
					Env:   r.buildNetclientEnvVars(ctx, service, netclientToken),
					VolumeMounts: []corev1.VolumeMount{
						{Name: "etc-netclient", MountPath: "/etc/netclient"},
						{Name: "log-netclient", MountPath: "/var/log"},
					},
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{"NET_ADMIN", "SYS_MODULE"},
						},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
				// TCP proxy container using socat (much simpler than nginx)
				{
					Name:    "proxy",
					Image:   proxyImage,
					Ports:   buildProxyPorts(service.Spec.Ports),
					Command: buildSocatCommand(targetIP, targetDNS, service.Spec.Ports),
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("32Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("10m"),
							corev1.ResourceMemory: resource.MustParse("16Mi"),
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "etc-netclient", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "log-netclient", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory}}},
			},
		},
	}

	return pod
}

// buildSocatCommand creates socat command for TCP forwarding
// Uses Service spec's targetPort for each port (standard Kubernetes way)
// For multiple ports, we use a shell script that runs multiple socat processes
func buildSocatCommand(targetIP, targetDNS string, servicePorts []corev1.ServicePort) []string {
	targetAddr := targetIP
	if targetDNS != "" {
		targetAddr = targetDNS
	}

	// Build socat commands for each port
	commands := []string{"/bin/sh", "-c"}
	socatCmds := ""

	for _, port := range servicePorts {
		// The Service routes traffic to the pod on targetPort, so socat must listen on targetPort
		// Then forward to the Netmaker device on the same port (targetPort)
		listenPort := port.Port   // Default to Service port
		netmakerPort := port.Port // Default to Service port

		// Use targetPort from Service spec - this is what the pod listens on
		if port.TargetPort.IntVal != 0 {
			listenPort = port.TargetPort.IntVal
			netmakerPort = port.TargetPort.IntVal // Forward to same port on Netmaker device
		} else if port.TargetPort.StrVal != "" {
			// If targetPort is a name, we can't resolve it here, so use Service port
			// In practice, targetPort names are usually the same as port numbers for egress
			listenPort = port.Port
			netmakerPort = port.Port
		}

		// socat listens on targetPort (what Service routes to) and forwards to Netmaker device
		// Format: TCP-LISTEN:listenPort -> TCP:target:netmakerPort
		socatCmds += fmt.Sprintf("socat TCP-LISTEN:%d,fork,reuseaddr TCP:%s:%d &\n", listenPort, targetAddr, netmakerPort)
	}

	// Wait for all background processes
	socatCmds += "wait\n"

	commands = append(commands, socatCmds)
	return commands
}

// buildProxyPorts creates container ports from service ports
// Container ports must match what socat listens on (targetPort from Service spec)
func buildProxyPorts(servicePorts []corev1.ServicePort) []corev1.ContainerPort {
	ports := make([]corev1.ContainerPort, 0, len(servicePorts))
	for _, port := range servicePorts {
		containerPort := port.Port // Default to Service port
		// Use targetPort if specified (this is what the pod listens on)
		if port.TargetPort.IntVal != 0 {
			containerPort = port.TargetPort.IntVal
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: containerPort,
			Protocol:      port.Protocol,
		})
	}
	return ports
}

// needsUpdate checks if pod needs to be updated
func needsUpdate(pod *corev1.Pod, targetIP, targetDNS string) bool {
	// Simple check - in production, you'd want more sophisticated comparison
	// For now, we'll recreate if target changes
	return false // Simplified - always return false to avoid unnecessary updates
}

// updateProxyPod updates an existing proxy pod
func (r *EgressProxyReconciler) updateProxyPod(ctx context.Context, pod *corev1.Pod, service *corev1.Service, targetIP, targetDNS string) error {
	// For simplicity, delete and recreate
	if err := r.Delete(ctx, pod); err != nil {
		return err
	}
	return r.ensureProxyPod(ctx, service, targetIP, targetDNS)
}

// updateServiceEndpoints updates Service endpoints to point to proxy pod
func (r *EgressProxyReconciler) updateServiceEndpoints(ctx context.Context, service *corev1.Service) error {
	podName := fmt.Sprintf("%s-egress-proxy", service.Name)

	// Get the proxy pod
	pod := &corev1.Pod{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      podName,
		Namespace: service.Namespace,
	}, pod); err != nil {
		if errors.IsNotFound(err) {
			// Pod not ready yet, skip endpoint update
			return nil
		}
		return err
	}

	// Check if pod is ready
	if pod.Status.Phase != corev1.PodRunning {
		return nil // Pod not ready yet
	}

	// Get or create Endpoints
	endpoints := &corev1.Endpoints{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      service.Name,
		Namespace: service.Namespace,
	}, endpoints)

	createEndpoints := errors.IsNotFound(err)
	if err != nil && !createEndpoints {
		return err
	}

	// Build endpoint addresses and ports
	addresses := []corev1.EndpointAddress{
		{
			IP: pod.Status.PodIP,
			TargetRef: &corev1.ObjectReference{
				Kind:      "Pod",
				Namespace: pod.Namespace,
				Name:      pod.Name,
				UID:       pod.UID,
			},
		},
	}

	ports := make([]corev1.EndpointPort, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		ports = append(ports, corev1.EndpointPort{
			Name:     port.Name,
			Port:     port.Port,
			Protocol: port.Protocol,
		})
	}

	if createEndpoints {
		endpoints = &corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      service.Name,
				Namespace: service.Namespace,
			},
			Subsets: []corev1.EndpointSubset{
				{
					Addresses: addresses,
					Ports:     ports,
				},
			},
		}
		return r.Create(ctx, endpoints)
	}

	// Update existing endpoints
	endpoints.Subsets = []corev1.EndpointSubset{
		{
			Addresses: addresses,
			Ports:     ports,
		},
	}
	return r.Update(ctx, endpoints)
}

// cleanupProxyPod removes the proxy pod when service is deleted or egress is disabled
func (r *EgressProxyReconciler) cleanupProxyPod(ctx context.Context, namespacedName types.NamespacedName) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	podName := fmt.Sprintf("%s-egress-proxy", namespacedName.Name)

	pod := &corev1.Pod{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      podName,
		Namespace: namespacedName.Namespace,
	}, pod); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Deleting egress proxy pod", "pod", podName)
	if err := r.Delete(ctx, pod); err != nil {
		return ctrl.Result{}, err
	}

	// No ConfigMap to clean up with socat approach

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *EgressProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}

// getNetclientToken gets the netclient token from secret
// First tries to get secret from service's namespace, then falls back to operator namespace
func (r *EgressProxyReconciler) getNetclientToken(ctx context.Context, service *corev1.Service) string {
	logger := log.FromContext(ctx)
	// Get secret configuration from Service annotations or environment variables
	secretName := r.getSecretNameFromService(service)
	secretKey := r.getSecretKeyFromService(service)
	operatorNamespace := getEnvOrDefault("OPERATOR_NAMESPACE", "netmaker-k8s-ops-system")

	secret := &corev1.Secret{}

	// First, try to get secret from service's namespace
	secretNamespacedName := types.NamespacedName{
		Name:      secretName,
		Namespace: service.Namespace,
	}

	if err := r.Get(ctx, secretNamespacedName, secret); err == nil {
		if tokenBytes, exists := secret.Data[secretKey]; exists {
			logger.Info("Found netclient token in service namespace", "namespace", service.Namespace, "secret", secretName)
			return string(tokenBytes)
		}
	}

	// Fallback: try operator namespace
	secretNamespacedName = types.NamespacedName{
		Name:      secretName,
		Namespace: operatorNamespace,
	}

	if err := r.Get(ctx, secretNamespacedName, secret); err == nil {
		if tokenBytes, exists := secret.Data[secretKey]; exists {
			logger.Info("Found netclient token in operator namespace (fallback)", "namespace", operatorNamespace, "secret", secretName)
			return string(tokenBytes)
		}
	}

	// Secret not found in either namespace - return empty (will fail at pod creation)
	logger.Info("Netclient token secret not found in service or operator namespace", "serviceNamespace", service.Namespace, "operatorNamespace", operatorNamespace, "secret", secretName)
	return ""
}

// getSecretNameFromService gets the secret name from Service annotations or uses default
// Default secret name is "netclient-token" in operator namespace
func (r *EgressProxyReconciler) getSecretNameFromService(service *corev1.Service) string {
	if service.Annotations != nil {
		if secretName, exists := service.Annotations["netmaker.io/secret-name"]; exists && secretName != "" {
			return secretName
		}
	}
	// Default secret name
	return getEnvOrDefault("NETCLIENT_SECRET_NAME", "netclient-token")
}

// getSecretKeyFromService gets the secret key from Service annotations or environment variable
func (r *EgressProxyReconciler) getSecretKeyFromService(service *corev1.Service) string {
	if service.Annotations != nil {
		if secretKey, exists := service.Annotations["netmaker.io/secret-key"]; exists && secretKey != "" {
			return secretKey
		}
	}
	return getEnvOrDefault("NETCLIENT_SECRET_KEY", "token")
}

// getSecretNamespaceFromService gets the secret namespace - deprecated, kept for backward compatibility
// The fallback logic is now handled in getNetclientToken and buildNetclientEnvVars
func (r *EgressProxyReconciler) getSecretNamespaceFromService(service *corev1.Service) string {
	// This method is kept for backward compatibility but is no longer used
	// The actual secret lookup now tries service namespace first, then operator namespace
	operatorNamespace := getEnvOrDefault("OPERATOR_NAMESPACE", "netmaker-k8s-ops-system")
	return operatorNamespace
}

// buildNetclientEnvVars builds environment variables for netclient container
// First tries to use secret from service's namespace, then falls back to operator namespace
func (r *EgressProxyReconciler) buildNetclientEnvVars(ctx context.Context, service *corev1.Service, tokenValue string) []corev1.EnvVar {
	logger := log.FromContext(ctx)
	// Get secret configuration from Service annotations or environment variables
	secretName := r.getSecretNameFromService(service)
	secretKey := r.getSecretKeyFromService(service)
	operatorNamespace := getEnvOrDefault("OPERATOR_NAMESPACE", "netmaker-k8s-ops-system")

	envVars := []corev1.EnvVar{
		{Name: "DAEMON", Value: "on"},
		{Name: "LOG_LEVEL", Value: "info"},
	}

	secret := &corev1.Secret{}

	// First, try to use secret from service's namespace
	secretNamespacedName := types.NamespacedName{
		Name:      secretName,
		Namespace: service.Namespace,
	}

	if err := r.Get(ctx, secretNamespacedName, secret); err == nil {
		if _, exists := secret.Data[secretKey]; exists {
			logger.Info("Using netclient token secret from service namespace", "namespace", service.Namespace, "secret", secretName)
			// Use secret reference from service namespace
			envVars = append(envVars, corev1.EnvVar{
				Name: "TOKEN",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secretName,
						},
						Key: secretKey,
					},
				},
			})
			return envVars
		}
	}

	// Fallback: if tokenValue was provided (from operator namespace), use it directly
	// Note: We can't use SecretKeyRef for cross-namespace secrets, so we use the value directly
	if tokenValue != "" {
		logger.Info("Using netclient token from operator namespace (fallback, using direct value)", "namespace", operatorNamespace, "secret", secretName)
		envVars = append(envVars, corev1.EnvVar{
			Name:  "TOKEN",
			Value: tokenValue,
		})
		return envVars
	}

	// Secret not found in either namespace - use empty value (will cause netclient to fail, which is expected)
	logger.Info("Netclient token secret not found in service or operator namespace, using empty token", "serviceNamespace", service.Namespace, "operatorNamespace", operatorNamespace, "secret", secretName)
	envVars = append(envVars, corev1.EnvVar{
		Name:  "TOKEN",
		Value: "",
	})

	return envVars
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
