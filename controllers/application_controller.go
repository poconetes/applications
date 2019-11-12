package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/go-logr/logr"
	appsv1 "github.com/lxfontes/poconetes/api/v1"
	stappsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	MaxReconcile time.Duration
	Log          logr.Logger
	Scheme       *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.poconetes.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.poconetes.dev,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *ApplicationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.MaxReconcile)
	defer cancel()

	ll := r.Log.WithValues("application", req.NamespacedName)

	app := &appsv1.Application{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		if apierrors.IsNotFound(err) {
			ll.Info("not found, skipping")
			return ctrl.Result{}, nil
		}
	}

	if app.Status.ObservedGeneration != app.GetGeneration() {
		if err := r.refreshObjects(ctx, ll, app); err != nil {
			ll.Error(err, "refreshing objects")
			return ctrl.Result{}, err
		}

		app.Status.ObservedGeneration = app.GetGeneration()
		return ctrl.Result{}, r.Status().Update(ctx, app)
	}

	oldStatus := app.Status
	if err := r.refreshStatus(ctx, ll, app); err != nil {
		ll.Error(err, "refreshing status")
		return ctrl.Result{}, err
	}

	if !reflect.DeepEqual(app.Status, oldStatus) {
		ll.Info("updating status")
		return ctrl.Result{}, r.Status().Update(ctx, app)
	}

	return ctrl.Result{}, nil
}

func (r *ApplicationReconciler) refreshObjects(ctx context.Context, ll logr.Logger, app *appsv1.Application) error {
	for _, formation := range app.Spec.Formations {
		if err := r.refreshService(ctx, ll, app, formation); err != nil {
			return err
		}
		if err := r.refreshDeployment(ctx, ll, app, formation); err != nil {
			return err
		}
		if err := r.refreshAutoscaler(ctx, ll, app, formation); err != nil {
			return err
		}
	}

	if err := r.deleteStaleDeployments(ctx, ll, app); err != nil {
		return err
	}

	if err := r.deleteStaleServices(ctx, ll, app); err != nil {
		return err
	}

	return nil
}

func (r *ApplicationReconciler) deleteStaleServices(ctx context.Context, ll logr.Logger, app *appsv1.Application) error {
	serviceList := &corev1.ServiceList{}
	if err := r.List(
		ctx,
		serviceList,
		client.InNamespace(app.GetNamespace()),
		client.MatchingLabels{
			appsv1.LabelApp: app.GetName(),
		}); err != nil {
		return err
	}

	var deleteService []corev1.Service
	for _, svc := range serviceList.Items {
		if !metav1.IsControlledBy(&svc, app) {
			continue
		}

		formation, ok := svc.GetLabels()[appsv1.LabelFormation]
		if !ok {
			continue
		}

		var found bool
		for _, f := range app.Spec.Formations {
			if f.Name == formation {
				found = true
				break
			}
		}

		if !found {
			deleteService = append(deleteService, svc)
		}
	}

	for _, svc := range deleteService {
		if err := r.Delete(ctx, &svc); err != nil {
			return err
		}
	}

	return nil
}

func (r *ApplicationReconciler) deleteStaleDeployments(ctx context.Context, ll logr.Logger, app *appsv1.Application) error {
	deployList := &stappsv1.DeploymentList{}
	if err := r.List(
		ctx,
		deployList,
		client.InNamespace(app.GetNamespace()),
		client.MatchingLabels{
			appsv1.LabelApp: app.GetName(),
		}); err != nil {
		return err
	}

	var deleteDeploy []stappsv1.Deployment
	for _, deploy := range deployList.Items {
		if !metav1.IsControlledBy(&deploy, app) {
			continue
		}

		formation, ok := deploy.GetLabels()[appsv1.LabelFormation]
		if !ok {
			continue
		}

		var found bool
		for _, f := range app.Spec.Formations {
			if f.Name == formation {
				found = true
				break
			}
		}

		if !found {
			deleteDeploy = append(deleteDeploy, deploy)
		}
	}

	for _, deploy := range deleteDeploy {
		if err := r.Delete(ctx, &deploy); err != nil {
			return err
		}
	}

	return nil
}

