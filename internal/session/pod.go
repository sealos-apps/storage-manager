package session

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/apienv"
	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
	"github.com/nixieboluo/sealos-storage-manager/internal/kube"
	"github.com/nixieboluo/sealos-storage-manager/internal/observability"
	"github.com/nixieboluo/sealos-storage-manager/internal/state"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	labelComponent      = "storage-management.sealos.io/component"
	labelPVCName        = "storage-management.sealos.io/pvc-name"
	labelPVCUID         = "storage-management.sealos.io/pvc-uid"
	labelPodSessionID   = "storage-management.sealos.io/pod-session-id"
	labelRuntimeVersion = "storage-management.sealos.io/runtime-version"
	componentViewer     = "viewer"

	annotationAccessMode     = "storage-management.sealos.io/access-mode"
	annotationCreatedAt      = "storage-management.sealos.io/created-at"
	annotationKeepaliveUntil = "storage-management.sealos.io/keepalive-until"
	annotationLastActiveAt   = "storage-management.sealos.io/last-active-at"
	annotationMode           = "storage-management.sealos.io/mode"
	annotationRuntimeVersion = "storage-management.sealos.io/runtime-version"
)

var viewerIngressCORSAnnotations = map[string]string{
	"nginx.ingress.kubernetes.io/enable-cors":             "true",
	"nginx.ingress.kubernetes.io/cors-allow-origin":       "*",
	"nginx.ingress.kubernetes.io/cors-allow-credentials":  "false",
	"nginx.ingress.kubernetes.io/cors-allow-methods":      "GET, PUT, POST, DELETE, PATCH, OPTIONS",
	"nginx.ingress.kubernetes.io/cors-allow-headers":      "Authorization, X-Auth, Content-Type, Cache-Control, Pragma, Tus-Resumable, Upload-Length, Upload-Metadata, Upload-Offset, Upload-Defer-Length, Upload-Concat, Upload-Checksum, X-HTTP-Method-Override",
	"nginx.ingress.kubernetes.io/cors-expose-headers":     "Location, Tus-Resumable, Upload-Offset, Upload-Length, Upload-Metadata, Upload-Defer-Length, Upload-Concat, Upload-Expires",
	"nginx.ingress.kubernetes.io/cors-max-age":            "600",
	"nginx.ingress.kubernetes.io/proxy-body-size":         "0",
	"nginx.ingress.kubernetes.io/proxy-request-buffering": "off",
}

var fileBrowserAllowedIngressPaths = []string{
	"/api/login",
	"/api/raw",
	"/api/resources",
	"/api/tus",
	"/api/usage",
}

var dns1123Invalid = regexp.MustCompile(`[^a-z0-9-]+`)

type PodService struct {
	cfg            config.Config
	store          *state.Store
	client         kube.Interface
	mounts         *kube.PVCMountDetector
	recorder       *observability.Recorder
	runtimeVersion string
	now            func() time.Time
}

type EnsurePodSessionInput struct {
	AdminContext bool
	Namespace    string
	PVCName      string
	PVCUID       string
	AccessMode   string
	Mode         string
	MountInfo    *domain.PVCMountInfo
}

func NewPodService(
	cfg config.Config,
	store *state.Store,
	client kube.Interface,
	recorder *observability.Recorder,
) *PodService {
	return &PodService{
		cfg:            cfg,
		store:          store,
		client:         client,
		mounts:         kube.NewPVCMountDetector(client),
		recorder:       recorder,
		runtimeVersion: runtimeVersion(cfg),
		now:            time.Now,
	}
}

func (s *PodService) MountDetector() *kube.PVCMountDetector {
	return s.mounts
}

