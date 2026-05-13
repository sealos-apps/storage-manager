package session

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/nixieboluo/sealos-stroage-manager/internal/config"
	"github.com/nixieboluo/sealos-stroage-manager/internal/domain"
	"github.com/nixieboluo/sealos-stroage-manager/internal/kube"
	"github.com/nixieboluo/sealos-stroage-manager/internal/observability"
	"github.com/nixieboluo/sealos-stroage-manager/internal/state"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	labelComponent    = "storage-management.sealos.io/component"
	labelPVCName      = "storage-management.sealos.io/pvc-name"
	labelPVCUID       = "storage-management.sealos.io/pvc-uid"
	labelPodSessionID = "storage-management.sealos.io/pod-session-id"
	componentViewer   = "viewer"
)

type PodService struct {
	cfg      config.Config
	store    *state.Store
	client   kube.Interface
	mounts   *kube.PVCMountDetector
	recorder *observability.Recorder
	now      func() time.Time
}

type EnsurePodSessionInput struct {
	Namespace  string
	PVCName    string
	PVCUID     string
	AccessMode string
	Mode       string
	MountInfo  *domain.PVCMountInfo
}

func NewPodService(
	cfg config.Config,
	store *state.Store,
	client kube.Interface,
	recorder *observability.Recorder,
) *PodService {
	return &PodService{
		cfg:      cfg,
		store:    store,
		client:   client,
		mounts:   kube.NewPVCMountDetector(client),
		recorder: recorder,
		now:      time.Now,
	}
}

func (s *PodService) MountDetector() *kube.PVCMountDetector {
	return s.mounts
}

func (s *PodService) EnsurePodSession(
	ctx context.Context,
	input EnsurePodSessionInput,
) (*domain.PodSession, error) {
	now := s.now()
	if session, ok := s.store.FindPodSessionByPVC(input.Namespace, input.PVCUID, now); ok {
		if session.Status != domain.PodStatusTerminated && now.Before(session.ExpiresAt) {
			s.recorder.Metrics().PodReused.Add(1)
			return session, nil
		}
	}

	existing, err := s.findExistingViewerPod(ctx, input.Namespace, input.PVCUID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		session := s.rebuildPodSession(existing, input, now)
		s.store.PutPodSession(session)
		s.recorder.Metrics().PodReused.Add(1)
		return session, nil
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
	podSession := &domain.PodSession{
		ID:           id,
		Namespace:    input.Namespace,
		PVCName:      input.PVCName,
		PVCUID:       input.PVCUID,
		AccessMode:   input.AccessMode,
		Mode:         input.Mode,
		PodName:      name,
		ServiceName:  name,
		ViewerURL:    viewerURL,
		Status:       domain.PodStatusCreating,
		CreatedAt:    now,
		UpdatedAt:    now,
		LastActiveAt: now,
		ExpiresAt:    now.Add(s.cfg.Sessions.PodKeepaliveGrace),
	}

	pod := s.buildPod(podSession, input.MountInfo)
	service := s.buildService(podSession)
	ingress, err := s.buildIngress(podSession)
	if err != nil {
		return nil, err
	}
	if _, err := s.client.CreatePod(ctx, pod); err != nil {
		return nil, err
	}
	if _, err := s.client.CreateService(ctx, service); err != nil {
		_ = s.client.DeletePod(ctx, pod.Namespace, pod.Name)
		return nil, err
	}
	if _, err := s.client.CreateIngress(ctx, ingress); err != nil {
		_ = s.client.DeleteService(ctx, service.Namespace, service.Name)
		_ = s.client.DeletePod(ctx, pod.Namespace, pod.Name)
		return nil, err
	}

	s.store.PutPodSession(podSession)
	s.recorder.Metrics().PodCreated.Add(1)
	return podSession, nil
}

func (s *PodService) SyncPodStatus(ctx context.Context, podSession *domain.PodSession) (*domain.PodSession, error) {
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
		}
	case corev1.PodFailed:
		updated.Status = domain.PodStatusFailed
		updated.Reason = pod.Status.Reason
	case corev1.PodSucceeded:
		updated.Status = domain.PodStatusTerminated
	default:
		updated.Status = domain.PodStatusCreating
	}
	s.store.PutPodSession(&updated)
	return &updated, nil
}

func (s *PodService) ClosePodSession(ctx context.Context, podSessionID string) (*domain.PodSession, error) {
	now := s.now()
	podSession, ok := s.store.GetPodSessionIncludingExpired(podSessionID)
	if !ok {
		return nil, fmt.Errorf("pod session %q not found", podSessionID)
	}
	podSession.Status = domain.PodStatusTerminating
	podSession.UpdatedAt = now
	s.store.PutPodSession(podSession)

	_ = s.client.DeleteIngress(ctx, podSession.Namespace, podSession.ServiceName)
	_ = s.client.DeleteService(ctx, podSession.Namespace, podSession.ServiceName)
	_ = s.client.DeletePod(ctx, podSession.Namespace, podSession.PodName)

	podSession.Status = domain.PodStatusTerminated
	podSession.UpdatedAt = now
	s.store.DeletePodSession(podSessionID)
	s.recorder.Metrics().PodDeleted.Add(1)
	return podSession, nil
}