func (r *ApplicationReconciler) refreshService(ctx context.Context, ll logr.Logger, app *appsv1.Application, formation appsv1.Formation) error {
	wantLabels := map[string]string{
		appsv1.LabelApp:       app.GetName(),
		appsv1.LabelFormation: formation.Name,
	}

	ports := []corev1.ServicePort{}
	for _, p := range formation.Ports {
		port := corev1.ServicePort{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: intstr.FromString(p.Name),
		}
		ports = append(ports, port)
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       app.GetNamespace(),
			Name:            formation.Name,
			Labels:          mergeLabels(app.GetLabels(), wantLabels),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(app, app.GroupVersionKind())},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  wantLabels,
			Ports:     ports,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r, service, func() error {
		service.Spec.Selector = wantLabels
		service.Spec.Ports = ports
		service.SetLabels(mergeLabels(service.GetLabels(), app.GetLabels(), wantLabels))
		return controllerutil.SetControllerReference(app, service, r.Scheme)
	})
	return err
}

func (r *ApplicationReconciler) refreshAutoscaler(ctx context.Context, ll logr.Logger, app *appsv1.Application, formation appsv1.Formation) error {
	wantLabels := map[string]string{
		appsv1.LabelApp:       app.GetName(),
		appsv1.LabelFormation: formation.Name,
	}

	spec := autoscaling.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: autoscaling.CrossVersionObjectReference{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
			Name:       formation.Name,
		},
		MinReplicas: formation.MinReplicas,
		MaxReplicas: 1,
		Metrics:     formation.Scaling,
	}

	if formation.MinReplicas != nil {
		spec.MaxReplicas = *formation.MinReplicas
	}

	if formation.MaxReplicas != nil {
		spec.MaxReplicas = *formation.MaxReplicas
	}

	if len(spec.Metrics) == 0 {
		spec.Metrics = append(spec.Metrics, autoscaling.MetricSpec{
			Type: autoscaling.ResourceMetricSourceType,
			Resource: &autoscaling.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscaling.MetricTarget{
					Type:               autoscaling.UtilizationMetricType,
					AverageUtilization: int32Ptr(80),
				},
			},
		})
	}

	hpa := &autoscaling.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       app.GetNamespace(),
			Name:            formation.Name,
			Labels:          mergeLabels(app.GetLabels(), wantLabels),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(app, app.GroupVersionKind())},
		},
		Spec: spec,
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r, hpa, func() error {
		hpa.Spec = spec
		hpa.SetLabels(mergeLabels(hpa.GetLabels(), app.GetLabels(), wantLabels))
		return controllerutil.SetControllerReference(app, hpa, r.Scheme)
	})
	return err
}

