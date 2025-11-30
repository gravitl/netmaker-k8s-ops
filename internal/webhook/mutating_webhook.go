package webhook

import (
	"context"
	"fmt"
	"os"

	"gomodules.xyz/jsonpatch/v2"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
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

// Handle handles the webhook request for Pods
func (w *NetclientSidecarWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Check if decoder is available
	if w.decoder == nil {
		return admission.Errored(500, fmt.Errorf("decoder not initialized"))
	}

	// Route to appropriate handler based on resource type
	switch req.Kind.Kind {
	case "Pod":
		return w.handlePod(ctx, req)
	case "Deployment":
		return w.handleDeployment(ctx, req)
	case "StatefulSet":
		return w.handleStatefulSet(ctx, req)
	case "DaemonSet":
		return w.handleDaemonSet(ctx, req)
	case "Job":
		return w.handleJob(ctx, req)
	case "ReplicaSet":
		return w.handleReplicaSet(ctx, req)
	default:
		return admission.Allowed(fmt.Sprintf("resource type %s not supported", req.Kind.Kind))
	}
}

// handlePod handles Pod webhook requests
func (w *NetclientSidecarWebhook) handlePod(ctx context.Context, req admission.Request) admission.Response {
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
	w.addNetclientSidecar(modifiedPod, pod.Labels, pod.Annotations, req.Namespace)

	// Return the modified pod
	return admission.Patched("netclient sidecar added", jsonpatch.Operation{
		Operation: "replace",
		Path:      "/spec",
		Value:     modifiedPod.Spec,
	})
}

// handleDeployment handles Deployment webhook requests
func (w *NetclientSidecarWebhook) handleDeployment(ctx context.Context, req admission.Request) admission.Response {
	var deployment appsv1.Deployment
	if err := w.decoder.Decode(req, &deployment); err != nil {
		return admission.Errored(400, err)
	}

	// Check if deployment has the netclient label
	if !hasNetclientLabel(deployment.Labels) && !hasNetclientLabel(deployment.Spec.Template.Labels) {
		return admission.Allowed("no netclient label")
	}

	// Check if netclient sidecar already exists
	if hasNetclientSidecar(deployment.Spec.Template.Spec.Containers) {
		return admission.Allowed("netclient sidecar already exists")
	}

	// Add netclient sidecar to pod template
	modifiedDeployment := deployment.DeepCopy()
	w.addNetclientSidecarToPodTemplate(&modifiedDeployment.Spec.Template.Spec, deployment.Labels, deployment.Annotations, req.Namespace)

	return admission.Patched("netclient sidecar added to deployment", jsonpatch.Operation{
		Operation: "replace",
		Path:      "/spec",
		Value:     modifiedDeployment.Spec,
	})
}

// handleStatefulSet handles StatefulSet webhook requests
func (w *NetclientSidecarWebhook) handleStatefulSet(ctx context.Context, req admission.Request) admission.Response {
	var statefulSet appsv1.StatefulSet
	if err := w.decoder.Decode(req, &statefulSet); err != nil {
		return admission.Errored(400, err)
	}

	// Check if statefulset has the netclient label
	if !hasNetclientLabel(statefulSet.Labels) && !hasNetclientLabel(statefulSet.Spec.Template.Labels) {
		return admission.Allowed("no netclient label")
	}

	// Check if netclient sidecar already exists
	if hasNetclientSidecar(statefulSet.Spec.Template.Spec.Containers) {
		return admission.Allowed("netclient sidecar already exists")
	}

	// Add netclient sidecar to pod template
	modifiedStatefulSet := statefulSet.DeepCopy()
	w.addNetclientSidecarToPodTemplate(&modifiedStatefulSet.Spec.Template.Spec, statefulSet.Labels, statefulSet.Annotations, req.Namespace)

	return admission.Patched("netclient sidecar added to statefulset", jsonpatch.Operation{
		Operation: "replace",
		Path:      "/spec",
		Value:     modifiedStatefulSet.Spec,
	})
}

// handleDaemonSet handles DaemonSet webhook requests
func (w *NetclientSidecarWebhook) handleDaemonSet(ctx context.Context, req admission.Request) admission.Response {
	var daemonSet appsv1.DaemonSet
	if err := w.decoder.Decode(req, &daemonSet); err != nil {
		return admission.Errored(400, err)
	}

	// Check if daemonset has the netclient label
	if !hasNetclientLabel(daemonSet.Labels) && !hasNetclientLabel(daemonSet.Spec.Template.Labels) {
		return admission.Allowed("no netclient label")
	}

	// Check if netclient sidecar already exists
	if hasNetclientSidecar(daemonSet.Spec.Template.Spec.Containers) {
		return admission.Allowed("netclient sidecar already exists")
	}

	// Add netclient sidecar to pod template
	modifiedDaemonSet := daemonSet.DeepCopy()
	w.addNetclientSidecarToPodTemplate(&modifiedDaemonSet.Spec.Template.Spec, daemonSet.Labels, daemonSet.Annotations, req.Namespace)

	return admission.Patched("netclient sidecar added to daemonset", jsonpatch.Operation{
		Operation: "replace",
		Path:      "/spec",
		Value:     modifiedDaemonSet.Spec,
	})
}

// handleJob handles Job webhook requests
func (w *NetclientSidecarWebhook) handleJob(ctx context.Context, req admission.Request) admission.Response {
	var job batchv1.Job
	if err := w.decoder.Decode(req, &job); err != nil {
		return admission.Errored(400, err)
	}

	// Check if job has the netclient label
	if !hasNetclientLabel(job.Labels) && !hasNetclientLabel(job.Spec.Template.Labels) {
		return admission.Allowed("no netclient label")
	}

	// Check if netclient sidecar already exists
	if hasNetclientSidecar(job.Spec.Template.Spec.Containers) {
		return admission.Allowed("netclient sidecar already exists")
	}

	// Add netclient sidecar to pod template
	modifiedJob := job.DeepCopy()
	w.addNetclientSidecarToPodTemplate(&modifiedJob.Spec.Template.Spec, job.Labels, job.Annotations, req.Namespace)

	return admission.Patched("netclient sidecar added to job", jsonpatch.Operation{
		Operation: "replace",
		Path:      "/spec",
		Value:     modifiedJob.Spec,
	})
}

// handleReplicaSet handles ReplicaSet webhook requests
func (w *NetclientSidecarWebhook) handleReplicaSet(ctx context.Context, req admission.Request) admission.Response {
	var replicaSet appsv1.ReplicaSet
	if err := w.decoder.Decode(req, &replicaSet); err != nil {
		return admission.Errored(400, err)
	}

	// Check if replicaset has the netclient label
	if !hasNetclientLabel(replicaSet.Labels) && !hasNetclientLabel(replicaSet.Spec.Template.Labels) {
		return admission.Allowed("no netclient label")
	}

	// Check if netclient sidecar already exists
	if hasNetclientSidecar(replicaSet.Spec.Template.Spec.Containers) {
		return admission.Allowed("netclient sidecar already exists")
	}

	// Add netclient sidecar to pod template
	modifiedReplicaSet := replicaSet.DeepCopy()
	w.addNetclientSidecarToPodTemplate(&modifiedReplicaSet.Spec.Template.Spec, replicaSet.Labels, replicaSet.Annotations, req.Namespace)

	return admission.Patched("netclient sidecar added to replicaset", jsonpatch.Operation{
		Operation: "replace",
		Path:      "/spec",
		Value:     modifiedReplicaSet.Spec,
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
func (w *NetclientSidecarWebhook) addNetclientSidecar(pod *corev1.Pod, labels map[string]string, annotations map[string]string, namespace string) {
	w.addNetclientSidecarToPodTemplate(&pod.Spec, labels, annotations, namespace)
}

// addNetclientSidecarToPodTemplate adds the netclient sidecar to a pod template spec
func (w *NetclientSidecarWebhook) addNetclientSidecarToPodTemplate(podSpec *corev1.PodSpec, labels map[string]string, annotations map[string]string, namespace string) {
	// Get netclient configuration from environment variables or use defaults
	netclientImage := getEnvOrDefault("NETCLIENT_IMAGE", "gravitl/netclient:v1.1.0")
	netclientServer := getEnvOrDefault("NETCLIENT_SERVER", "")
	netclientNetwork := getEnvOrDefault("NETCLIENT_NETWORK", "")

	// Get netclient token from secret (create a temporary pod object for secret lookup)
	// We'll use labels and annotations directly in the getNetclientTokenFromSecret call
	// by creating a minimal pod structure
	tempPod := &corev1.Pod{}
	tempPod.Namespace = namespace
	if labels != nil {
		tempPod.Labels = labels
	}
	if annotations != nil {
		tempPod.Annotations = annotations
	}
	netclientToken, err := w.getNetclientTokenFromSecret(tempPod)
	if err != nil {
		klog.Warning("Failed to get netclient token from secret", "error", err)
		// Fallback to environment variable
		netclientToken = getEnvOrDefault("NETCLIENT_TOKEN", "")
	}

	if netclientToken == "" {
		klog.Warning("NETCLIENT_TOKEN not found in secret or environment, netclient sidecar may not work properly")
	}

	// Build environment variables
	envVars := []corev1.EnvVar{
		{
			Name:  "TOKEN",
			Value: netclientToken,
		},
		{
			Name:  "DAEMON",
			Value: "on",
		},
		{
			Name:  "LOG_LEVEL",
			Value: "info",
		},
	}

	// Add server and network if provided
	if netclientServer != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "SERVER",
			Value: netclientServer,
		})
	}
	if netclientNetwork != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "NETWORK",
			Value: netclientNetwork,
		})
	}

	// Create netclient container
	netclientContainer := corev1.Container{
		Name:  "netclient",
		Image: netclientImage,
		Env:   envVars,
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

	// Add netclient container to pod spec
	podSpec.Containers = append(podSpec.Containers, netclientContainer)

	// Add required volumes if they don't exist
	addNetclientVolumesToPodSpec(podSpec)

	// Note: hostNetwork is not required since containers in a pod share the network namespace.
	// The WireGuard interface created by netclient will be accessible to all containers in the pod.
}

// addNetclientVolumes adds the required volumes for netclient (for Pod objects)
func addNetclientVolumes(pod *corev1.Pod) {
	addNetclientVolumesToPodSpec(&pod.Spec)
}

// addNetclientVolumesToPodSpec adds the required volumes for netclient to a pod spec
func addNetclientVolumesToPodSpec(podSpec *corev1.PodSpec) {
	// Check if volumes already exist
	hasEtcNetclient := false
	hasLogNetclient := false

	for _, volume := range podSpec.Volumes {
		if volume.Name == "etc-netclient" {
			hasEtcNetclient = true
		}
		if volume.Name == "log-netclient" {
			hasLogNetclient = true
		}
	}

	// Add etc-netclient volume
	// Use EmptyDir instead of HostPath to ensure each pod gets its own isolated
	// configuration directory. This prevents multiple netclient instances from
	// sharing the same identity and IP address.
	if !hasEtcNetclient {
		etcVolume := corev1.Volume{
			Name: "etc-netclient",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
		podSpec.Volumes = append(podSpec.Volumes, etcVolume)
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
		podSpec.Volumes = append(podSpec.Volumes, logVolume)
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