func (s *PodService) EnsurePodSession(
	ctx context.Context,
	input EnsurePodSessionInput,
) (podSession *domain.PodSession, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pod.ensure_session",
		slog.String("namespace", input.Namespace),
		slog.String("pvc_name", input.PVCName),
		slog.String("access_mode", input.AccessMode),
		slog.String("mode", input.Mode),
	)
	defer func() {
		finish(err)
	}()

	now := s.now()
	if session, ok := s.store.FindPodSessionByPVC(input.Namespace, input.PVCUID, now); ok {
		if session.RuntimeVersion == s.runtimeVersion &&
			session.AdminContext == input.AdminContext &&
			session.Status != domain.PodStatusTerminated &&
			now.Before(session.ExpiresAt) {
			s.recorder.ObservePodSession("reused")
			s.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "pod.session_reused",
				slog.String("pod_session_id", session.ID),
				slog.String("namespace", session.Namespace),
				slog.String("pvc_name", session.PVCName),
				slog.String("runtime_version", session.RuntimeVersion),
				slog.String("status", session.Status),
				slog.String("source", "state"),
			)
			return session, nil
		}
	}

	existing, err := s.findExistingViewerPod(ctx, input.Namespace, input.PVCUID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if err := s.deletePodIfExists(ctx, existing.Namespace, existing.Name); err != nil {
			return nil, err
		}
		s.recorder.ObserveCleanupDeleted()
		s.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "pod.orphan_viewer_pod_deleted",
			slog.String("namespace", existing.Namespace),
			slog.String("pod_name", existing.Name),
			slog.String("pvc_uid", input.PVCUID),
		)
	}

	id, err := newID("ps")
	if err != nil {
		return nil, err
	}
	name := resourceName("viewer-" + id)
	viewerURL, err := s.viewerURL(id)
	if err != nil {
		return nil, err
	}
	podSession = &domain.PodSession{
		ID:             id,
		Namespace:      input.Namespace,
		PVCName:        input.PVCName,
		PVCUID:         input.PVCUID,
		AccessMode:     input.AccessMode,
		Mode:           input.Mode,
		PodName:        name,
		ServiceName:    name,
		ViewerURL:      viewerURL,
		RuntimeVersion: s.runtimeVersion,
		Status:         domain.PodStatusCreating,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastActiveAt:   now,
		ExpiresAt:      now.Add(s.cfg.Sessions.PodKeepaliveGrace),
		AdminContext:   input.AdminContext,
	}

	pod := s.buildPod(podSession, input.MountInfo)
	createdPod, err := s.client.CreatePod(ctx, pod)
	if err != nil {
		if apierrors.IsForbidden(err) {
			return nil, apienv.NewError(403, apienv.CodeViewerPodFailed, err.Error(), nil)
		}
		return nil, err
	}
	owner := podOwnerReference(createdPod)
	hookConfigMap := s.buildHookConfigMap(podSession, owner)
	if _, err := s.client.CreateConfigMap(ctx, hookConfigMap); err != nil {
		_ = s.client.DeletePod(ctx, createdPod.Namespace, createdPod.Name)
		return nil, err
	}
	service := s.buildService(podSession, owner)
	ingress, err := s.buildIngress(podSession, owner)
	if err != nil {
		_ = s.client.DeletePod(ctx, createdPod.Namespace, createdPod.Name)
		return nil, err
	}
	if _, err := s.client.CreateService(ctx, service); err != nil {
		_ = s.client.DeletePod(ctx, createdPod.Namespace, createdPod.Name)
		return nil, err
	}
	if _, err := s.client.CreateIngress(ctx, ingress); err != nil {
		_ = s.client.DeletePod(ctx, createdPod.Namespace, createdPod.Name)
		return nil, err
	}

	s.store.PutPodSession(podSession)
	s.recorder.ObservePodSession("created")
	s.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "pod.session_created",
		slog.String("pod_session_id", podSession.ID),
		slog.String("namespace", podSession.Namespace),
		slog.String("pod_name", podSession.PodName),
		slog.String("pvc_name", podSession.PVCName),
		slog.String("runtime_version", podSession.RuntimeVersion),
		slog.String("mode", podSession.Mode),
	)
	return podSession, nil
}

func (s *PodService) buildHookConfigMap(
	session *domain.PodSession,
	owner metav1.OwnerReference,
) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       session.Namespace,
			Name:            hookConfigMapName(session),
			Labels:          managedLabels(session),
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Data: map[string]string{
			"filebrowser-auth-hook.sh": s.cfg.Viewer.HookScript,
		},
	}
}