func (r *ApplicationReconciler) refreshDeployment(ctx context.Context, ll logr.Logger, app *appsv1.Application, formation appsv1.Formation) error {
	wantLabels := map[string]string{
		appsv1.LabelApp:       app.GetName(),
		appsv1.LabelFormation: formation.Name,
	}

	defaultMounts := []appsv1.Mount{
		{
			Name: "pocoshare",
			Path: "/pocoshare",
		},
	}

	spec := stappsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: wantLabels,
		},
		Replicas: formation.MinReplicas,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: mergeLabels(app.GetLabels(), wantLabels),
			},
			Spec: corev1.PodSpec{
				EnableServiceLinks: boolPtr(false),
				Volumes: []corev1.Volume{
					{
						Name: "pocoshare",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
			},
		},
	}

	defaultResources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("0.250"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("0.05"),
		},
	}

	container := corev1.Container{
		Name:         "pocolet",
		Image:        app.Spec.Image,
		Command:      formation.Command,
		Args:         formation.Args,
		EnvFrom:      mergeEnvFromSource(app.Spec.EnvironmentRefs, formation.EnvironmentRefs),
		Env:          mergeEnvVar(app.Spec.Environment, formation.Environment),
		VolumeMounts: mergeMounts(defaultMounts, app.Spec.Mounts, formation.Mounts),
		Resources:    defaultResources,
	}

	if formation.SLO != nil {
		container.Resources = *formation.SLO
	}

	for _, p := range formation.Ports {
		port := corev1.ContainerPort{
			Name:          p.Name,
			ContainerPort: p.Port,
		}
		container.Ports = append(container.Ports, port)
	}

	spec.Template.Spec.Containers = append(spec.Template.Spec.Containers, container)

	allVolumes := append([]appsv1.Mount{}, app.Spec.Mounts...)
	allVolumes = append(allVolumes, formation.Mounts...)

	sidecarList := &appsv1.SidecarList{}
	if err := r.List(ctx, sidecarList, client.InNamespace(app.GetNamespace())); err != nil {
		return err
	}

	for _, sidecar := range sidecarList.Items {
		sidecarContainer := corev1.Container{
			Name:         sidecar.GetName(),
			Image:        sidecar.Spec.Image,
			Command:      sidecar.Spec.Command,
			Args:         sidecar.Spec.Args,
			EnvFrom:      mergeEnvFromSource(app.Spec.EnvironmentRefs, formation.EnvironmentRefs, sidecar.Spec.EnvironmentRefs),
			Env:          mergeEnvVar(app.Spec.Environment, formation.Environment, sidecar.Spec.Environment),
			VolumeMounts: mergeMounts(defaultMounts, app.Spec.Mounts, formation.Mounts, sidecar.Spec.Mounts),
			Resources:    defaultResources,
		}

		if sidecar.Spec.SLO != nil {
			sidecarContainer.Resources = *sidecar.Spec.SLO
		}

		for _, p := range sidecar.Spec.Ports {
			port := corev1.ContainerPort{
				Name:          p.Name,
				ContainerPort: p.Port,
			}
			sidecarContainer.Ports = append(sidecarContainer.Ports, port)
		}

		spec.Template.Spec.Containers = append(spec.Template.Spec.Containers, sidecarContainer)

		allVolumes = append(allVolumes, sidecar.Spec.Mounts...)
	}

	initializerList := &appsv1.InitializerList{}
	if err := r.List(ctx, initializerList, client.InNamespace(app.GetNamespace())); err != nil {
		return err
	}

	for _, initializer := range initializerList.Items {
		initializerContainer := corev1.Container{
			Name:         initializer.GetName(),
			Image:        initializer.Spec.Image,
			Command:      initializer.Spec.Command,
			Args:         initializer.Spec.Args,
			EnvFrom:      mergeEnvFromSource(app.Spec.EnvironmentRefs, formation.EnvironmentRefs, initializer.Spec.EnvironmentRefs),
			Env:          mergeEnvVar(app.Spec.Environment, formation.Environment, initializer.Spec.Environment),
			VolumeMounts: mergeMounts(defaultMounts, app.Spec.Mounts, formation.Mounts, initializer.Spec.Mounts),
			Resources:    defaultResources,
		}

		if initializer.Spec.SLO != nil {
			initializerContainer.Resources = *initializer.Spec.SLO
		}

		spec.Template.Spec.InitContainers = append(spec.Template.Spec.InitContainers, initializerContainer)

		allVolumes = append(allVolumes, initializer.Spec.Mounts...)
	}

	if err := mergeVolumes(&spec.Template.Spec, allVolumes...); err != nil {
		return err
	}

	deploy := &stappsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            formation.Name,
			Namespace:       app.GetNamespace(),
			Labels:          mergeLabels(app.GetLabels(), wantLabels),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(app, app.GroupVersionKind())},
		},
		Spec: spec,
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r, deploy, func() error {
		spec.Replicas = deploy.Spec.Replicas // keep this stable, as HPA will be the one controlling it
		deploy.Spec = spec
		deploy.SetLabels(mergeLabels(deploy.GetLabels(), app.GetLabels(), wantLabels))
		return controllerutil.SetControllerReference(app, deploy, r.Scheme)
	})
	return err
}

func (r *ApplicationReconciler) refreshStatus(ctx context.Context, ll logr.Logger, app *appsv1.Application) error {
	app.Status.Formations = nil
	app.Status.Pocolets = 0
	state := appsv1.ApplicationStateOnline
	for _, formation := range app.Spec.Formations {
		status, err := r.refreshFormationStatus(ctx, ll, app, formation)
		if err != nil {
			return err
		}
		switch status.State {
		case appsv1.FormationStateError:
			state = appsv1.ApplicationStateError
		case appsv1.FormationStateWaiting:
			if state != appsv1.ApplicationStateError {
				state = appsv1.ApplicationStateWaiting
			}
		}
		app.Status.Formations = append(app.Status.Formations, status)
		app.Status.Pocolets += status.ReplicasDesired
	}

	sort.Sort(appsv1.FormationStatuses(app.Status.Formations))
	app.Status.State = state
	return nil
}

