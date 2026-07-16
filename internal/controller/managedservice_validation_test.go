/*
Copyright 2026.

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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

var _ = Describe("ManagedService API validation", func() {
	ctx := context.Background()

	It("accepts an approved template and defaults replicas", func() {
		resource := &platformv1alpha1.ManagedService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "valid-managed-service",
				Namespace: "default",
			},
			Spec: platformv1alpha1.ManagedServiceSpec{
				Template: platformv1alpha1.ManagedServiceTemplateDemoHTTP,
			},
		}

		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(
				k8sClient.Delete(ctx, resource),
			)).To(Succeed())
		})

		Expect(resource.Spec.Replicas).NotTo(BeNil())
		Expect(*resource.Spec.Replicas).To(Equal(int32(1)))
	})

	DescribeTable(
		"rejects invalid specifications",
		func(
			name string,
			spec platformv1alpha1.ManagedServiceSpec,
			expectedField string,
		) {
			resource := &platformv1alpha1.ManagedService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "default",
				},
				Spec: spec,
			}

			err := k8sClient.Create(ctx, resource)

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring(expectedField))
		},
		Entry(
			"unsupported template",
			"invalid-template",
			platformv1alpha1.ManagedServiceSpec{
				Template: platformv1alpha1.ManagedServiceTemplate(
					"arbitrary-image",
				),
			},
			"spec.template",
		),
		Entry(
			"zero replicas",
			"invalid-zero-replicas",
			platformv1alpha1.ManagedServiceSpec{
				Template: platformv1alpha1.ManagedServiceTemplateDemoHTTP,
				Replicas: int32Pointer(0),
			},
			"spec.replicas",
		),
		Entry(
			"too many replicas",
			"invalid-many-replicas",
			platformv1alpha1.ManagedServiceSpec{
				Template: platformv1alpha1.ManagedServiceTemplateDemoHTTP,
				Replicas: int32Pointer(4),
			},
			"spec.replicas",
		),
		Entry(
			"message exceeds the maximum length",
			"invalid-message",
			platformv1alpha1.ManagedServiceSpec{
				Template: platformv1alpha1.ManagedServiceTemplateDemoHTTP,
				Message:  strings.Repeat("x", 121),
			},
			"spec.message",
		),
	)
})

func int32Pointer(value int32) *int32 {
	return &value
}
