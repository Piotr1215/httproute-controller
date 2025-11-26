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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var _ = Describe("Service Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling a Service without expose annotation", func() {
		It("should not create HTTPRoute or ReferenceGrant", func() {
			ctx := context.Background()

			// ARRANGE: Create service WITHOUT expose annotation
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc-no-expose",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, svc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, svc) }()

			// ACT: Wait for potential reconciliation
			time.Sleep(2 * time.Second)

			// ASSERT: HTTPRoute should NOT exist
			route := &gatewayv1.HTTPRoute{}
			routeKey := types.NamespacedName{
				Name:      "default-test-svc-no-expose",
				Namespace: "envoy-gateway-system",
			}
			err := k8sClient.Get(ctx, routeKey, route)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "HTTPRoute should not be created")

			// ASSERT: ReferenceGrant should NOT exist
			grant := &gatewayv1beta1.ReferenceGrant{}
			grantKey := types.NamespacedName{
				Name:      "test-svc-no-expose-backend",
				Namespace: "default",
			}
			err = k8sClient.Get(ctx, grantKey, grant)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "ReferenceGrant should not be created")
		})
	})

	Context("When reconciling a Service with expose=false", func() {
		It("should not create HTTPRoute or ReferenceGrant", func() {
			ctx := context.Background()

			// ARRANGE: Create service with expose=false
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc-expose-false",
					Namespace: "default",
					Annotations: map[string]string{
						"httproute.controller/expose": "false",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, svc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, svc) }()

			// ACT: Wait for potential reconciliation
			time.Sleep(2 * time.Second)

			// ASSERT: HTTPRoute should NOT exist
			route := &gatewayv1.HTTPRoute{}
			routeKey := types.NamespacedName{
				Name:      "default-test-svc-expose-false",
				Namespace: "envoy-gateway-system",
			}
			err := k8sClient.Get(ctx, routeKey, route)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "HTTPRoute should not be created")
		})
	})

	Context("When reconciling a Service with expose=true and valid hostname", func() {
		It("should create HTTPRoute and ReferenceGrant", func() {
			ctx := context.Background()

			// ARRANGE: Create service with expose=true and hostname
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc-exposed",
					Namespace: "default",
					Annotations: map[string]string{
						"httproute.controller/expose":   "true",
						"httproute.controller/hostname": "test.homelab.local",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, svc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, svc) }()

			// ACT & ASSERT: HTTPRoute should be created
			route := &gatewayv1.HTTPRoute{}
			routeKey := types.NamespacedName{
				Name:      "default-test-svc-exposed",
				Namespace: "envoy-gateway-system",
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, routeKey, route)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// ASSERT: Verify HTTPRoute content
			Expect(route.Spec.Hostnames).To(ContainElement(gatewayv1.Hostname("test.homelab.local")))
			Expect(route.Spec.ParentRefs).To(HaveLen(1))
			Expect(string(route.Spec.ParentRefs[0].Name)).To(Equal("test-gateway"))
			Expect(route.Spec.Rules).To(HaveLen(1))
			Expect(route.Spec.Rules[0].BackendRefs).To(HaveLen(1))
			backendRef := route.Spec.Rules[0].BackendRefs[0]
			Expect(string(backendRef.Name)).To(Equal("test-svc-exposed"))
			Expect(string(*backendRef.Namespace)).To(Equal("default"))
			Expect(*backendRef.Port).To(Equal(gatewayv1.PortNumber(80)))

			// ASSERT: HTTPRoute should NOT have OwnerReferences (cross-namespace not supported)
			// Cleanup is handled via finalizers on the Service
			Expect(route.OwnerReferences).To(BeEmpty())

			// ACT & ASSERT: ReferenceGrant should be created
			grant := &gatewayv1beta1.ReferenceGrant{}
			grantKey := types.NamespacedName{
				Name:      "test-svc-exposed-backend",
				Namespace: "default",
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, grantKey, grant)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// ASSERT: Verify ReferenceGrant content
			Expect(grant.Spec.From).To(HaveLen(1))
			Expect(string(grant.Spec.From[0].Group)).To(Equal(gatewayv1.GroupName))
			Expect(string(grant.Spec.From[0].Kind)).To(Equal("HTTPRoute"))
			Expect(string(grant.Spec.From[0].Namespace)).To(Equal("envoy-gateway-system"))
			Expect(grant.Spec.To).To(HaveLen(1))
			Expect(string(grant.Spec.To[0].Group)).To(Equal(""))
			Expect(string(grant.Spec.To[0].Kind)).To(Equal("Service"))

			// ASSERT: Verify OwnerReference for garbage collection
			Expect(grant.OwnerReferences).To(HaveLen(1))
			Expect(grant.OwnerReferences[0].Kind).To(Equal("Service"))
			Expect(grant.OwnerReferences[0].Name).To(Equal("test-svc-exposed"))
		})
	})

	Context("When reconciling a Service with custom gateway configuration", func() {
		It("should use custom gateway name and namespace", func() {
			ctx := context.Background()

			// ARRANGE: Create service with custom gateway config
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc-custom-gw",
					Namespace: "default",
					Annotations: map[string]string{
						"httproute.controller/expose":            "true",
						"httproute.controller/hostname":          "custom.homelab.local",
						"httproute.controller/gateway":           "custom-gateway",
						"httproute.controller/gateway-namespace": "custom-ns",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, svc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, svc) }()

			// ACT & ASSERT: HTTPRoute should be created in custom namespace
			route := &gatewayv1.HTTPRoute{}
			routeKey := types.NamespacedName{
				Name:      "default-test-svc-custom-gw",
				Namespace: "custom-ns",
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, routeKey, route)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// ASSERT: Verify custom gateway configuration
			Expect(string(route.Spec.ParentRefs[0].Name)).To(Equal("custom-gateway"))
			Expect(string(*route.Spec.ParentRefs[0].Namespace)).To(Equal("custom-ns"))

			// ASSERT: ReferenceGrant should allow access from custom namespace
			grant := &gatewayv1beta1.ReferenceGrant{}
			grantKey := types.NamespacedName{
				Name:      "test-svc-custom-gw-backend",
				Namespace: "default",
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, grantKey, grant)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(string(grant.Spec.From[0].Namespace)).To(Equal("custom-ns"))
		})
	})

	Context("When Service hostname annotation changes", func() {
		It("should update existing HTTPRoute", func() {
			ctx := context.Background()

			// ARRANGE: Create service
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc-update",
					Namespace: "default",
					Annotations: map[string]string{
						"httproute.controller/expose":   "true",
						"httproute.controller/hostname": "original.homelab.local",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, svc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, svc) }()

			// Wait for initial HTTPRoute
			routeKey := types.NamespacedName{
				Name:      "default-test-svc-update",
				Namespace: "envoy-gateway-system",
			}
			route := &gatewayv1.HTTPRoute{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, routeKey, route)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// ACT: Update hostname annotation
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, svc)).Should(Succeed())
			svc.Annotations["httproute.controller/hostname"] = "updated.homelab.local"
			Expect(k8sClient.Update(ctx, svc)).Should(Succeed())

			// ASSERT: HTTPRoute should be updated with new hostname
			Eventually(func() bool {
				err := k8sClient.Get(ctx, routeKey, route)
				if err != nil {
					return false
				}
				return len(route.Spec.Hostnames) > 0 &&
					string(route.Spec.Hostnames[0]) == "updated.homelab.local"
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When Service expose annotation is removed", func() {
		It("should clean up HTTPRoute and ReferenceGrant", func() {
			ctx := context.Background()

			// ARRANGE: Create service with expose=true
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc-remove-expose",
					Namespace: "default",
					Annotations: map[string]string{
						"httproute.controller/expose":   "true",
						"httproute.controller/hostname": "remove.homelab.local",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, svc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, svc) }()

			// Wait for resources to be created
			routeKey := types.NamespacedName{
				Name:      "default-test-svc-remove-expose",
				Namespace: "envoy-gateway-system",
			}
			route := &gatewayv1.HTTPRoute{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, routeKey, route)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// ACT: Remove expose annotation
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, svc)).Should(Succeed())
			delete(svc.Annotations, "httproute.controller/expose")
			Expect(k8sClient.Update(ctx, svc)).Should(Succeed())

			// ASSERT: HTTPRoute should be deleted (by controller removing it or by garbage collection)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, routeKey, route)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When Service with expose=true but missing hostname", func() {
		It("should not create HTTPRoute or ReferenceGrant", func() {
			ctx := context.Background()

			// ARRANGE: Create service with expose=true but NO hostname
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc-no-hostname",
					Namespace: "default",
					Annotations: map[string]string{
						"httproute.controller/expose": "true",
						// hostname annotation missing
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, svc)).Should(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, svc) }()

			// ACT: Wait for potential reconciliation
			time.Sleep(2 * time.Second)

			// ASSERT: HTTPRoute should NOT exist (invalid configuration)
			route := &gatewayv1.HTTPRoute{}
			routeKey := types.NamespacedName{
				Name:      "default-test-svc-no-hostname",
				Namespace: "envoy-gateway-system",
			}
			err := k8sClient.Get(ctx, routeKey, route)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "HTTPRoute should not be created for invalid config")
		})
	})

	Context("When Service is deleted", func() {
		// NOTE: OwnerReference garbage collection requires kube-controller-manager
		// which is not present in envtest. This test requires a real cluster.
		// See: https://github.com/kubernetes-sigs/controller-runtime/issues/626
		PIt("should automatically clean up HTTPRoute and ReferenceGrant via OwnerReferences", func() {
			ctx := context.Background()

			// ARRANGE: Create service
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc-delete",
					Namespace: "default",
					Annotations: map[string]string{
						"httproute.controller/expose":   "true",
						"httproute.controller/hostname": "delete.homelab.local",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, svc)).Should(Succeed())

			// Wait for resources to be created
			routeKey := types.NamespacedName{
				Name:      "default-test-svc-delete",
				Namespace: "envoy-gateway-system",
			}
			grantKey := types.NamespacedName{
				Name:      "test-svc-delete-backend",
				Namespace: "default",
			}

			route := &gatewayv1.HTTPRoute{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, routeKey, route)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			grant := &gatewayv1beta1.ReferenceGrant{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, grantKey, grant)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// ACT: Delete service
			Expect(k8sClient.Delete(ctx, svc)).Should(Succeed())

			// ASSERT: HTTPRoute should be garbage collected
			Eventually(func() bool {
				err := k8sClient.Get(ctx, routeKey, route)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// ASSERT: ReferenceGrant should be garbage collected
			Eventually(func() bool {
				err := k8sClient.Get(ctx, grantKey, grant)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})
