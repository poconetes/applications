package controllers

import (
	"context"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	appsv1 "github.com/poconetes/applications/api/v1"
	stappsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// PlanReconciler reconciles a Plan object
type PlanReconciler struct {
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

func (r *PlanReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.MaxReconcile)
	defer cancel()

	ll := r.Log.WithValues("application", req.NamespacedName)

	plan := &appsv1.Plan{}
	if err := r.Get(ctx, req.NamespacedName, plan); err != nil {
		if apierrors.IsNotFound(err) {
			ll.Info("not found, skipping")
			return ctrl.Result{}, nil
		}
	}

	oldStatus := plan.Status
	if err := r.refreshStatus(ctx, ll, plan); err != nil {
		ll.Error(err, "refreshing status")
		return ctrl.Result{}, err
	}

	if !reflect.DeepEqual(plan.Status, oldStatus) {
		ll.Info("updating status")
		return ctrl.Result{}, r.Status().Update(ctx, plan)
	}

	return ctrl.Result{}, nil
}

func (r *PlanReconciler) refreshStatus(ctx context.Context, ll logr.Logger, plan *appsv1.Plan) error {
	return nil
}

func (r *PlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Plan{}).
		Watches(&source.Kind{Type: &stappsv1.Deployment{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: r}).
		Complete(r)
}

func (r *PlanReconciler) Map(obj handler.MapObject) []reconcile.Request {
	var reqs []reconcile.Request
	plan, ok := obj.Meta.GetLabels()[appsv1.LabelPlan]
	if ok {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: plan,
			},
		})
	}

	return reqs
}
