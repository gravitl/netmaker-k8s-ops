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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// Merge annotations: pod template annotations take priority over deployment annotations
	mergedAnnotations := mergeAnnotations(deployment.Annotations, deployment.Spec.Template.Annotations)
	mergedLabels := mergeLabels(deployment.Labels, deployment.Spec.Template.Labels)
	klog.Info("Processing deployment for netclient injection",
		"deployment", deployment.Name,
		"namespace", req.Namespace,
		"deploymentAnnotations", deployment.Annotations,
		"podTemplateAnnotations", deployment.Spec.Template.Annotations,
		"mergedAnnotations", mergedAnnotations)
	w.addNetclientSidecarToPodTemplate(&modifiedDeployment.Spec.Template.Spec, mergedLabels, mergedAnnotations, req.Namespace)

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
	// Merge annotations: pod template annotations take priority over statefulset annotations
	mergedAnnotations := mergeAnnotations(statefulSet.Annotations, statefulSet.Spec.Template.Annotations)
	mergedLabels := mergeLabels(statefulSet.Labels, statefulSet.Spec.Template.Labels)
	w.addNetclientSidecarToPodTemplate(&modifiedStatefulSet.Spec.Template.Spec, mergedLabels, mergedAnnotations, req.Namespace)

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
	// Merge annotations: pod template annotations take priority over daemonset annotations
	mergedAnnotations := mergeAnnotations(daemonSet.Annotations, daemonSet.Spec.Template.Annotations)
	mergedLabels := mergeLabels(daemonSet.Labels, daemonSet.Spec.Template.Labels)
	w.addNetclientSidecarToPodTemplate(&modifiedDaemonSet.Spec.Template.Spec, mergedLabels, mergedAnnotations, req.Namespace)

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
	// Merge annotations: pod template annotations take priority over job annotations
	mergedAnnotations := mergeAnnotations(job.Annotations, job.Spec.Template.Annotations)
	mergedLabels := mergeLabels(job.Labels, job.Spec.Template.Labels)
	w.addNetclientSidecarToPodTemplate(&modifiedJob.Spec.Template.Spec, mergedLabels, mergedAnnotations, req.Namespace)

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
	// Merge annotations: pod template annotations take priority over replicaset annotations
	mergedAnnotations := mergeAnnotations(replicaSet.Annotations, replicaSet.Spec.Template.Annotations)
	mergedLabels := mergeLabels(replicaSet.Labels, replicaSet.Spec.Template.Labels)
	w.addNetclientSidecarToPodTemplate(&modifiedReplicaSet.Spec.Template.Spec, mergedLabels, mergedAnnotations, req.Namespace)

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
			// Privileged: &[]bool{true}[0],
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
	w.addNetclientVolumesToPodSpec(podSpec, namespace, labels, annotations)

	// Note: hostNetwork is not required since containers in a pod share the network namespace.
	// The WireGuard interface created by netclient will be accessible to all containers in the pod.
}

// addNetclientVolumesToPodSpec adds the required volumes for netclient to a pod spec
func (w *NetclientSidecarWebhook) addNetclientVolumesToPodSpec(podSpec *corev1.PodSpec, namespace string, labels map[string]string, annotations map[string]string) {
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

	// Get PVC name from pod annotation, environment variable, or use EmptyDir
	pvcName := getPVCNameFromPod(annotations, namespace)

	// Debug logging
	if len(annotations) > 0 {
		klog.Info("Processing netclient volumes", "annotations", annotations, "pvcName", pvcName, "namespace", namespace)
	} else {
		klog.Info("No annotations found for netclient PVC configuration", "namespace", namespace)
	}

	// If PVC is specified, ensure it exists (create if it doesn't)
	if pvcName != "" && w.client != nil {
		if err := w.ensurePVCExists(pvcName, namespace); err != nil {
			klog.Error(err, "Failed to ensure PVC exists, falling back to EmptyDir", "pvc", pvcName, "namespace", namespace)
			pvcName = "" // Fall back to EmptyDir if PVC creation fails
		}
	}

	// Add etc-netclient volume
	// Use PersistentVolumeClaim if configured, otherwise use EmptyDir for backward compatibility
	// EmptyDir ensures each pod gets its own isolated configuration directory when PVC is not used.
	if !hasEtcNetclient {
		var etcVolume corev1.Volume
		if pvcName != "" {
			// Use PersistentVolumeClaim for persistent storage
			etcVolume = corev1.Volume{
				Name: "etc-netclient",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
						ReadOnly:  false,
					},
				},
			}
			klog.Info("Using PersistentVolumeClaim for netclient", "pvc", pvcName, "namespace", namespace)
		} else {
			// Fallback to EmptyDir for backward compatibility
			etcVolume = corev1.Volume{
				Name: "etc-netclient",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			}
			klog.Info("Using EmptyDir for netclient (no PVC configured)", "namespace", namespace)
		}
		podSpec.Volumes = append(podSpec.Volumes, etcVolume)
	} else {
		klog.Info("etc-netclient volume already exists, skipping", "namespace", namespace)
	}

	// Add log-netclient volume (always use EmptyDir with Memory medium for logs)
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

