package webhook

import (
	"context"
	"fmt"
	"os"

	"gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NetclientSidecarWebhook handles mutating webhook requests
type NetclientSidecarWebhook struct {
	decoder admission.Decoder
	client  client.Client
}

// NewNetclientSidecarWebhook creates a new webhook
func NewNetclientSidecarWebhook() *NetclientSidecarWebhook {
	return &NetclientSidecarWebhook{}
}

// InjectDecoder injects the decoder
func (w *NetclientSidecarWebhook) InjectDecoder(d admission.Decoder) error {
	w.decoder = d
	return nil
}

// Handle handles the webhook request
func (w *NetclientSidecarWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Only handle pod creation/update
	if req.Kind.Kind != "Pod" {
		return admission.Allowed("not a pod")
	}

	// Check if decoder is available
	if w.decoder == nil {
		return admission.Errored(500, fmt.Errorf("decoder not initialized"))
	}

	// Decode the pod
	var pod corev1.Pod
	if err := w.decoder.Decode(req, &pod); err != nil {
		return admission.Errored(400, err)
	}

	// Check if pod has the netclient label
	if !hasNetclientLabel(pod.Labels) {
		return admission.Allowed("no netclient label")
	}

	// Check if netclient sidecar already exists
	if hasNetclientSidecar(pod.Spec.Containers) {
		return admission.Allowed("netclient sidecar already exists")
	}

	// Add netclient sidecar
	modifiedPod := pod.DeepCopy()
	w.addNetclientSidecar(modifiedPod)

	// Return the modified pod using admission.Patched with the modified pod
	return admission.Patched("netclient sidecar added", jsonpatch.Operation{
		Operation: "replace",
		Path:      "/spec",
		Value:     modifiedPod.Spec,
	})
}

// InjectClient injects the client
func (w *NetclientSidecarWebhook) InjectClient(c client.Client) error {
	w.client = c
	return nil
}

// hasNetclientLabel checks if the pod has the netclient label
func hasNetclientLabel(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	// Check for netclient label
	value, exists := labels["netmaker.io/netclient"]
	return exists && value == "enabled"
}

// hasNetclientSidecar checks if the pod already has a netclient sidecar
func hasNetclientSidecar(containers []corev1.Container) bool {
	for _, container := range containers {
		if container.Name == "netclient" {
			return true
		}
	}
	return false
}

// addNetclientSidecar adds the netclient sidecar to the pod
func (w *NetclientSidecarWebhook) addNetclientSidecar(pod *corev1.Pod) {
	// Get netclient configuration from environment variables or use defaults
	netclientImage := getEnvOrDefault("NETCLIENT_IMAGE", "gravitl/netclient:v1.1.0")
	netclientServer := getEnvOrDefault("NETCLIENT_SERVER", "")
	netclientNetwork := getEnvOrDefault("NETCLIENT_NETWORK", "")

	// Get netclient token from secret
	netclientToken, err := w.getNetclientTokenFromSecret(pod)
	if err != nil {
		klog.Warning("Failed to get netclient token from secret", "error", err)
		// Fallback to environment variable
		netclientToken = getEnvOrDefault("NETCLIENT_TOKEN", "")
	}

	if netclientToken == "" {
		klog.Warning("NETCLIENT_TOKEN not found in secret or environment, netclient sidecar may not work properly")
	}

	// Create netclient container
	netclientContainer := corev1.Container{
		Name:  "netclient",
		Image: netclientImage,
		Env: []corev1.EnvVar{
			{
				Name:  "TOKEN",
				Value: netclientToken,
			},
			{
				Name:  "SERVER",
				Value: netclientServer,
			},
			{
				Name:  "NETWORK",
				Value: netclientNetwork,
			},
			{
				Name:  "DAEMON",
				Value: "on",
			},
			{
				Name:  "LOG_LEVEL",
				Value: "info",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "etc-netclient",
				MountPath: "/etc/netclient",
			},
			{
				Name:      "log-netclient",
				MountPath: "/var/log",
			},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: &[]bool{true}[0],
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"NET_ADMIN",
					"SYS_MODULE",
				},
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
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"/bin/sh",
						"-c",
						"ip link show netmaker && ip addr show netmaker | grep inet",
					},
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       5,
			TimeoutSeconds:      3,
			FailureThreshold:    12,
		},
	}

	// Add netclient container to pod
	pod.Spec.Containers = append(pod.Spec.Containers, netclientContainer)

	// Add required volumes if they don't exist
	addNetclientVolumes(pod)

	// Set hostNetwork to true for WireGuard connectivity
	pod.Spec.HostNetwork = true
}

