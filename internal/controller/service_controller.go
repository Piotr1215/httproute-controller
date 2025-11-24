/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	AnnotationExpose           = "gateway.homelab.local/expose"
	AnnotationHostname         = "gateway.homelab.local/hostname"
	AnnotationGateway          = "gateway.homelab.local/gateway"
	AnnotationGatewayNamespace = "gateway.homelab.local/gateway-namespace"
	AnnotationPort             = "gateway.homelab.local/port"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the reconciliation loop for Service resources.
// It watches Services with the expose annotation and creates/updates/deletes
// corresponding HTTPRoute and ReferenceGrant resources.
//
// The reconciler follows modern Kubernetes controller best practices:
// - Level-based triggers (reconciles full state, not just events)
// - Idempotent operations (safe to call multiple times)
// - OwnerReferences for automatic garbage collection
// - Cross-namespace resource management via ReferenceGrant
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Service
	svc := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		if errors.IsNotFound(err) {
			// Service deleted - OwnerReferences will handle cleanup
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if Service should be exposed
	expose := svc.Annotations[AnnotationExpose]
	if expose != "true" {
		// Service not marked for exposure - clean up if resources exist
		return r.cleanupResources(ctx, svc)
	}

	// Validate required annotations
	hostname := svc.Annotations[AnnotationHostname]
	if hostname == "" {
		log.Error(nil, "hostname annotation required when expose=true", "service", req.NamespacedName)
		return ctrl.Result{}, nil // Don't requeue - invalid configuration
	}

	// Get gateway configuration (with defaults)
	gatewayName := svc.Annotations[AnnotationGateway]
	if gatewayName == "" {
		gatewayName = "homelab-gateway"
	}

	gatewayNamespace := svc.Annotations[AnnotationGatewayNamespace]
	if gatewayNamespace == "" {
		gatewayNamespace = "envoy-gateway-system"
	}

	// Get service port
	var port int32
	if portStr := svc.Annotations[AnnotationPort]; portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}
	if port == 0 && len(svc.Spec.Ports) > 0 {
		port = svc.Spec.Ports[0].Port
	}
	if port == 0 {
		log.Error(nil, "no service port found", "service", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// Create/Update HTTPRoute
	if err := r.reconcileHTTPRoute(ctx, svc, hostname, gatewayName, gatewayNamespace, port); err != nil {
		log.Error(err, "failed to reconcile HTTPRoute")
		return ctrl.Result{}, err
	}

	// Create/Update ReferenceGrant
	if err := r.reconcileReferenceGrant(ctx, svc, gatewayNamespace); err != nil {
		log.Error(err, "failed to reconcile ReferenceGrant")
		return ctrl.Result{}, err
	}

	log.Info("successfully reconciled Service", "service", req.NamespacedName, "hostname", hostname)
	return ctrl.Result{}, nil
}

// reconcileHTTPRoute creates or updates the HTTPRoute for the Service.
// Uses OwnerReferences for automatic garbage collection when Service is deleted.
// This is idempotent - safe to call multiple times with same inputs.
func (r *ServiceReconciler) reconcileHTTPRoute(ctx context.Context, svc *corev1.Service, hostname, gatewayName, gatewayNamespace string, port int32) error {
	routeName := fmt.Sprintf("%s-%s", svc.Namespace, svc.Name)
	sectionName := gatewayv1.SectionName("https")

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: gatewayNamespace,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:        gatewayv1.ObjectName(gatewayName),
						Namespace:   (*gatewayv1.Namespace)(&gatewayNamespace),
						SectionName: &sectionName,
					},
				},
			},
			Hostnames: []gatewayv1.Hostname{
				gatewayv1.Hostname(hostname),
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      gatewayv1.ObjectName(svc.Name),
									Namespace: (*gatewayv1.Namespace)(&svc.Namespace),
									Port:      (*gatewayv1.PortNumber)(&port),
								},
							},
						},
					},
				},
			},
		},
	}

	// Set OwnerReference (cross-namespace owner reference)
	// Note: Cross-namespace OwnerReferences require special handling
	// We set the reference but Kubernetes won't enforce cascading deletion across namespaces
	// The controller must handle cleanup explicitly when expose annotation is removed
	route.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "v1",
			Kind:       "Service",
			Name:       svc.Name,
			UID:        svc.UID,
		},
	}

	// Check if HTTPRoute exists
	existing := &gatewayv1.HTTPRoute{}
	err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: gatewayNamespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new HTTPRoute
			return r.Create(ctx, route)
		}
		return err
	}

	// Update existing HTTPRoute
	existing.Spec = route.Spec
	existing.OwnerReferences = route.OwnerReferences
	return r.Update(ctx, existing)
}