func (r *ApplicationReconciler) refreshFormationStatus(ctx context.Context, ll logr.Logger, app *appsv1.Application, formation appsv1.Formation) (appsv1.FormationStatus, error) {
	status := appsv1.FormationStatus{
		Name: formation.Name,
	}

	deploy := &stappsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: app.GetNamespace(), Name: formation.Name}, deploy); err != nil {
		if apierrors.IsNotFound(err) {
			ll.Info("missing deployment, skipping")
			status.State = appsv1.FormationStateWaiting
			status.Message = "waiting setup"
			return status, nil
		}
		return status, err
	}

	if !metav1.IsControlledBy(deploy, app) {
		ll.Info("invalid owner for deployment")
		status.State = appsv1.FormationStateError
		status.Message = "invalid deployment ownership"
		return status, nil
	}

	status.ReplicasDesired = *deploy.Spec.Replicas
	status.ReplicasAvailable = deploy.Status.AvailableReplicas
	status.ReplicasUnavailable = deploy.Status.UnavailableReplicas

	if deploy.Status.ObservedGeneration != deploy.GetGeneration() {
		status.State = appsv1.FormationStateWaiting
		status.Message = "deployment catching up"
		return status, nil
	}

	if deploy.Status.ReadyReplicas == deploy.Status.UpdatedReplicas &&
		deploy.Status.UnavailableReplicas == 0 {
		status.State = appsv1.FormationStateOnline
		status.Message = "all good"
		return status, nil
	}

	if deploy.Status.Replicas != deploy.Status.UpdatedReplicas {
		status.State = appsv1.FormationStateWaiting
		status.Message = "rolling out"
		return status, nil
	}

	status.State = appsv1.FormationStateError
	status.Message = "pocolets unavailable"

	return status, nil
}

func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Application{}).
		Owns(&stappsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func mergeLabels(lbls ...map[string]string) map[string]string {
	ret := make(map[string]string)

	for _, lbl := range lbls {
		for k, v := range lbl {
			ret[k] = v
		}
	}

	return ret
}

func mergeMounts(mounts ...[]appsv1.Mount) []corev1.VolumeMount {
	ret := make([]corev1.VolumeMount, 0)

	for _, mnts := range mounts {
		for _, mount := range mnts {
			ret = append(ret, corev1.VolumeMount{
				Name:      mount.Name,
				MountPath: mount.Path,
			})
		}
	}

	return ret
}

func mergeVolumes(spec *corev1.PodSpec, mounts ...appsv1.Mount) error {
	idx := make(map[string]appsv1.Mount)
	// make sure volume is under spec.volumes
	for _, mount := range mounts {
		if dup, ok := idx[mount.Name]; ok {
			if !reflect.DeepEqual(mount, dup) {
				return fmt.Errorf("duplicated mount: %s", mount.Name)
			}
		}

		src := corev1.VolumeSource{}
		if mount.ConfigMap != nil {
			src.ConfigMap = &corev1.ConfigMapVolumeSource{
				Optional:             boolPtr(true),
				LocalObjectReference: *mount.ConfigMap,
			}
		} else if mount.Secret != nil {
			src.Secret = &corev1.SecretVolumeSource{
				Optional:   boolPtr(true),
				SecretName: mount.Secret.Name,
			}
		}

		spec.Volumes = append(spec.Volumes, corev1.Volume{
			Name:         mount.Name,
			VolumeSource: src,
		})

		idx[mount.Name] = mount
	}

	return nil
}

func mergeEnvFromSource(srcs ...[]corev1.EnvFromSource) []corev1.EnvFromSource {
	ret := make([]corev1.EnvFromSource, 0)

	for _, evs := range srcs {
		ret = append(ret, evs...)
	}

	return ret
}

func mergeEnvVar(envs ...[]corev1.EnvVar) []corev1.EnvVar {
	idx := make(map[string]corev1.EnvVar)

	for _, evs := range envs {
		for _, ev := range evs {
			idx[ev.Name] = ev
		}
	}

	keys := make([]string, 0)
	for k := range idx {
		keys = append(keys, k)
	}
	sort.Sort(sort.StringSlice(keys))

	ret := make([]corev1.EnvVar, len(keys))
	for i, k := range keys {
		ret[i] = idx[k]
	}

	return ret
}

func int32Ptr(i int32) *int32 {
	return &i
}
func boolPtr(t bool) *bool {
	return &t
}
