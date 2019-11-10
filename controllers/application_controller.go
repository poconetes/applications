package controllers

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "github.com/lxfontes/poconetes/api/v1"
	stappsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=apps.poconetes.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.poconetes.dev,resources=applications/status,verbs=get;update;patch

func (r *ApplicationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
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
	}

	if err := r.refreshStatus(ctx, ll, app); err != nil {
		ll.Error(err, "refreshing status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ApplicationReconciler) refreshObjects(ctx context.Context, ll logr.Logger, app *appsv1.Application) error {
	for _, formation := range app.Spec.Formations {
		if err := r.refreshStatefulSet(ctx, ll, app, formation); err != nil {
			return err
		}
	}

	return nil
}

func (r *ApplicationReconciler) refreshStatefulSet(ctx context.Context, ll logr.Logger, app *appsv1.Application, formation appsv1.Formation) error {
	wantLabels := map[string]string{
		appsv1.LabelApp:       app.GetName(),
		appsv1.LabelFormation: formation.Name,
	}

	sspec := stappsv1.StatefulSetSpec{
		ServiceName: formation.Name,
		Selector: &metav1.LabelSelector{
			MatchLabels: wantLabels,
		},
		Replicas: int32Ptr(3),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: mergeLabels(app.GetLabels(), wantLabels),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "pocolet",
						Image: app.Spec.Image,
					},
				},
			},
		},
	}

	sset := &stappsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      formation.Name,
			Namespace: app.GetNamespace(),
			Labels:    app.GetLabels(),
		},
		Spec: sspec,
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r, sset, func() error {
		sset.Spec = sspec
		return nil
	})
	return err
}

func (r *ApplicationReconciler) refreshStatus(ctx context.Context, ll logr.Logger, app *appsv1.Application) error {
	return nil
}

func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Application{}).
		Owns(&stappsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func int32Ptr(i int32) *int32 {
	return &i
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