func (s *PodService) ReconcileViewerPods(ctx context.Context, namespace string) error {
	pods, err := s.client.ListViewerPods(ctx, namespace, map[string]string{labelComponent: componentViewer})
	if err != nil {
		return err
	}
	now := s.now()
	for i := range pods {
		pod := &pods[i]
		podSessionID := pod.Labels[labelPodSessionID]
		if podSessionID == "" {
			continue
		}
		if existing, ok := s.store.GetPodSession(podSessionID, now); ok {
			_, _ = s.SyncPodStatus(ctx, existing)
			continue
		}
		age := now.Sub(pod.CreationTimestamp.Time)
		if age <= s.cfg.Sessions.RecoveryGrace {
			input := EnsurePodSessionInput{
				Namespace:  pod.Namespace,
				PVCName:    pod.Labels[labelPVCName],
				PVCUID:     pod.Labels[labelPVCUID],
				AccessMode: domain.AccessModeReadWriteMany,
				Mode:       domain.ModeReadWrite,
			}
			session := s.rebuildPodSession(pod, input, now)
			s.store.PutPodSession(session)
			continue
		}
		if age > s.cfg.Sessions.OrphanGrace {
			_ = s.client.DeleteIngress(ctx, pod.Namespace, pod.Name)
			_ = s.client.DeleteService(ctx, pod.Namespace, pod.Name)
			_ = s.client.DeletePod(ctx, pod.Namespace, pod.Name)
			s.recorder.Metrics().CleanupDeleted.Add(1)
		}
	}
	return nil
}

func (s *PodService) findExistingViewerPod(
	ctx context.Context,
	namespace string,
	pvcUID string,
) (*corev1.Pod, error) {
	pods, err := s.client.ListViewerPods(ctx, namespace, map[string]string{
		labelComponent: componentViewer,
		labelPVCUID:    pvcUID,
	})
	if err != nil {
		return nil, err
	}
	for i := range pods {
		if pods[i].Status.Phase == corev1.PodFailed || pods[i].Status.Phase == corev1.PodSucceeded {
			continue
		}
		return &pods[i], nil
	}
	return nil, nil
}

func (s *PodService) rebuildPodSession(
	pod *corev1.Pod,
	input EnsurePodSessionInput,
	now time.Time,
) *domain.PodSession {
	id := pod.Labels[labelPodSessionID]
	if id == "" {
		id = strings.TrimPrefix(pod.Name, "viewer-")
	}
	viewerURL, _ := s.viewerURL(id)
	status := domain.PodStatusCreating
	if pod.Status.Phase == corev1.PodRunning && podReady(pod) {
		status = domain.PodStatusReady
	}
	return &domain.PodSession{
		ID:           id,
		Namespace:    input.Namespace,
		PVCName:      input.PVCName,
		PVCUID:       input.PVCUID,
		AccessMode:   input.AccessMode,
		Mode:         input.Mode,
		PodName:      pod.Name,
		ServiceName:  pod.Name,
		ViewerURL:    viewerURL,
		Status:       status,
		NodeName:     pod.Spec.NodeName,
		CreatedAt:    pod.CreationTimestamp.Time,
		UpdatedAt:    now,
		LastActiveAt: now,
		ExpiresAt:    now.Add(s.cfg.Sessions.PodKeepaliveGrace),
	}
}

func (s *PodService) buildPod(session *domain.PodSession, mountInfo *domain.PVCMountInfo) *corev1.Pod {
	readOnly := session.Mode == domain.ModeReadOnly
	labels := managedLabels(session)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: session.Namespace,
			Name:      session.PodName,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: s.cfg.Viewer.Pod.ServiceAccountName,
			Containers: []corev1.Container{
				{
					Name:  "filebrowser",
					Image: s.cfg.Viewer.FileBrowser.Image,
					Args: []string{
						"--root", s.cfg.Viewer.Pod.MountPath,
						"--address", "0.0.0.0",
						"--port", fmt.Sprint(s.cfg.Viewer.FileBrowser.Port),
						"--auth.method=hook",
						"--auth.header=",
						"--tokenExpirationTime=" + s.cfg.Viewer.FileBrowser.TokenTTL.String(),
						"--database", s.cfg.Viewer.Pod.DatabasePath,
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

func (s *PodService) buildService(session *domain.PodSession) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: session.Namespace,
			Name:      session.ServiceName,
			Labels:    managedLabels(session),
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

func (s *PodService) buildIngress(session *domain.PodSession) (*networkingv1.Ingress, error) {
	host, err := s.viewerHost(session.ID)
	if err != nil {
		return nil, err
	}
	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: session.Namespace,
			Name:      session.ServiceName,
			Labels:    managedLabels(session),
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &s.cfg.Viewer.Ingress.ClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: session.ServiceName,
											Port: networkingv1.ServiceBackendPort{
												Number: s.cfg.Viewer.Service.Port,
											},
										},
									},
								},
							},
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
	if err := tmpl.Execute(&out, map[string]string{"PodSessionID": id}); err != nil {
		return "", fmt.Errorf("executing viewer host template: %w", err)
	}
	return out.String(), nil
}

func resourceName(name string) string {
	name = strings.ToLower(name)
	if len(name) <= 63 {
		return name
	}
	return name[:63]
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

func managedLabels(session *domain.PodSession) map[string]string {
	return map[string]string{
		labelComponent:    componentViewer,
		labelPVCName:      session.PVCName,
		labelPVCUID:       session.PVCUID,
		labelPodSessionID: session.ID,
	}
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
