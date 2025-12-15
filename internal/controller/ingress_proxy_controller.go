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

// IngressProxyReconciler reconciles Services with ingress annotations
type IngressProxyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch

// Reconcile processes Service objects to create ingress proxy pods
func (r *IngressProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	// Check if ingress is enabled
	if !isIngressEnabled(service) {
		// Ingress not enabled, clean up any existing proxy pod
		return r.cleanupProxyPod(ctx, req.NamespacedName)
	}

	// Prevent both egress and ingress on the same service (they conflict)
	if service.Annotations != nil && service.Annotations["netmaker.io/egress"] == "enabled" {
		logger.Info("Service has both egress and ingress enabled, skipping ingress", "service", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// Create or update ingress proxy pod
	if err := r.ensureProxyPod(ctx, service); err != nil {
		logger.Error(err, "Failed to ensure ingress proxy pod", "service", req.NamespacedName)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// isIngressEnabled checks if ingress is enabled for the service
func isIngressEnabled(service *corev1.Service) bool {
	if service.Annotations == nil {
		return false
	}
	return service.Annotations["netmaker.io/ingress"] == "enabled"
}

// getIngressConfig extracts ingress configuration from service annotations
func getIngressConfig(service *corev1.Service) (bindIP, dnsName string) {
	if service.Annotations == nil {
		return "", ""
	}

	bindIP = service.Annotations["netmaker.io/ingress-bind-ip"]
	dnsName = service.Annotations["netmaker.io/ingress-dns-name"]

	return bindIP, dnsName
}

// ensureProxyPod creates or updates the ingress proxy pod
func (r *IngressProxyReconciler) ensureProxyPod(ctx context.Context, service *corev1.Service) error {
	logger := log.FromContext(ctx)
	podName := fmt.Sprintf("%s-ingress-proxy", service.Name)

	// Check if pod already exists
	existingPod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      podName,
		Namespace: service.Namespace,
	}, existingPod)

	if err == nil {
		// Pod exists, check if it needs update
		if needsIngressUpdate(existingPod, service) {
			logger.Info("Updating ingress proxy pod", "pod", podName)
			return r.updateProxyPod(ctx, existingPod, service)
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	// Create new pod
	logger.Info("Creating ingress proxy pod", "pod", podName, "service", service.Name)

	pod := r.buildProxyPod(ctx, service, podName)

	if err := r.Create(ctx, pod); err != nil {
		return fmt.Errorf("failed to create ingress proxy pod: %w", err)
	}

	return nil
}

// buildProxyPod builds the ingress proxy pod specification
func (r *IngressProxyReconciler) buildProxyPod(ctx context.Context, service *corev1.Service, podName string) *corev1.Pod {
	// Get configuration from environment or use defaults
	netclientImage := getEnvOrDefaultIngress("NETCLIENT_IMAGE", "gravitl/netclient:v1.2.0")
	// Try to get token from secret first (checks Service annotations), fallback to environment variable
	netclientToken := r.getNetclientToken(ctx, service)
	// Use socat for simple TCP forwarding
	proxyImage := getEnvOrDefaultIngress("INGRESS_PROXY_IMAGE", "alpine/socat:latest")

	bindIP, dnsName := getIngressConfig(service)

	// Build pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: service.Namespace,
			Labels: map[string]string{
				"app":                 "netmaker-ingress-proxy",
				"service-name":        service.Name,
				"managed-by":          "netmaker-k8s-ops",
				"netmaker.io/ingress": "enabled",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       service.Name,
					UID:        service.UID,
				},
			},
			Annotations: map[string]string{
				"netmaker.io/ingress-dns-name": dnsName,
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
				// TCP proxy container using socat
				// Listens on Netmaker network IP and forwards to Kubernetes Service
				// WireGuard IP is detected dynamically at runtime
				{
					Name:    "proxy",
					Image:   proxyImage,
					Ports:   buildIngressProxyPorts(service.Spec.Ports),
					Command: buildIngressSocatCommand(service, bindIP),
					Env: []corev1.EnvVar{
						{Name: "SERVICE_NAME", Value: service.Name},
						{Name: "SERVICE_NAMESPACE", Value: service.Namespace},
					},
					// Share netclient's network namespace to access WireGuard interface
					// Both containers run in the same pod, so they share network namespace by default
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
					// Add readiness probe to ensure WireGuard IP is detected before marking ready
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{
									"/bin/sh",
									"-c",
									"ip addr show | grep -E 'inet.*(10\\.|172\\.(1[6-9]|2[0-9]|3[01])\\.|192\\.168\\.)' | grep -v '127.0.0.1' || exit 1",
								},
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       5,
						TimeoutSeconds:      2,
						FailureThreshold:    3,
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

// buildIngressSocatCommand creates socat command for ingress proxying
// Listens on Netmaker network IP and forwards to Kubernetes Service
func buildIngressSocatCommand(service *corev1.Service, bindIP string) []string {
	serviceAddr := fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace)
	commands := []string{"/bin/sh", "-c"}
	socatCmds := ""

	// Wait for netclient to establish WireGuard connection and get IP dynamically
	// WireGuard IP is assigned dynamically by Netmaker, so we detect it at runtime
	if bindIP == "" {
		socatCmds += `# Wait for WireGuard interface to be ready and get dynamic IP
# Try common WireGuard interface names: netmaker, wg0, wg1, etc.
# WireGuard interfaces may show as state UNKNOWN or UP, so check for interface existence and IP presence
WG_INTERFACE=""
WG_IP=""

# First, try to find interface by name and get its IP
for iface in netmaker wg0 wg1; do
  if ip link show "$iface" 2>/dev/null >/dev/null; then
    # Interface exists, check if it has an IP
    WG_IP=$(ip addr show "$iface" 2>/dev/null | grep "inet " | awk '{print $2}' | cut -d/ -f1 | head -n1)
    if [ -n "$WG_IP" ]; then
      WG_INTERFACE="$iface"
      echo "Found WireGuard IP: $WG_IP on interface $WG_INTERFACE"
      break
    fi
  fi
done

# If no interface found with IP, wait and retry (netclient might still be starting)
if [ -z "$WG_INTERFACE" ]; then
  echo "Waiting for WireGuard interface to be created and get IP..."
  for i in $(seq 1 60); do
    for iface in netmaker wg0 wg1; do
      if ip link show "$iface" 2>/dev/null >/dev/null; then
        WG_IP=$(ip addr show "$iface" 2>/dev/null | grep "inet " | awk '{print $2}' | cut -d/ -f1 | head -n1)
        if [ -n "$WG_IP" ]; then
          WG_INTERFACE="$iface"
          echo "Found WireGuard IP: $WG_IP on interface $WG_INTERFACE"
          break 2
        fi
      fi
    done
    sleep 1
  done
fi

# Fallback: if still no IP, look for POINTOPOINT interfaces (WireGuard uses POINTOPOINT)
# Exclude eth0 and other non-WireGuard interfaces
if [ -z "$WG_IP" ]; then
  echo "Trying to detect WireGuard IP from POINTOPOINT interfaces..."
  # Find all POINTOPOINT interfaces (WireGuard characteristic)
  for iface in $(ip link show | grep -B 1 "POINTOPOINT" | grep "^[0-9]" | awk '{print $2}' | cut -d: -f1 | sed 's/@.*//' | grep -v "^lo$" | grep -v "^eth"); do
    if ip link show "$iface" 2>/dev/null >/dev/null; then
      WG_IP=$(ip addr show "$iface" 2>/dev/null | grep "inet " | awk '{print $2}' | cut -d/ -f1 | head -n1)
      if [ -n "$WG_IP" ]; then
        WG_INTERFACE="$iface"
        echo "Found WireGuard IP: $WG_IP on POINTOPOINT interface $WG_INTERFACE"
        break
      fi
    fi
  done
fi

# Final fallback: bind to all interfaces (less secure but ensures connectivity)
if [ -z "$WG_IP" ]; then
  echo "Warning: Could not detect WireGuard IP dynamically, binding to all interfaces (0.0.0.0)"
  echo "This may expose the service on all network interfaces. Consider setting netmaker.io/ingress-bind-ip annotation."
  WG_IP="0.0.0.0"
fi

echo "Using WireGuard IP: $WG_IP"
`
	} else {
		socatCmds += fmt.Sprintf("WG_IP=%s\n", bindIP)
		socatCmds += "echo \"Using configured bind IP: $WG_IP\"\n"
	}

	// Build socat commands for each port
	for _, port := range service.Spec.Ports {
		servicePort := port.Port
		// Forward to Service port (Service will route to pods via targetPort)
		socatCmds += fmt.Sprintf("socat TCP-LISTEN:%d,bind=$WG_IP,fork,reuseaddr TCP:%s:%d &\n", servicePort, serviceAddr, servicePort)
	}

	// Wait for all background processes
	socatCmds += "wait\n"

	commands = append(commands, socatCmds)
	return commands
}

// buildIngressProxyPorts creates container ports from service ports
func buildIngressProxyPorts(servicePorts []corev1.ServicePort) []corev1.ContainerPort {
	ports := make([]corev1.ContainerPort, 0, len(servicePorts))
	for _, port := range servicePorts {
		ports = append(ports, corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.Port,
			Protocol:      port.Protocol,
		})
	}
	return ports
}

// needsIngressUpdate checks if pod needs to be updated
func needsIngressUpdate(pod *corev1.Pod, service *corev1.Service) bool {
	// Simple check - in production, you'd want more sophisticated comparison
	return false // Simplified - always return false to avoid unnecessary updates
}

// updateProxyPod updates an existing proxy pod
func (r *IngressProxyReconciler) updateProxyPod(ctx context.Context, pod *corev1.Pod, service *corev1.Service) error {
	// For simplicity, delete and recreate
	if err := r.Delete(ctx, pod); err != nil {
		return err
	}
	return r.ensureProxyPod(ctx, service)
}

// cleanupProxyPod removes the proxy pod when service is deleted or ingress is disabled
func (r *IngressProxyReconciler) cleanupProxyPod(ctx context.Context, namespacedName types.NamespacedName) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	podName := fmt.Sprintf("%s-ingress-proxy", namespacedName.Name)

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

	logger.Info("Deleting ingress proxy pod", "pod", podName)
	if err := r.Delete(ctx, pod); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *IngressProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}

// getNetclientToken gets the netclient token from secret
// Checks Service annotations first, then environment variables for secret name/key
func (r *IngressProxyReconciler) getNetclientToken(ctx context.Context, service *corev1.Service) string {
	// Get secret configuration from Service annotations or environment variables
	secretName := r.getSecretNameFromService(service)
	secretKey := r.getSecretKeyFromService(service)
	secretNamespace := r.getSecretNamespaceFromService(service)

	secret := &corev1.Secret{}
	secretNamespacedName := types.NamespacedName{
		Name:      secretName,
		Namespace: secretNamespace,
	}

	if err := r.Get(ctx, secretNamespacedName, secret); err == nil {
		if tokenBytes, exists := secret.Data[secretKey]; exists {
			return string(tokenBytes)
		}
	}

	// Secret not found or key doesn't exist - return empty (will fail at pod creation)
	return ""
}

// getSecretNameFromService gets the secret name from Service annotations or environment variable
func (r *IngressProxyReconciler) getSecretNameFromService(service *corev1.Service) string {
	if service.Annotations != nil {
		if secretName, exists := service.Annotations["netmaker.io/secret-name"]; exists && secretName != "" {
			return secretName
		}
	}
	return getEnvOrDefaultIngress("NETCLIENT_SECRET_NAME", "netclient-token")
}

// getSecretKeyFromService gets the secret key from Service annotations or environment variable
func (r *IngressProxyReconciler) getSecretKeyFromService(service *corev1.Service) string {
	if service.Annotations != nil {
		if secretKey, exists := service.Annotations["netmaker.io/secret-key"]; exists && secretKey != "" {
			return secretKey
		}
	}
	return getEnvOrDefaultIngress("NETCLIENT_SECRET_KEY", "token")
}

// getSecretNamespaceFromService gets the secret namespace - always uses operator namespace for security
// Secrets can only be read from the operator namespace
func (r *IngressProxyReconciler) getSecretNamespaceFromService(service *corev1.Service) string {
	// Always use operator namespace - ignore any namespace specified in annotations for security
	operatorNamespace := getEnvOrDefaultIngress("OPERATOR_NAMESPACE", "netmaker-k8s-ops-system")
	return operatorNamespace
}

// buildNetclientEnvVars builds environment variables for netclient container
// Uses secret if available, otherwise falls back to direct value
// Checks Service annotations first for secret configuration
func (r *IngressProxyReconciler) buildNetclientEnvVars(ctx context.Context, service *corev1.Service, tokenValue string) []corev1.EnvVar {
	// Get secret configuration from Service annotations or environment variables
	secretName := r.getSecretNameFromService(service)
	secretKey := r.getSecretKeyFromService(service)
	secretNamespace := r.getSecretNamespaceFromService(service)

	// Check if secret exists
	secret := &corev1.Secret{}
	secretNamespacedName := types.NamespacedName{
		Name:      secretName,
		Namespace: secretNamespace,
	}

	envVars := []corev1.EnvVar{
		{Name: "DAEMON", Value: "on"},
		{Name: "LOG_LEVEL", Value: "info"},
	}

	// Try to use secret if it exists
	if err := r.Get(ctx, secretNamespacedName, secret); err == nil {
		if _, exists := secret.Data[secretKey]; exists {
			// Use secret reference
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

	// Secret not found - use empty value (will cause netclient to fail, which is expected)
	envVars = append(envVars, corev1.EnvVar{
		Name:  "TOKEN",
		Value: "",
	})

	return envVars
}

// getEnvOrDefaultIngress gets an environment variable or returns a default value
func getEnvOrDefaultIngress(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