// ensurePVCExists ensures that the PVC exists, creating it if it doesn't
func (w *NetclientSidecarWebhook) ensurePVCExists(pvcName, namespace string) error {
	if w.client == nil {
		return fmt.Errorf("client not initialized")
	}

	// Check if PVC already exists
	pvc := &corev1.PersistentVolumeClaim{}
	namespacedName := types.NamespacedName{
		Name:      pvcName,
		Namespace: namespace,
	}

	err := w.client.Get(context.Background(), namespacedName, pvc)
	if err == nil {
		// PVC exists, nothing to do
		klog.Info("PVC already exists", "pvc", pvcName, "namespace", namespace)
		return nil
	}

	// Check if error is "not found" - if so, create the PVC
	if client.IgnoreNotFound(err) == nil {
		// PVC doesn't exist, create it
		klog.Info("Creating PVC", "pvc", pvcName, "namespace", namespace)

		// Get PVC configuration from environment variables or use defaults
		storageSize := getEnvOrDefault("NETCLIENT_PVC_STORAGE_SIZE", "1Gi")
		storageClass := getEnvOrDefault("NETCLIENT_PVC_STORAGE_CLASS", "") // Empty means use default

		// Create PVC spec
		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/component":  "netclient",
					"app.kubernetes.io/managed-by": "netmaker-k8s-ops-webhook",
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(storageSize),
					},
				},
			},
		}

		// Set storage class if specified
		if storageClass != "" {
			pvc.Spec.StorageClassName = &storageClass
		}

		// Create the PVC
		if err := w.client.Create(context.Background(), pvc); err != nil {
			return fmt.Errorf("failed to create PVC %s in namespace %s: %w", pvcName, namespace, err)
		}

		klog.Info("Successfully created PVC", "pvc", pvcName, "namespace", namespace, "storageSize", storageSize)
		return nil
	}

	// Some other error occurred
	return fmt.Errorf("failed to check PVC existence: %w", err)
}

// getPVCNameFromPod gets the PVC name from pod annotations or environment variable
func getPVCNameFromPod(annotations map[string]string, namespace string) string {
	// Check if pod has custom PVC name annotation
	if annotations != nil {
		if pvcName, exists := annotations["netmaker.io/pvc-name"]; exists && pvcName != "" {
			klog.Info("Found PVC name from annotation", "pvc", pvcName, "annotation", "netmaker.io/pvc-name")
			return pvcName
		}
		// Check for namespace-specific PVC annotation
		nsAnnotation := fmt.Sprintf("netmaker.io/pvc-name.%s", namespace)
		if pvcName, exists := annotations[nsAnnotation]; exists && pvcName != "" {
			klog.Info("Found PVC name from namespace-specific annotation", "pvc", pvcName, "annotation", nsAnnotation)
			return pvcName
		}
		klog.V(2).Info("No PVC annotation found", "checkedAnnotations", []string{"netmaker.io/pvc-name", nsAnnotation})
	}

	// Fallback to environment variable or default
	envPVC := getEnvOrDefault("NETCLIENT_PVC_NAME", "")
	if envPVC != "" {
		klog.Info("Using PVC name from environment variable", "pvc", envPVC)
		return envPVC
	}

	// Default to empty string to maintain backward compatibility (use EmptyDir)
	klog.V(2).Info("No PVC configured, will use EmptyDir")
	return ""
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

// mergeAnnotations merges two annotation maps, with the second map taking priority
func mergeAnnotations(base, override map[string]string) map[string]string {
	result := make(map[string]string)

	// Copy base annotations (ranging over nil map is safe in Go)
	for k, v := range base {
		result[k] = v
	}

	// Override with pod template annotations (ranging over nil map is safe in Go)
	for k, v := range override {
		result[k] = v
	}

	// Debug: log if we found the PVC annotation
	if pvcName, exists := result["netmaker.io/pvc-name"]; exists {
		klog.Info("Found netmaker.io/pvc-name in merged annotations", "pvc", pvcName)
	}

	return result
}

// mergeLabels merges two label maps, with the second map taking priority
func mergeLabels(base, override map[string]string) map[string]string {
	result := make(map[string]string)

	// Copy base labels (ranging over nil map is safe in Go)
	for k, v := range base {
		result[k] = v
	}

	// Override with pod template labels (ranging over nil map is safe in Go)
	for k, v := range override {
		result[k] = v
	}

	return result
}