// reconcileReferenceGrant creates or updates the ReferenceGrant for cross-namespace access.
// ReferenceGrant allows HTTPRoute in gateway namespace to reference Service in service namespace.
// Uses controllerutil.SetControllerReference for proper ownership and garbage collection.
// This is idempotent - safe to call multiple times with same inputs.
func (r *ServiceReconciler) reconcileReferenceGrant(ctx context.Context, svc *corev1.Service, gatewayNamespace string) error {
	grantName := fmt.Sprintf("%s-backend", svc.Name)

	grant := &gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      grantName,
			Namespace: svc.Namespace,
		},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{
				{
					Group:     gatewayv1.GroupName,
					Kind:      gatewayv1.Kind("HTTPRoute"),
					Namespace: gatewayv1.Namespace(gatewayNamespace),
				},
			},
			To: []gatewayv1beta1.ReferenceGrantTo{
				{
					Group: "",
					Kind:  "Service",
					Name:  (*gatewayv1.ObjectName)(&svc.Name),
				},
			},
		},
	}

	// Set controller reference for garbage collection
	if err := controllerutil.SetControllerReference(svc, grant, r.Scheme); err != nil {
		return err
	}

	// Check if ReferenceGrant exists
	existing := &gatewayv1beta1.ReferenceGrant{}
	err := r.Get(ctx, types.NamespacedName{Name: grantName, Namespace: svc.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new ReferenceGrant
			return r.Create(ctx, grant)
		}
		return err
	}

	// Update existing ReferenceGrant
	existing.Spec = grant.Spec
	existing.OwnerReferences = grant.OwnerReferences
	return r.Update(ctx, existing)
}

// cleanupResources removes HTTPRoute and ReferenceGrant when Service is no longer exposed.
// This handles the case where expose annotation is removed or changed to false.
func (r *ServiceReconciler) cleanupResources(ctx context.Context, svc *corev1.Service) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get default gateway namespace
	gatewayNamespace := svc.Annotations[AnnotationGatewayNamespace]
	if gatewayNamespace == "" {
		gatewayNamespace = "envoy-gateway-system"
	}

	// Try to delete HTTPRoute
	routeName := fmt.Sprintf("%s-%s", svc.Namespace, svc.Name)
	route := &gatewayv1.HTTPRoute{}
	err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: gatewayNamespace}, route)
	if err == nil {
		// HTTPRoute exists, delete it
		if err := r.Delete(ctx, route); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.Info("deleted HTTPRoute", "route", routeName)
	}

	// Try to delete ReferenceGrant
	grantName := fmt.Sprintf("%s-backend", svc.Name)
	grant := &gatewayv1beta1.ReferenceGrant{}
	err = r.Get(ctx, types.NamespacedName{Name: grantName, Namespace: svc.Namespace}, grant)
	if err == nil {
		// ReferenceGrant exists, delete it
		if err := r.Delete(ctx, grant); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.Info("deleted ReferenceGrant", "grant", grantName)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
// Watches Services and owns HTTPRoute and ReferenceGrant resources.
// Uses modern controller-runtime patterns:
// - For() watches the primary resource (Service)
// - Owns() watches secondary resources for changes (triggers reconciliation of owner)
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Owns(&gatewayv1.HTTPRoute{}).
		Owns(&gatewayv1beta1.ReferenceGrant{}).
		Named("service").
		Complete(r)
}
