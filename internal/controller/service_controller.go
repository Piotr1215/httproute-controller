/*
Copyright 2025 Piotr Zaniewski.

Licensed under the MIT License. See LICENSE file in the project root for full license information.
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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// Fixed annotation prefix - not configurable
const (
	AnnotationPrefix             = "httproute.controller"
	AnnotationExpose             = AnnotationPrefix + "/expose"
	AnnotationHostname           = AnnotationPrefix + "/hostname"
	AnnotationGateway            = AnnotationPrefix + "/gateway"
	AnnotationGatewayNamespace   = AnnotationPrefix + "/gateway-namespace"
	AnnotationSectionName        = AnnotationPrefix + "/section-name"
	AnnotationPort               = AnnotationPrefix + "/port"
	AnnotationSkipReferenceGrant = AnnotationPrefix + "/skip-reference-grant"
	FinalizerHTTPRoute           = AnnotationPrefix + "/httproute-finalizer"
)

// Config holds the controller configuration (required values, no defaults)
type Config struct {
	// DefaultGateway is the default gateway name (REQUIRED)
	DefaultGateway string
	// DefaultGatewayNamespace is the default gateway namespace (REQUIRED)
	DefaultGatewayNamespace string
	// DefaultSectionName is the default gateway listener section name
	DefaultSectionName string
}

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   Config
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//nolint:lll
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//nolint:lll
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants,verbs=get;list;watch;create;update;patch;delete

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	svc := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !svc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(svc, FinalizerHTTPRoute) {
			if err := r.cleanupResources(ctx, svc); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(svc, FinalizerHTTPRoute)
			if err := r.Update(ctx, svc); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Not exposed - cleanup and remove finalizer
	if svc.Annotations[AnnotationExpose] != "true" {
		if err := r.cleanupResources(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
		if controllerutil.ContainsFinalizer(svc, FinalizerHTTPRoute) {
			controllerutil.RemoveFinalizer(svc, FinalizerHTTPRoute)
			if err := r.Update(ctx, svc); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	hostname := svc.Annotations[AnnotationHostname]
	if hostname == "" {
		log.Error(nil, "hostname annotation required", "service", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	gatewayName := svc.Annotations[AnnotationGateway]
	if gatewayName == "" {
		gatewayName = r.Config.DefaultGateway
	}
	gatewayNamespace := svc.Annotations[AnnotationGatewayNamespace]
	if gatewayNamespace == "" {
		gatewayNamespace = r.Config.DefaultGatewayNamespace
	}
	sectionName := svc.Annotations[AnnotationSectionName]
	if sectionName == "" {
		sectionName = r.Config.DefaultSectionName
	}

	var port int32
	if portStr := svc.Annotations[AnnotationPort]; portStr != "" {
		_, _ = fmt.Sscanf(portStr, "%d", &port)
	}
	if port == 0 && len(svc.Spec.Ports) > 0 {
		port = svc.Spec.Ports[0].Port
	}
	if port == 0 {
		log.Error(nil, "no port found", "service", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	if err := r.reconcileHTTPRoute(ctx, svc, hostname, gatewayName, gatewayNamespace, sectionName, port); err != nil {
		r.recordEvent(svc, corev1.EventTypeWarning, "HTTPRouteFailed", err.Error())
		return ctrl.Result{}, err
	}
	r.recordEvent(svc, corev1.EventTypeNormal, "HTTPRouteReconciled",
		fmt.Sprintf("HTTPRoute %s-%s in %s", svc.Namespace, svc.Name, gatewayNamespace))

	if svc.Annotations[AnnotationSkipReferenceGrant] != "true" {
		if err := r.reconcileReferenceGrant(ctx, svc, gatewayNamespace); err != nil {
			r.recordEvent(svc, corev1.EventTypeWarning, "ReferenceGrantFailed", err.Error())
			return ctrl.Result{}, err
		}
	}

	if !controllerutil.ContainsFinalizer(svc, FinalizerHTTPRoute) {
		controllerutil.AddFinalizer(svc, FinalizerHTTPRoute)
		if err := r.Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.Info("reconciled", "service", req.NamespacedName, "hostname", hostname)
	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) reconcileHTTPRoute(
	ctx context.Context, svc *corev1.Service,
	hostname, gatewayName, gatewayNamespace, sectionNameStr string, port int32,
) error {
	routeName := fmt.Sprintf("%s-%s", svc.Namespace, svc.Name)
	sectionName := gatewayv1.SectionName(sectionNameStr)

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: gatewayNamespace,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name:        gatewayv1.ObjectName(gatewayName),
					Namespace:   (*gatewayv1.Namespace)(&gatewayNamespace),
					SectionName: &sectionName,
				}},
			},
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(hostname)},
			Rules: []gatewayv1.HTTPRouteRule{{
				BackendRefs: []gatewayv1.HTTPBackendRef{{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name:      gatewayv1.ObjectName(svc.Name),
							Namespace: (*gatewayv1.Namespace)(&svc.Namespace),
							Port:      (*gatewayv1.PortNumber)(&port),
						},
					},
				}},
			}},
		},
	}

	existing := &gatewayv1.HTTPRoute{}
	err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: gatewayNamespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, route)
	}
	if err != nil {
		return err
	}
	existing.Spec = route.Spec
	return r.Update(ctx, existing)
}

func (r *ServiceReconciler) reconcileReferenceGrant(
	ctx context.Context, svc *corev1.Service, gatewayNamespace string,
) error {
	grantName := fmt.Sprintf("%s-backend", svc.Name)

	grant := &gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      grantName,
			Namespace: svc.Namespace,
		},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{
				Group:     gatewayv1.GroupName,
				Kind:      "HTTPRoute",
				Namespace: gatewayv1.Namespace(gatewayNamespace),
			}},
			To: []gatewayv1beta1.ReferenceGrantTo{{
				Group: "",
				Kind:  "Service",
				Name:  (*gatewayv1.ObjectName)(&svc.Name),
			}},
		},
	}

	if err := controllerutil.SetControllerReference(svc, grant, r.Scheme); err != nil {
		return err
	}

	existing := &gatewayv1beta1.ReferenceGrant{}
	err := r.Get(ctx, types.NamespacedName{Name: grantName, Namespace: svc.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, grant)
	}
	if err != nil {
		return err
	}
	existing.Spec = grant.Spec
	existing.OwnerReferences = grant.OwnerReferences
	return r.Update(ctx, existing)
}

func (r *ServiceReconciler) cleanupResources(ctx context.Context, svc *corev1.Service) error {
	gatewayNamespace := svc.Annotations[AnnotationGatewayNamespace]
	if gatewayNamespace == "" {
		gatewayNamespace = r.Config.DefaultGatewayNamespace
	}

	routeName := fmt.Sprintf("%s-%s", svc.Namespace, svc.Name)
	route := &gatewayv1.HTTPRoute{}
	if err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: gatewayNamespace}, route); err == nil {
		if err := r.Delete(ctx, route); err != nil && !errors.IsNotFound(err) {
			return err
		}
		r.recordEvent(svc, corev1.EventTypeNormal, "HTTPRouteDeleted", routeName)
	}

	grantName := fmt.Sprintf("%s-backend", svc.Name)
	grant := &gatewayv1beta1.ReferenceGrant{}
	if err := r.Get(ctx, types.NamespacedName{Name: grantName, Namespace: svc.Namespace}, grant); err == nil {
		if err := r.Delete(ctx, grant); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *ServiceReconciler) recordEvent(svc *corev1.Service, eventType, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(svc, eventType, reason, message)
	}
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Owns(&gatewayv1beta1.ReferenceGrant{}).
		Named("service").
		Complete(r)
}