func (s *PodService) SyncPodStatus(
	ctx context.Context,
	podSession *domain.PodSession,
) (synced *domain.PodSession, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pod.sync_status",
		slog.String("pod_session_id", podSession.ID),
		slog.String("namespace", podSession.Namespace),
		slog.String("pod_name", podSession.PodName),
	)
	defer func() {
		finish(err)
	}()

	pod, err := s.client.GetPod(ctx, podSession.Namespace, podSession.PodName)
	if err != nil {
		return nil, err
	}
	updated := *podSession
	updated.UpdatedAt = s.now()
	updated.NodeName = pod.Spec.NodeName
	switch pod.Status.Phase {
	case corev1.PodRunning:
		if podReady(pod) {
			updated.Status = domain.PodStatusReady
			updated.Reason = ""
			break
		}
		if reason := containerFailureReason(pod); reason != "" {
			updated.Status = domain.PodStatusFailed
			updated.Reason = reason
		}
	case corev1.PodFailed:
		updated.Status = domain.PodStatusFailed
		updated.Reason = pod.Status.Reason
	case corev1.PodSucceeded:
		updated.Status = domain.PodStatusTerminated
	default:
		updated.Status = domain.PodStatusCreating
	}
	s.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "pod.status_synced",
		slog.String("pod_session_id", updated.ID),
		slog.String("namespace", updated.Namespace),
		slog.String("pod_name", updated.PodName),
		slog.String("status", updated.Status),
		slog.String("node_name", updated.NodeName),
		slog.String("reason", updated.Reason),
	)
	return &updated, nil
}

func containerFailureReason(pod *corev1.Pod) string {
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			switch status.State.Waiting.Reason {
			case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError":
				return status.State.Waiting.Reason
			}
		}
		if status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode != 0 {
			reason := status.LastTerminationState.Terminated.Reason
			if reason == "" {
				reason = fmt.Sprintf("ContainerExited%d", status.LastTerminationState.Terminated.ExitCode)
			}
			return reason
		}
	}
	return ""
}

func (s *PodService) ClosePodSession(
	ctx context.Context,
	podSessionID string,
) (closed *domain.PodSession, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pod.close_session",
		slog.String("pod_session_id", podSessionID),
	)
	defer func() {
		finish(err)
	}()

	now := s.now()
	podSession, ok := s.store.GetPodSessionIncludingExpired(podSessionID)
	if !ok {
		return nil, fmt.Errorf("pod session %q not found", podSessionID)
	}
	podSession.Status = domain.PodStatusTerminating
	podSession.UpdatedAt = now
	s.store.PutPodSession(podSession)

	if err := s.deletePodIfExists(ctx, podSession.Namespace, podSession.PodName); err != nil {
		return nil, fmt.Errorf("closing pod session %q: %w", podSessionID, err)
	}

	podSession.Status = domain.PodStatusTerminated
	podSession.UpdatedAt = now
	s.store.DeletePodSession(podSessionID)
	s.recorder.ObservePodSession("deleted")
	s.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "pod.session_closed",
		slog.String("pod_session_id", podSession.ID),
		slog.String("namespace", podSession.Namespace),
		slog.String("pod_name", podSession.PodName),
	)
	return podSession, nil
}

func (s *PodService) RefreshPodSessionKeepalive(
	ctx context.Context,
	podSession *domain.PodSession,
) (refreshed *domain.PodSession, err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pod.refresh_keepalive",
		slog.String("pod_session_id", podSession.ID),
		slog.String("namespace", podSession.Namespace),
		slog.String("pod_name", podSession.PodName),
	)
	defer func() {
		finish(err)
	}()

	now := s.now()
	updated := *podSession
	updated.LastActiveAt = now
	updated.ExpiresAt = now.Add(s.cfg.Sessions.PodKeepaliveGrace)
	if _, err := s.client.PatchPodAnnotations(
		ctx,
		updated.Namespace,
		updated.PodName,
		keepaliveAnnotations(&updated),
	); err != nil {
		return nil, err
	}
	s.store.PutPodSession(&updated)
	return &updated, nil
}

