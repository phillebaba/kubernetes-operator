package controller

import (
	"context"
	"strings"
	"time"

	netbird "github.com/netbirdio/netbird/shared/management/client/rest"
	"github.com/netbirdio/netbird/shared/management/http/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	netbirdiov1 "github.com/netbirdio/kubernetes-operator/api/v1"
	"github.com/netbirdio/kubernetes-operator/internal/util"
)

const (
	// defaultRequeueAfter default requeue duration
	// due to controller-runtime limitations, sync periods may reach up to 10 hours if no changes are detected
	// in watched resources.
	// This may cause issues when NetBird-side resources are out-of-sync and need to be reconciled, this is a temporary
	// fix to this issue by syncing with NetBird more frequently.
	defaultRequeueAfter = 15 * time.Minute
)

const (
	GroupFinalizer = "netbird.io/group-cleanup"
)

// NBGroupReconciler reconciles a NBGroup object
type NBGroupReconciler struct {
	client.Client

	Netbird *netbird.Client
}

func (r *NBGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	logger := ctrl.Log.WithName("NBGroup").WithValues("namespace", req.Namespace, "name", req.Name)
	logger.Info("Reconciling NBGroup")

	nbGroup := netbirdiov1.NBGroup{}
	err = r.Client.Get(ctx, req.NamespacedName, &nbGroup)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !nbGroup.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, nbGroup)
	}

	if controllerutil.AddFinalizer(&nbGroup, GroupFinalizer) {
		err := r.Client.Update(ctx, &nbGroup)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	group, err := func() (*api.Group, error) {
		if nbGroup.Status.GroupID != nil {
			groupReq := api.PutApiGroupsGroupIdJSONRequestBody{
				Name: nbGroup.Spec.Name,
			}
			group, err := r.Netbird.Groups.Update(ctx, *nbGroup.Status.GroupID, groupReq)
			if err != nil && !netbird.IsNotFound(err) {
				return nil, err
			}
			if err == nil {
				return group, nil
			}
		}
		group, err := r.Netbird.Groups.GetByName(ctx, nbGroup.Spec.Name)
		if err != nil && !netbird.IsNotFound(err) {
			return nil, err
		}
		if err == nil {
			return group, nil
		}
		groupReq := api.GroupRequest{
			Name: nbGroup.Spec.Name,
		}
		group, err = r.Netbird.Groups.Create(ctx, groupReq)
		if err != nil {
			return nil, err
		}
		return group, nil
	}()
	if err != nil {
		return ctrl.Result{}, err
	}

	nbGroup.Status.GroupID = &group.Id
	nbGroup.Status.Conditions = netbirdiov1.NBConditionTrue()
	err = r.Client.Status().Update(ctx, &nbGroup)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
}

func (r *NBGroupReconciler) reconcileDelete(ctx context.Context, nbGroup netbirdiov1.NBGroup) error {
	if nbGroup.Status.GroupID != nil {
		err := r.Netbird.Groups.Delete(ctx, *nbGroup.Status.GroupID)
		if err != nil && strings.Contains(err.Error(), "linked") && !nbGroup.DeletionTimestamp.Add(time.Minute).Before(time.Now()) {
			var groups netbirdiov1.NBGroupList
			listErr := r.Client.List(ctx, &groups)
			if listErr != nil {
				return listErr
			}
			for _, v := range groups.Items {
				if v.UID == nbGroup.UID {
					continue
				}
				if v.Status.GroupID != nil && nbGroup.Status.GroupID != nil && *v.Status.GroupID == *nbGroup.Status.GroupID {
					// Same group, multiple resources
					nbGroup.Finalizers = util.Without(nbGroup.Finalizers, "netbird.io/group-cleanup")
					err = r.Client.Update(ctx, &nbGroup)
					if err != nil {
						return err
					}
					return nil
				}
			}
			return err
		}
		if err != nil && !netbird.IsNotFound(err) {
			return err
		}
	}

	controllerutil.RemoveFinalizer(&nbGroup, GroupFinalizer)
	err := r.Client.Update(ctx, &nbGroup)
	if err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NBGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netbirdiov1.NBGroup{}).
		Complete(r)
}
