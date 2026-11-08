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
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	databasev1alpha1 "github.com/sudhir-u/my-mysql-operator/api/v1alpha1"
)

var _ = Describe("MySQL Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"
		const pinsNS = "default"
		const pinsCM = "test-mysql-channel-pins"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			os.Setenv("MYSQL_CHANNEL_PINS_CONFIGMAP", pinsCM)
			os.Setenv("POD_NAMESPACE", pinsNS)

			By("creating channel pins ConfigMap")
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: pinsCM, Namespace: pinsNS},
				Data: map[string]string{
					"channels.yaml": "channels:\n  \"8.4\": \"8.4.13\"\n",
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: pinsCM, Namespace: pinsNS}, &corev1.ConfigMap{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			}

			By("creating the custom resource for the Kind MySQL")
			mysql := &databasev1alpha1.MySQL{}
			err = k8sClient.Get(ctx, typeNamespacedName, mysql)
			if err != nil && errors.IsNotFound(err) {
				// Wide window so pin-driven upgrades are allowed in tests
				resource := &databasev1alpha1.MySQL{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: databasev1alpha1.MySQLSpec{
						Version:     "8.4",
						StorageSize: "1Gi",
						MaintenanceWindow: &databasev1alpha1.MaintenanceWindowSpec{
							Start: "00:00",
							End:   "23:59",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &databasev1alpha1.MySQL{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: pinsCM, Namespace: pinsNS}, cm); err == nil {
				_ = k8sClient.Delete(ctx, cm)
			}
			os.Unsetenv("MYSQL_CHANNEL_PINS_CONFIGMAP")
			os.Unsetenv("POD_NAMESPACE")
		})

		It("should create StatefulSet with pin-resolved image and Always pull", func() {
			controllerReconciler := &MySQLReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: "default"}, sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("mysql:8.4.13"))
			Expect(sts.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))
		})

		It("should update StatefulSet image when spec.version pin changes", func() {
			controllerReconciler := &MySQLReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			mysql := &databasev1alpha1.MySQL{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, mysql)).To(Succeed())
			mysql.Spec.Version = "8.4.14"
			Expect(k8sClient.Update(ctx, mysql)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: "default"}, sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("mysql:8.4.14"))
		})

		It("should defer pin-driven upgrade outside maintenance window", func() {
			controllerReconciler := &MySQLReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Simulate older image already running
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: "default"}, sts)).To(Succeed())
			sts.Spec.Template.Spec.Containers[0].Image = "mysql:8.4.10"
			Expect(k8sClient.Update(ctx, sts)).To(Succeed())

			mysql := &databasev1alpha1.MySQL{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, mysql)).To(Succeed())
			// Window that does not include "now" — use a 1-minute window far from typical test time is hard;
			// instead set a window of start==end which is full day... use empty by clearing and
			// a window we know is closed: pick start/end based on time.Now().UTC()
			now := time.Now().UTC()
			closedStart := fmt.Sprintf("%02d:%02d", (now.Hour()+3)%24, now.Minute())
			closedEnd := fmt.Sprintf("%02d:%02d", (now.Hour()+4)%24, now.Minute())
			mysql.Spec.MaintenanceWindow = &databasev1alpha1.MaintenanceWindowSpec{Start: closedStart, End: closedEnd}
			Expect(k8sClient.Update(ctx, mysql)).To(Succeed())

			res, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(res.RequeueAfter).To(BeNumerically(">", 0))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: "default"}, sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("mysql:8.4.10"))

			Expect(k8sClient.Get(ctx, typeNamespacedName, mysql)).To(Succeed())
			Expect(mysql.Status.Message).To(ContainSubstring("deferred"))
			Expect(mysql.Status.ResolvedImage).To(Equal("mysql:8.4.13"))
		})
	})
})