// addNetclientVolumes adds the required volumes for netclient
func addNetclientVolumes(pod *corev1.Pod) {
	// Check if volumes already exist
	hasEtcNetclient := false
	hasLogNetclient := false

	for _, volume := range pod.Spec.Volumes {
		if volume.Name == "etc-netclient" {
			hasEtcNetclient = true
		}
		if volume.Name == "log-netclient" {
			hasLogNetclient = true
		}
	}

	// Add etc-netclient volume
	if !hasEtcNetclient {
		etcVolume := corev1.Volume{
			Name: "etc-netclient",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/netclient",
					Type: &[]corev1.HostPathType{corev1.HostPathDirectoryOrCreate}[0],
				},
			},
		}
		pod.Spec.Volumes = append(pod.Spec.Volumes, etcVolume)
	}

	// Add log-netclient volume
	if !hasLogNetclient {
		logVolume := corev1.Volume{
			Name: "log-netclient",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumMemory,
				},
			},
		}
		pod.Spec.Volumes = append(pod.Spec.Volumes, logVolume)
	}
}

// getNetclientTokenFromSecret reads the netclient token from a Kubernetes secret
func (w *NetclientSidecarWebhook) getNetclientTokenFromSecret(pod *corev1.Pod) (string, error) {
	// Check if client is available
	if w.client == nil {
		return "", fmt.Errorf("client not initialized")
	}

	// Get secret configuration from pod labels or environment variables
	secretName := w.getSecretNameFromPod(pod)
	secretKey := w.getSecretKeyFromPod(pod)
	secretNamespace := w.getSecretNamespaceFromPod(pod)

	// Create secret object
	secret := &corev1.Secret{}
	secretNamespacedName := types.NamespacedName{
		Name:      secretName,
		Namespace: secretNamespace,
	}

	// Get the secret
	if err := w.client.Get(context.Background(), secretNamespacedName, secret); err != nil {
		return "", err
	}

	// Extract the token
	tokenBytes, exists := secret.Data[secretKey]
	if !exists {
		return "", fmt.Errorf("key %s not found in secret %s in namespace %s", secretKey, secretName, secretNamespace)
	}

	return string(tokenBytes), nil
}

// getSecretNameFromPod gets the secret name from pod labels or environment variable
func (w *NetclientSidecarWebhook) getSecretNameFromPod(pod *corev1.Pod) string {
	// Check if pod has custom secret name label
	if pod.Labels != nil {
		if secretName, exists := pod.Labels["netmaker.io/secret-name"]; exists && secretName != "" {
			return secretName
		}
	}

	// Fallback to environment variable or default
	return getEnvOrDefault("NETCLIENT_SECRET_NAME", "netclient-token")
}

// getSecretKeyFromPod gets the secret key from pod labels or environment variable
func (w *NetclientSidecarWebhook) getSecretKeyFromPod(pod *corev1.Pod) string {
	// Check if pod has custom secret key label
	if pod.Labels != nil {
		if secretKey, exists := pod.Labels["netmaker.io/secret-key"]; exists && secretKey != "" {
			return secretKey
		}
	}

	// Fallback to environment variable or default
	return getEnvOrDefault("NETCLIENT_SECRET_KEY", "token")
}

// getSecretNamespaceFromPod gets the secret namespace from pod labels or uses pod namespace
func (w *NetclientSidecarWebhook) getSecretNamespaceFromPod(pod *corev1.Pod) string {
	// Check if pod has custom secret namespace label
	if pod.Labels != nil {
		if secretNamespace, exists := pod.Labels["netmaker.io/secret-namespace"]; exists && secretNamespace != "" {
			return secretNamespace
		}
	}

	// Fallback to pod's namespace
	return pod.Namespace
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
