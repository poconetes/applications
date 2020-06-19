package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	appsv1 "github.com/poconetes/applications/api/v1"
	stappsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
// +kubebuilder:rbac:groups=apps.poconetes.dev,resources=sidecars,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.poconetes.dev,resources=initializers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.poconetes.dev,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

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

	if app.GetGeneration() != app.Status.ObservedGeneration {
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
	if err := r.refreshService(ctx, ll, app); err != nil {
		return err
	}
	if err := r.refreshDeployment(ctx, ll, app); err != nil {
		return err
	}
	if err := r.refreshAutoscaler(ctx, ll, app); err != nil {
		return err
	}

	return nil
}

func (r *ApplicationReconciler) refreshService(ctx context.Context, ll logr.Logger, app *appsv1.Application) error {
	wantLabels := map[string]string{
		appsv1.LabelApp: app.GetName(),
	}

	ports := []corev1.ServicePort{}
	for _, p := range app.Spec.Ports {
		port := corev1.ServicePort{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: intstr.FromString(p.Name),
		}
		ports = append(ports, port)
	}

	sidecarList := &appsv1.SidecarList{}
	if err := r.List(ctx, sidecarList, client.InNamespace(app.GetNamespace())); err != nil {
		return err
	}

	for _, sidecar := range sidecarList.Items {
		for _, p := range sidecar.Spec.Ports {
			port := corev1.ServicePort{
				Name:       p.Name,
				Port:       p.Port,
				TargetPort: intstr.FromString(p.Name),
			}
			ports = append(ports, port)
		}
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       app.GetNamespace(),
			Name:            app.GetName(),
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

func (r *ApplicationReconciler) refreshAutoscaler(ctx context.Context, ll logr.Logger, app *appsv1.Application) error {
	wantLabels := map[string]string{
		appsv1.LabelApp: app.GetName(),
	}

	spec := autoscaling.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: autoscaling.CrossVersionObjectReference{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
			Name:       app.GetName(),
		},
		MinReplicas: app.Spec.MinReplicas,
		MaxReplicas: app.Spec.MaxReplicas,
		Metrics:     app.Spec.Scaling,
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
			Name:            app.GetName(),
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

func (r *ApplicationReconciler) planFor(ctx context.Context, ll logr.Logger, app *appsv1.Application) (*appsv1.Plan, error) {
	planName := app.Spec.Plan
	if planName == "" {
		planName = appsv1.DefaultPlanName
	}
	plan := &appsv1.Plan{}
	if err := r.Get(ctx, types.NamespacedName{Name: planName}, plan); err != nil {
		return nil, err
	}

	return plan, nil
}

func (r *ApplicationReconciler) refreshDeployment(ctx context.Context, ll logr.Logger, app *appsv1.Application) error {
	plan, err := r.planFor(ctx, ll, app)
	if err != nil {
		return err
	}

	wantLabels := map[string]string{
		appsv1.LabelApp: app.GetName(),
	}

	defaultMounts := []appsv1.Mount{
		{
			Name: "pocoshare",
			Path: "/pocoshare",
		},
	}

	defaultEnv := []corev1.EnvVar{
		{
			Name:  "POCO_REVISION",
			Value: strconv.FormatInt(app.GetGeneration(), 10),
		},
	}

	spec := stappsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: wantLabels,
		},
		Replicas: app.Spec.MinReplicas,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: mergeLabels(app.GetLabels(), wantLabels),
			},
			Spec: corev1.PodSpec{
				EnableServiceLinks:           boolPtr(false),
				AutomountServiceAccountToken: boolPtr(false),
				Affinity:                     plan.Spec.Affinity,
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

	container := corev1.Container{
		Name:         "pocolet",
		Image:        app.Spec.Image,
		Command:      app.Spec.Command,
		Args:         app.Spec.Args,
		EnvFrom:      mergeEnvFromSource(app.Spec.EnvironmentRefs),
		Env:          mergeEnvVar(app.Spec.Environment, defaultEnv),
		VolumeMounts: mergeMounts(defaultMounts, app.Spec.Mounts),
	}

	if plan.Spec.ContainerResources != nil {
		container.Resources = *plan.Spec.ContainerResources
	}

	for _, p := range app.Spec.Ports {
		port := corev1.ContainerPort{
			Name:          p.Name,
			ContainerPort: p.Port,
		}
		container.Ports = append(container.Ports, port)
	}

	spec.Template.Spec.Containers = append(spec.Template.Spec.Containers, container)

	allVolumes := append([]appsv1.Mount{}, app.Spec.Mounts...)

	sidecarList := &appsv1.SidecarList{}
	if err := r.List(ctx, sidecarList, client.InNamespace(app.GetNamespace())); err != nil {
		return err
	}

	for _, sidecar := range sidecarList.Items {
		sidecarContainer := corev1.Container{
			Name:         "sidecar-" + sidecar.GetName(),
			Image:        sidecar.Spec.Image,
			Command:      sidecar.Spec.Command,
			Args:         sidecar.Spec.Args,
			EnvFrom:      mergeEnvFromSource(app.Spec.EnvironmentRefs, sidecar.Spec.EnvironmentRefs),
			Env:          mergeEnvVar(app.Spec.Environment, sidecar.Spec.Environment, defaultEnv),
			VolumeMounts: mergeMounts(defaultMounts, app.Spec.Mounts, sidecar.Spec.Mounts),
		}

		if plan.Spec.SidecarResources != nil {
			sidecarContainer.Resources = *plan.Spec.SidecarResources
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
			Name:         "init-" + initializer.GetName(),
			Image:        initializer.Spec.Image,
			Command:      initializer.Spec.Command,
			Args:         initializer.Spec.Args,
			EnvFrom:      mergeEnvFromSource(app.Spec.EnvironmentRefs, initializer.Spec.EnvironmentRefs),
			Env:          mergeEnvVar(app.Spec.Environment, initializer.Spec.Environment, defaultEnv),
			VolumeMounts: mergeMounts(defaultMounts, app.Spec.Mounts, initializer.Spec.Mounts),
		}

		if plan.Spec.InitializerResources != nil {
			initializerContainer.Resources = *plan.Spec.InitializerResources
		}

		spec.Template.Spec.InitContainers = append(spec.Template.Spec.InitContainers, initializerContainer)

		allVolumes = append(allVolumes, initializer.Spec.Mounts...)
	}

	if err := mergeVolumes(&spec.Template.Spec, allVolumes...); err != nil {
		return err
	}

	deploy := &stappsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            app.GetName(),
			Namespace:       app.GetNamespace(),
			Labels:          mergeLabels(app.GetLabels(), wantLabels),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(app, app.GroupVersionKind())},
		},
		Spec: spec,
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r, deploy, func() error {
		spec.Replicas = deploy.Spec.Replicas // keep this stable, as HPA will be the one controlling it
		deploy.Spec = spec
		deploy.SetLabels(mergeLabels(deploy.GetLabels(), app.GetLabels(), wantLabels))
		return controllerutil.SetControllerReference(app, deploy, r.Scheme)
	})
	return err
}