func (s *PodService) ReconcileViewerPods(ctx context.Context, namespace string) (err error) {
	ctx, finish := s.recorder.TraceOperation(ctx,
		"pod.reconcile_viewer_pods",
		slog.String("namespace", namespace),
	)
	var scanned, deleted, skippedInvalid int
	defer func() {
		s.recorder.Logger().LogAttrs(ctx, slog.LevelDebug, "pod.reconcile_viewer_pods.result",
			slog.String("namespace", namespace),
			slog.String("runtime_version", s.runtimeVersion),
			slog.Int("scanned", scanned),
			slog.Int("deleted", deleted),
			slog.Int("skipped_invalid", skippedInvalid),
		)
		finish(err)
	}()

	pods, err := s.client.ListViewerPods(ctx, namespace, map[string]string{labelComponent: componentViewer})
	if err != nil {
		return err
	}
	now := s.now()
	for i := range pods {
		scanned++
		pod := &pods[i]
		podSessionID := pod.Labels[labelPodSessionID]
		if podSessionID == "" {
			skippedInvalid++
			continue
		}
		if pod.DeletionTimestamp != nil {
			skippedInvalid++
			continue
		}
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			if err := s.deletePodIfExists(ctx, pod.Namespace, pod.Name); err != nil {
				return err
			}
			s.recorder.ObserveCleanupDeleted()
			deleted++
			continue
		}
		if existing, ok := s.store.GetPodSessionIncludingExpired(podSessionID); ok {
			if existing.RuntimeVersion == s.runtimeVersion {
				if synced, err := s.SyncPodStatus(ctx, existing); err == nil {
					s.store.PutPodSession(synced)
				}
				continue
			}
		}
		if pod.Labels[labelRuntimeVersion] != s.runtimeVersion {
			if now.Sub(pod.CreationTimestamp.Time) <= s.cfg.Sessions.OrphanGrace {
				skippedInvalid++
				continue
			}
			if err := s.deletePodIfExists(ctx, pod.Namespace, pod.Name); err != nil {
				return err
			}
			s.recorder.ObserveCleanupDeleted()
			deleted++
			continue
		}
		if !s.viewerPodStillValid(pod, now) {
			if err := s.deletePodIfExists(ctx, pod.Namespace, pod.Name); err != nil {
				return err
			}
			s.recorder.ObserveCleanupDeleted()
			deleted++
			continue
		}
		if err := s.deletePodIfExists(ctx, pod.Namespace, pod.Name); err != nil {
			return err
		}
		s.recorder.ObserveCleanupDeleted()
		deleted++
	}
	return nil
}

func (s *PodService) viewerPodStillValid(pod *corev1.Pod, now time.Time) bool {
	if keepaliveUntil, ok := parseAnnotationTime(pod.Annotations, annotationKeepaliveUntil); ok {
		return now.Before(keepaliveUntil)
	}
	return now.Sub(pod.CreationTimestamp.Time) <= s.cfg.Sessions.RecoveryGrace
}

