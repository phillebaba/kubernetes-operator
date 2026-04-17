package controller

import (
	"context"
	"fmt"

	netbird "github.com/netbirdio/netbird/shared/management/client/rest"
	"github.com/netbirdio/netbird/shared/management/http/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nbv1alpha1 "github.com/netbirdio/kubernetes-operator/api/v1alpha1"
	// nbv1alpha1ac "github.com/netbirdio/kubernetes-operator/pkg/applyconfigurations/api/v1alpha1"
)

// NetworkResourceReconciler reconciles a NetworkResource object
type NetworkResourceReconciler struct {
	client.Client

	Netbird *netbird.Client
}

// +kubebuilder:rbac:groups=netbird.io,resources=networkresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=netbird.io,resources=networkresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=netbird.io,resources=networkresources/finalizers,verbs=update
func (r *NetworkResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	netRes := nbv1alpha1.NetworkResource{}
	err := r.Get(ctx, req.NamespacedName, &netRes)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !netRes.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, netRes)
	}

	// Get the routing peer.
	routingPeer := &nbv1alpha1.RoutingPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netRes.Spec.RoutingPeerRef.Name,
			Namespace: netRes.Namespace,
		},
	}
	err = r.Client.Get(ctx, client.ObjectKeyFromObject(routingPeer), routingPeer)
	if err != nil {
		return ctrl.Result{}, err
	}
	// TODO: Check routing peer is ready.

	// Get service to expose to network.
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netRes.Spec.ServiceRef.Name,
			Namespace: netRes.Namespace,
		},
	}
	err = r.Client.Get(ctx, client.ObjectKeyFromObject(svc), svc)
	if err != nil {
		return ctrl.Result{}, err
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		//TODO: This is not a retryable error, should set condition.
		return ctrl.Result{}, fmt.Errorf("unsupported service type %s", svc.Spec.Type)
	}
	// TODO: Check service is ready and cluster ip is set.

	// TODO: Deduplicate copied code from setup key controller.
	groupIDs := []string{}
	for _, ref := range netRes.Spec.Groups {
		switch {
		case ref.ID != nil:
			_, err := r.Netbird.Groups.Get(ctx, *ref.ID)
			if err != nil {
				return ctrl.Result{}, err
			}
			groupIDs = append(groupIDs, *ref.ID)
		case ref.LocalRef != nil:
			group := nbv1alpha1.Group{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ref.LocalRef.Name,
					Namespace: netRes.Namespace,
				},
			}
			err = r.Client.Get(ctx, client.ObjectKeyFromObject(&group), &group)
			if err != nil {
				return ctrl.Result{}, err
			}
			if group.Status.GroupID == nil {
				return ctrl.Result{}, nil
			}
			groupIDs = append(groupIDs, *group.Status.GroupID)
		}
	}

	resourceID, err := func() (string, error) {
		resourceReq := api.NetworkResourceRequest{
			// TODO: consider dual stack
			Address: svc.Spec.ClusterIP,
			Enabled: true,
			Groups:  groupIDs,
			Name:    svc.Name + "/" + svc.Namespace,
		}

		if netRes.Status.ResourceID != nil {
			resourceResp, err := r.Netbird.Networks.Resources(*routingPeer.Status.NetworkID).Update(ctx, *netRes.Status.ResourceID, resourceReq)
			if err == nil {
				return resourceResp.Id, nil
			}
			if err != nil && !netbird.IsNotFound(err) {
				return "", err
			}
		}
		resourceResp, err := r.Netbird.Networks.Resources(*routingPeer.Status.NetworkID).Create(ctx, resourceReq)
		if err != nil {
			return "", err
		}
		return resourceResp.Id, nil
	}()
	if err != nil {
		return ctrl.Result{}, err
	}

	netRes.Status.ResourceID = &resourceID

	return ctrl.Result{}, nil
}

func (r *NetworkResourceReconciler) reconcileDelete(ctx context.Context, netRes nbv1alpha1.NetworkResource) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nbv1alpha1.NetworkResource{}).
		Complete(r)
}