func (r *ApplicationReconciler) refreshStatus(ctx context.Context, ll logr.Logger, app *appsv1.Application) error {
	deploy := &stappsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: app.GetNamespace(), Name: app.GetName()}, deploy); err != nil {
		if apierrors.IsNotFound(err) {
			app.Status.State = appsv1.ApplicationStateError
			app.Status.Message = "deployment not found"
			return nil
		}
	}

	if !metav1.IsControlledBy(deploy, app) {
		ll.Info("invalid owner for deployment")
		app.Status.State = appsv1.ApplicationStateError
		app.Status.Message = "invalid deployment ownership"
		return nil
	}

	app.Status.ReplicasDesired = *deploy.Spec.Replicas
	app.Status.ReplicasAvailable = deploy.Status.AvailableReplicas
	app.Status.ReplicasUnavailable = deploy.Status.UnavailableReplicas

	if deploy.Status.ObservedGeneration != deploy.GetGeneration() {
		app.Status.State = appsv1.ApplicationStateUpdate
		app.Status.Message = "deployment being created"
		return nil
	}

	if deploy.Status.ReadyReplicas == deploy.Status.UpdatedReplicas &&
		deploy.Status.UnavailableReplicas == 0 {
		app.Status.State = appsv1.ApplicationStateOnline
		app.Status.Message = ""
		return nil
	}

	if deploy.Status.Replicas != deploy.Status.UpdatedReplicas {
		app.Status.State = appsv1.ApplicationStateUpdate
		app.Status.Message = "rolling out"
		return nil
	}

	app.Status.State = appsv1.ApplicationStateError
	app.Status.Message = "could not determine state"

	return nil
}

func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Application{}).
		Owns(&stappsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&autoscaling.HorizontalPodAutoscaler{}).
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