func (s *PodService) deletePodIfExists(ctx context.Context, namespace string, name string) error {
	if err := s.client.DeletePod(ctx, namespace, name); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (s *PodService) DeleteViewerPodsBySessionID(
	ctx context.Context,
	namespace string,
	podSessionID string,
) error {
	pods, err := s.client.ListViewerPods(ctx, namespace, map[string]string{
		labelComponent:    componentViewer,
		labelPodSessionID: podSessionID,
	})
	if err != nil {
		return err
	}
	for i := range pods {
		if err := s.deletePodIfExists(ctx, pods[i].Namespace, pods[i].Name); err != nil {
			return err
		}
		s.recorder.ObserveCleanupDeleted()
		s.recorder.Logger().LogAttrs(ctx, slog.LevelInfo, "pod.stale_viewer_pod_deleted",
			slog.String("pod_session_id", podSessionID),
			slog.String("namespace", pods[i].Namespace),
			slog.String("pod_name", pods[i].Name),
		)
	}
	return nil
}

func (s *PodService) findExistingViewerPod(
	ctx context.Context,
	namespace string,
	pvcUID string,
) (*corev1.Pod, error) {
	pods, err := s.client.ListViewerPods(ctx, namespace, map[string]string{
		labelComponent:      componentViewer,
		labelPVCUID:         pvcUID,
		labelRuntimeVersion: s.runtimeVersion,
	})
	if err != nil {
		return nil, err
	}
	for i := range pods {
		if pods[i].DeletionTimestamp != nil {
			continue
		}
		if pods[i].Status.Phase == corev1.PodFailed || pods[i].Status.Phase == corev1.PodSucceeded {
			continue
		}
		return &pods[i], nil
	}
	return nil, nil
}

func (s *PodService) buildPod(session *domain.PodSession, mountInfo *domain.PVCMountInfo) *corev1.Pod {
	readOnly := session.Mode == domain.ModeReadOnly
	labels := managedLabels(session)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   session.Namespace,
			Name:        session.PodName,
			Labels:      labels,
			Annotations: lifecycleAnnotations(session),
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: s.cfg.Viewer.Pod.ServiceAccountName,
			Containers: []corev1.Container{
				{
					Name:    "filebrowser",
					Image:   s.cfg.Viewer.FileBrowser.Image,
					Command: []string{"/bin/sh", "-c"},
					Args: []string{
						shellQuote(s.cfg.Viewer.FileBrowser.BinaryPath) + " config init " +
							"--database " + shellQuote(s.cfg.Viewer.Pod.DatabasePath) + " " +
							"--root " + shellQuote(s.cfg.Viewer.Pod.MountPath) + " " +
							"--address 0.0.0.0 " +
							"--port " + fmt.Sprint(s.cfg.Viewer.FileBrowser.Port) + " " +
							"--auth.method=hook " +
							"--auth.command=/hooks/filebrowser-auth-hook.sh " +
							"--auth.header= " +
							"--token-expiration-time " + shellQuote(s.cfg.Viewer.FileBrowser.TokenTTL.String()) + " " +
							"--disable-exec " +
							"&& exec " + shellQuote(s.cfg.Viewer.FileBrowser.BinaryPath) + " " +
							"--database " + shellQuote(s.cfg.Viewer.Pod.DatabasePath) + " " +
							"--root " + shellQuote(s.cfg.Viewer.Pod.MountPath) + " " +
							"--address 0.0.0.0 " +
							"--port " + fmt.Sprint(s.cfg.Viewer.FileBrowser.Port),
					},
					Ports: []corev1.ContainerPort{{ContainerPort: s.cfg.Viewer.FileBrowser.Port}},
					Env: []corev1.EnvVar{
						{Name: "POD_SESSION_ID", Value: session.ID},
						{Name: "VIEWER_POD_NAME", Value: session.PodName},
						{Name: "BACKEND_VERIFY_URL", Value: s.cfg.Viewer.BackendVerifyURL},
						{Name: "HOOK_CLIENT_TOKEN", Value: s.cfg.Viewer.HookClientToken},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "pvc",
							MountPath: s.cfg.Viewer.Pod.MountPath,
							ReadOnly:  readOnly,
						},
						{
							Name:      "hook",
							MountPath: "/hooks",
							ReadOnly:  true,
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: resourceList(s.cfg.Viewer.Pod.CPURequest, s.cfg.Viewer.Pod.MemoryRequest),
						Limits:   resourceList(s.cfg.Viewer.Pod.CPULimit, s.cfg.Viewer.Pod.MemoryLimit),
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "pvc",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: session.PVCName,
							ReadOnly:  readOnly,
						},
					},
				},
				{
					Name: "hook",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: hookConfigMapName(session),
							},
							DefaultMode: ptrInt32(0o555),
						},
					},
				},
			},
		},
	}
	if session.AccessMode == domain.AccessModeReadWriteOnce && mountInfo != nil && len(mountInfo.Nodes) == 1 {
		pod.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/hostname",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{mountInfo.Nodes[0]},
								},
							},
						},
					},
				},
			},
		}
	}
	return pod
}

func (s *PodService) buildService(
	session *domain.PodSession,
	owner metav1.OwnerReference,
) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       session.Namespace,
			Name:            session.ServiceName,
			Labels:          managedLabels(session),
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceType(s.cfg.Viewer.Service.Type),
			Selector: map[string]string{
				labelPodSessionID: session.ID,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       s.cfg.Viewer.Service.Port,
					TargetPort: intstr.FromInt32(s.cfg.Viewer.FileBrowser.Port),
				},
			},
		},
	}
}

func (s *PodService) buildIngress(
	session *domain.PodSession,
	owner metav1.OwnerReference,
) (*networkingv1.Ingress, error) {
	host, err := s.viewerHost(session.ID)
	if err != nil {
		return nil, err
	}
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       session.Namespace,
			Name:            session.ServiceName,
			Labels:          managedLabels(session),
			Annotations:     viewerIngressAnnotations(),
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &s.cfg.Viewer.Ingress.ClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: s.fileBrowserIngressPaths(session),
						},
					},
				},
			},
		},
	}
	if s.cfg.Viewer.Ingress.TLSSecretName != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{host},
				SecretName: s.cfg.Viewer.Ingress.TLSSecretName,
			},
		}
	}
	return ingress, nil
}

func (s *PodService) fileBrowserIngressPaths(session *domain.PodSession) []networkingv1.HTTPIngressPath {
	paths := make([]networkingv1.HTTPIngressPath, 0, len(fileBrowserAllowedIngressPaths))
	for _, path := range fileBrowserAllowedIngressPaths {
		paths = append(paths, networkingv1.HTTPIngressPath{
			Path:     path,
			PathType: ptr(networkingv1.PathTypePrefix),
			Backend: networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: session.ServiceName,
					Port: networkingv1.ServiceBackendPort{
						Number: s.cfg.Viewer.Service.Port,
					},
				},
			},
		})
	}
	return paths
}

func viewerIngressAnnotations() map[string]string {
	annotations := make(map[string]string, len(viewerIngressCORSAnnotations))
	for key, value := range viewerIngressCORSAnnotations {
		annotations[key] = value
	}
	return annotations
}

func (s *PodService) viewerURL(id string) (string, error) {
	host, err := s.viewerHost(id)
	if err != nil {
		return "", err
	}
	scheme := "https"
	if s.cfg.Viewer.Ingress.TLSSecretName == "" {
		scheme = "http"
	}
	return scheme + "://" + host, nil
}

func (s *PodService) viewerHost(id string) (string, error) {
	tmpl, err := template.New("host").Parse(s.cfg.Viewer.Ingress.HostTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing viewer host template: %w", err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, map[string]string{"PodSessionID": dnsLabel(id)}); err != nil {
		return "", fmt.Errorf("executing viewer host template: %w", err)
	}
	return out.String(), nil
}

func resourceName(name string) string {
	name = dnsLabel(name)
	if len(name) <= 63 {
		return name
	}
	return name[:63]
}

func dnsLabel(value string) string {
	value = strings.ToLower(value)
	value = dns1123Invalid.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "viewer"
	}
	return value
}

func resourceList(cpu string, memory string) corev1.ResourceList {
	resources := corev1.ResourceList{}
	if cpu != "" {
		resources[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		resources[corev1.ResourceMemory] = resource.MustParse(memory)
	}
	return resources
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func ptrInt32(value int32) *int32 {
	return ptr(value)
}

func ptr[T any](value T) *T {
	return &value
}

func hookConfigMapName(session *domain.PodSession) string {
	return session.PodName
}

func podOwnerReference(pod *corev1.Pod) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       pod.Name,
		UID:        pod.UID,
	}
}

func managedLabels(session *domain.PodSession) map[string]string {
	return map[string]string{
		labelComponent:      componentViewer,
		labelPVCName:        session.PVCName,
		labelPVCUID:         session.PVCUID,
		labelPodSessionID:   session.ID,
		labelRuntimeVersion: session.RuntimeVersion,
	}
}

func lifecycleAnnotations(session *domain.PodSession) map[string]string {
	annotations := map[string]string{
		annotationAccessMode:     session.AccessMode,
		annotationCreatedAt:      session.CreatedAt.Format(time.RFC3339Nano),
		annotationKeepaliveUntil: session.ExpiresAt.Format(time.RFC3339Nano),
		annotationLastActiveAt:   session.LastActiveAt.Format(time.RFC3339Nano),
		annotationMode:           session.Mode,
		annotationRuntimeVersion: session.RuntimeVersion,
	}
	return annotations
}

func keepaliveAnnotations(session *domain.PodSession) map[string]string {
	return map[string]string{
		annotationKeepaliveUntil: session.ExpiresAt.Format(time.RFC3339Nano),
		annotationLastActiveAt:   session.LastActiveAt.Format(time.RFC3339Nano),
	}
}

func parseAnnotationTime(annotations map[string]string, key string) (time.Time, bool) {
	value := annotations[key]
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func runtimeVersion(cfg config.Config) string {
	type versionedViewerConfig struct {
		BackendVerifyURL    string                   `json:"backend_verify_url"`
		HookClientTokenHash string                   `json:"hook_client_token_hash"`
		HookScript          string                   `json:"hook_script"`
		FileBrowser         config.FileBrowserConfig `json:"filebrowser"`
		Pod                 config.PodConfig         `json:"pod"`
		Service             config.ServiceConfig     `json:"service"`
		Ingress             config.IngressConfig     `json:"ingress"`
	}
	tokenHash := sha256.Sum256([]byte(cfg.Viewer.HookClientToken))
	body, err := json.Marshal(versionedViewerConfig{
		BackendVerifyURL:    cfg.Viewer.BackendVerifyURL,
		HookClientTokenHash: hex.EncodeToString(tokenHash[:]),
		HookScript:          cfg.Viewer.HookScript,
		FileBrowser:         cfg.Viewer.FileBrowser,
		Pod:                 cfg.Viewer.Pod,
		Service:             cfg.Viewer.Service,
		Ingress:             cfg.Viewer.Ingress,
	})
	if err != nil {
		panic(fmt.Sprintf("marshaling runtime version config: %v", err))
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])[:12]
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
