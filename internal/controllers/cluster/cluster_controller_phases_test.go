/*
Copyright 2019 The Kubernetes Authors.

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

package cluster

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	externalfake "sigs.k8s.io/cluster-api/controllers/external/fake"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestClusterReconcileInfrastructure(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Status: clusterv1.ClusterStatus{
			Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
				Host: "1.2.3.4",
				Port: 8443,
			},
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "GenericInfrastructureMachine",
				Name:     "test",
			},
		},
	}
	clusterNoEndpoint := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Status: clusterv1.ClusterStatus{
			Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "GenericInfrastructureMachine",
				Name:     "test",
			},
		},
	}

	tests := []struct {
		name         string
		cluster      *clusterv1.Cluster
		infraRef     map[string]interface{}
		expectErr    bool
		expectResult ctrl.Result
		check        func(g *GomegaWithT, in *clusterv1.Cluster)
	}{
		{
			name:      "returns no error if infrastructure ref is nil",
			cluster:   &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-namespace"}},
			expectErr: false,
			check: func(g *GomegaWithT, in *clusterv1.Cluster) {
				g.Expect(ptr.Deref(in.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
				g.Expect(v1beta1conditions.IsTrue(in, clusterv1.InfrastructureReadyV1Beta1Condition)).To(BeTrue())
			},
		},
		{
			name: "requeue if unable to get infrastructure ref and cluster did not yet reported infrastructure ready",
			cluster: func() *clusterv1.Cluster {
				c := cluster.DeepCopy()
				c.Status.Initialization.InfrastructureProvisioned = ptr.To(false)
				return c
			}(),
			expectErr:    false,
			expectResult: ctrl.Result{RequeueAfter: externalReadyWait},
		},
		{
			name:      "returns error if unable to get infrastructure ref and cluster already reported infrastructure ready",
			cluster:   cluster.DeepCopy(),
			expectErr: true,
		},
		{
			name: "tolerate if unable to get infrastructure ref and cluster is deleting",
			cluster: func() *clusterv1.Cluster {
				c := cluster.DeepCopy()
				c.Finalizers = append(c.Finalizers, "test") // Note: this is required to get fake client to accept objects with DeletionTimestamp.
				c.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-1 * time.Hour)}
				return c
			}(),
			expectErr:    false,
			expectResult: ctrl.Result{},
		},
		{
			name:    "returns no error if infrastructure is marked ready on cluster",
			cluster: cluster.DeepCopy(),
			infraRef: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"deletionTimestamp": "sometime",
				},
			},
			expectErr: false,
		},
		{
			name:    "returns no error if the control plane endpoint is not yet set",
			cluster: clusterNoEndpoint.DeepCopy(),
			infraRef: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"deletionTimestamp": "sometime",
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			expectErr: false,
		},
		{
			name:    "should propagate the control plane endpoint once set",
			cluster: clusterNoEndpoint.DeepCopy(),
			infraRef: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"deletionTimestamp": "sometime",
				},
				"spec": map[string]interface{}{
					"controlPlaneEndpoint": map[string]interface{}{
						"host": "example.com",
						"port": int64(6443),
					},
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			expectErr: false,
			check: func(g *GomegaWithT, in *clusterv1.Cluster) {
				g.Expect(in.Spec.ControlPlaneEndpoint.Host).To(Equal("example.com"))
				g.Expect(in.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(6443))
			},
		},
		{
			name:    "do not allows to change infrastructure ready and control plane endpoint once set",
			cluster: cluster.DeepCopy(),
			infraRef: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"deletionTimestamp": "sometime",
				},
				"spec": map[string]interface{}{
					"controlPlaneEndpoint": map[string]interface{}{
						"host": "example.com",
						"port": int64(6443),
					},
				},
				"status": map[string]interface{}{
					"ready": false,
				},
			},
			expectErr: false,
			check: func(g *GomegaWithT, in *clusterv1.Cluster) {
				g.Expect(in.Spec.ControlPlaneEndpoint.Host).To(Equal("1.2.3.4"))
				g.Expect(in.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(8443))
				g.Expect(ptr.Deref(in.Status.Initialization.InfrastructureProvisioned, false)).To(BeTrue())
			},
		},
		{
			name: "do not reconcile if infra config is marked for deletion",
			cluster: func() *clusterv1.Cluster {
				c := clusterNoEndpoint.DeepCopy()
				c.Status.Initialization.InfrastructureProvisioned = ptr.To(false)
				return c
			}(),
			infraRef: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"finalizers":        []interface{}{"test"},
					"deletionTimestamp": "2020-01-01T00:00:00Z",
				},
				"spec": map[string]interface{}{
					"controlPlaneEndpoint": map[string]interface{}{
						"host": "example.com",
						"port": int64(6443),
					},
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			expectErr: false,
			check: func(g *GomegaWithT, in *clusterv1.Cluster) {
				g.Expect(in.Spec.ControlPlaneEndpoint.Host).To(Equal(""))
				g.Expect(in.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(0))
				g.Expect(ptr.Deref(in.Status.Initialization.InfrastructureProvisioned, false)).To(BeFalse())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var c client.Client
			if tt.infraRef != nil {
				infraConfig := &unstructured.Unstructured{Object: tt.infraRef}
				c = fake.NewClientBuilder().
					WithObjects(builder.GenericInfrastructureMachineCRD.DeepCopy(), tt.cluster, infraConfig).
					Build()
			} else {
				c = fake.NewClientBuilder().
					WithObjects(builder.GenericInfrastructureMachineCRD.DeepCopy(), tt.cluster).
					Build()
			}
			r := &Reconciler{
				Client:   c,
				recorder: record.NewFakeRecorder(32),
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          c.Scheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}

			s := &scope{
				cluster: tt.cluster,
			}
			res, err := r.reconcileInfrastructure(ctx, s)
			g.Expect(res).To(BeComparableTo(tt.expectResult))
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tt.check != nil {
				tt.check(g, tt.cluster)
			}
		})
	}
}

func TestClusterReconcileControlPlane(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Status: clusterv1.ClusterStatus{
			Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
				Host: "1.2.3.4",
				Port: 8443,
			},
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionControlPlane.Group,
				Kind:     "GenericControlPlane",
				Name:     "test",
			},
		},
	}
	clusterNoEndpoint := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Status: clusterv1.ClusterStatus{
			Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionControlPlane.Group,
				Kind:     "GenericControlPlane",
				Name:     "test",
			},
		},
	}

	tests := []struct {
		name         string
		cluster      *clusterv1.Cluster
		cpRef        map[string]interface{}
		expectErr    bool
		expectResult ctrl.Result
		check        func(g *GomegaWithT, in *clusterv1.Cluster)
	}{
		{
			name:      "returns no error if control plane ref is nil",
			cluster:   &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-namespace"}},
			expectErr: false,
		},
		{
			name:         "requeue if unable to get control plane ref and cluster did not yet reported control plane ready",
			cluster:      cluster.DeepCopy(),
			expectErr:    false,
			expectResult: ctrl.Result{RequeueAfter: externalReadyWait},
		},
		{
			name: "returns error if unable to get control plane ref and cluster already reported  control plane ready",
			cluster: func() *clusterv1.Cluster {
				c := cluster.DeepCopy()
				c.Status.Initialization.ControlPlaneInitialized = ptr.To(true)
				return c
			}(),
			expectErr: true,
		},
		{
			name: "tolerate if unable to get control plane ref and cluster is deleting",
			cluster: func() *clusterv1.Cluster {
				c := cluster.DeepCopy()
				c.Finalizers = append(c.Finalizers, "test") // Note: this is required to get fake client to accept objects with DeletionTimestamp.
				c.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-1 * time.Hour)}
				return c
			}(),
			expectErr:    false,
			expectResult: ctrl.Result{},
		},
		{
			name:    "returns no error if control plane ref is marked for deletion",
			cluster: cluster.DeepCopy(),
			cpRef: map[string]interface{}{
				"kind":       "GenericControlPlane",
				"apiVersion": clusterv1.GroupVersionControlPlane.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"deletionTimestamp": "sometime",
				},
			},
			expectErr: false,
		},
		{
			name:    "returns no error if control plane has the paused annotation",
			cluster: cluster.DeepCopy(),
			cpRef: map[string]interface{}{
				"kind":       "GenericControlPlane",
				"apiVersion": clusterv1.GroupVersionControlPlane.String(),
				"metadata": map[string]interface{}{
					"name":      "test",
					"namespace": "test-namespace",
					"annotations": map[string]interface{}{
						"cluster.x-k8s.io/paused": "true",
					},
				},
			},
			expectErr: false,
		},
		{
			name:    "returns no error if the control plane endpoint is not yet set",
			cluster: clusterNoEndpoint.DeepCopy(),
			cpRef: map[string]interface{}{
				"kind":       "GenericControlPlane",
				"apiVersion": clusterv1.GroupVersionControlPlane.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"deletionTimestamp": "sometime",
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			expectErr: false,
		},
		{
			name:    "should propagate the control plane endpoint if set",
			cluster: clusterNoEndpoint.DeepCopy(),
			cpRef: map[string]interface{}{
				"kind":       "GenericControlPlane",
				"apiVersion": clusterv1.GroupVersionControlPlane.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"deletionTimestamp": "sometime",
				},
				"spec": map[string]interface{}{
					"controlPlaneEndpoint": map[string]interface{}{
						"host": "example.com",
						"port": int64(6443),
					},
				},
				"status": map[string]interface{}{
					"initialization": map[string]interface{}{
						"controlPlaneInitialized": true,
					},
				},
			},
			expectErr: false,
			check: func(g *GomegaWithT, in *clusterv1.Cluster) {
				g.Expect(in.Spec.ControlPlaneEndpoint.Host).To(Equal("example.com"))
				g.Expect(in.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(6443))
			},
		},
		{
			name:    "should propagate the initialized and ready conditions",
			cluster: clusterNoEndpoint.DeepCopy(),
			cpRef: map[string]interface{}{
				"kind":       "GenericControlPlane",
				"apiVersion": clusterv1.GroupVersionControlPlane.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"deletionTimestamp": "sometime",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"initialization": map[string]interface{}{
						"controlPlaneInitialized": true,
					},
				},
			},
			expectErr: false,
			check: func(g *GomegaWithT, in *clusterv1.Cluster) {
				g.Expect(ptr.Deref(in.Status.Initialization.ControlPlaneInitialized, false)).To(BeTrue())

				g.Expect(v1beta1conditions.IsTrue(in, clusterv1.ControlPlaneReadyV1Beta1Condition)).To(BeTrue())
				g.Expect(v1beta1conditions.IsTrue(in, clusterv1.ControlPlaneInitializedV1Beta1Condition)).To(BeTrue())
			},
		},
		{
			name: "do not allows to change control plane ready and control plane endpoint once set",
			cluster: func() *clusterv1.Cluster {
				c := cluster.DeepCopy()
				c.Status.Initialization.ControlPlaneInitialized = ptr.To(true)
				return c
			}(),
			cpRef: map[string]interface{}{
				"kind":       "GenericControlPlane",
				"apiVersion": clusterv1.GroupVersionControlPlane.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"deletionTimestamp": "sometime",
				},
				"spec": map[string]interface{}{
					"controlPlaneEndpoint": map[string]interface{}{
						"host": "example.com",
						"port": int64(6443),
					},
				},
				"status": map[string]interface{}{
					"ready":       false,
					"initialized": false,
				},
			},
			expectErr: false,
			check: func(g *GomegaWithT, in *clusterv1.Cluster) {
				g.Expect(in.Spec.ControlPlaneEndpoint.Host).To(Equal("1.2.3.4"))
				g.Expect(in.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(8443))
				g.Expect(ptr.Deref(in.Status.Initialization.ControlPlaneInitialized, false)).To(BeTrue())
			},
		},
		{
			name:    "do not reconcile if control plane is marked for deletion",
			cluster: clusterNoEndpoint.DeepCopy(),
			cpRef: map[string]interface{}{
				"kind":       "GenericControlPlane",
				"apiVersion": clusterv1.GroupVersionControlPlane.String(),
				"metadata": map[string]interface{}{
					"name":              "test",
					"namespace":         "test-namespace",
					"finalizers":        []interface{}{"test"},
					"deletionTimestamp": "2020-01-01T00:00:00Z",
				},
				"spec": map[string]interface{}{
					"controlPlaneEndpoint": map[string]interface{}{
						"host": "example.com",
						"port": int64(6443),
					},
				},
				"status": map[string]interface{}{
					"ready":       true,
					"initialized": true,
				},
			},
			expectErr: false,
			check: func(g *GomegaWithT, in *clusterv1.Cluster) {
				g.Expect(in.Spec.ControlPlaneEndpoint.Host).To(Equal(""))
				g.Expect(in.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(0))
				g.Expect(ptr.Deref(in.Status.Initialization.ControlPlaneInitialized, false)).To(BeFalse())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var c client.Client
			if tt.cpRef != nil {
				cpConfig := &unstructured.Unstructured{Object: tt.cpRef}
				c = fake.NewClientBuilder().
					WithObjects(builder.GenericControlPlaneCRD.DeepCopy(), tt.cluster, cpConfig).
					Build()
			} else {
				c = fake.NewClientBuilder().
					WithObjects(builder.GenericControlPlaneCRD.DeepCopy(), tt.cluster).
					Build()
			}
			r := &Reconciler{
				Client:   c,
				recorder: record.NewFakeRecorder(32),
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          c.Scheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}

			s := &scope{
				cluster: tt.cluster,
			}
			res, err := r.reconcileControlPlane(ctx, s)
			g.Expect(res).To(BeComparableTo(tt.expectResult))
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tt.check != nil {
				tt.check(g, tt.cluster)
			}
		})
	}
}

func TestClusterReconcileKubeConfig(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
				Host: "1.2.3.4",
				Port: 8443,
			},
		},
	}

	tests := []struct {
		name        string
		cluster     *clusterv1.Cluster
		secret      *corev1.Secret
		wantErr     bool
		wantRequeue bool
	}{
		{
			name:    "cluster not provisioned, apiEndpoint is not set",
			cluster: &clusterv1.Cluster{},
			wantErr: false,
		},
		{
			name:    "kubeconfig secret found",
			cluster: cluster,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-kubeconfig",
				},
			},
			wantErr: false,
		},
		{
			name:        "kubeconfig secret not found, should requeue",
			cluster:     cluster,
			wantErr:     false,
			wantRequeue: true,
		},
		{
			name:    "invalid ca secret, should return error",
			cluster: cluster,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-ca",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().
				WithObjects(tt.cluster).
				Build()
			if tt.secret != nil {
				c = fake.NewClientBuilder().
					WithObjects(tt.cluster, tt.secret).
					Build()
			}
			r := &Reconciler{
				Client:   c,
				recorder: record.NewFakeRecorder(32),
			}

			s := &scope{
				cluster: tt.cluster,
			}
			res, err := r.reconcileKubeconfig(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tt.wantRequeue {
				g.Expect(res.RequeueAfter).To(BeNumerically(">=", 0))
			}
		})
	}
}

func TestClusterReconcilePhases_reconcileFailureDomains(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Status: clusterv1.ClusterStatus{
			Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
				Host: "1.2.3.4",
				Port: 8443,
			},
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "GenericInfrastructureCluster",
				Name:     "test",
			},
		},
	}

	newFailureDomain := []clusterv1.FailureDomain{
		{
			Name:         "newdomain",
			ControlPlane: ptr.To(false),
			Attributes: map[string]string{
				"attribute1": "value1",
			},
		},
	}

	newFailureDomainUpdated := []clusterv1.FailureDomain{
		{
			Name:         "newdomain",
			ControlPlane: ptr.To(false),
			Attributes: map[string]string{
				"attribute2": "value2",
			},
		},
	}

	clusterWithNewFailureDomainUpdated := cluster.DeepCopy()
	clusterWithNewFailureDomainUpdated.Status.FailureDomains = newFailureDomainUpdated

	oldFailureDomain := []clusterv1.FailureDomain{
		{
			Name:         "olddomain",
			ControlPlane: ptr.To(false),
			Attributes: map[string]string{
				"attribute1": "value1",
			},
		},
	}

	clusterWithOldFailureDomain := cluster.DeepCopy()
	clusterWithOldFailureDomain.Status.FailureDomains = oldFailureDomain

	tests := []struct {
		name                 string
		cluster              *clusterv1.Cluster
		infraRef             map[string]interface{}
		contract             string
		expectFailureDomains []clusterv1.FailureDomain
		expectErr            bool
	}{
		{
			name:    "expect no failure domain if infrastructure ref is nil",
			cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-namespace"}},
		},
		{
			name:                 "expect no failure domain if infra config does not have failure domain",
			cluster:              cluster.DeepCopy(),
			infraRef:             generateInfraCluster(false),
			contract:             "v1beta2",
			expectFailureDomains: []clusterv1.FailureDomain{},
		},
		{
			name:                 "expect cluster failure domain to be reset to empty if infra config does not have failure domain",
			cluster:              clusterWithOldFailureDomain.DeepCopy(),
			infraRef:             generateInfraCluster(false),
			contract:             "v1beta2",
			expectFailureDomains: []clusterv1.FailureDomain{},
		},
		{
			name:                 "expect failure domain to remain same if infra config have same failure domain",
			cluster:              cluster.DeepCopy(),
			infraRef:             generateInfraCluster(true),
			contract:             "v1beta2",
			expectFailureDomains: newFailureDomain,
		},
		{
			name:                 "expect failure domain to be updated if infra config has updates to failure domain",
			cluster:              clusterWithNewFailureDomainUpdated.DeepCopy(),
			infraRef:             generateInfraCluster(true),
			contract:             "v1beta2",
			expectFailureDomains: newFailureDomain,
		},
		{
			name:                 "expect failure domain to be reset if infra config have different failure domain",
			cluster:              clusterWithOldFailureDomain.DeepCopy(),
			infraRef:             generateInfraCluster(true),
			contract:             "v1beta2",
			expectFailureDomains: newFailureDomain,
		},
		{
			name:                 "expect failure domain to remain same if infra config have same failure domain (v1beta1)",
			cluster:              cluster.DeepCopy(),
			infraRef:             generateInfraClusterV1Beta1(true),
			contract:             "v1beta1",
			expectFailureDomains: newFailureDomain,
		},
		{
			name:                 "expect failure domain to be updated if infra config has updates to failure domain (v1beta1)",
			cluster:              clusterWithNewFailureDomainUpdated.DeepCopy(),
			infraRef:             generateInfraClusterV1Beta1(true),
			contract:             "v1beta1",
			expectFailureDomains: newFailureDomain,
		},
		{
			name:                 "expect failure domain to be reset if infra config have different failure domain (v1beta1)",
			cluster:              clusterWithOldFailureDomain.DeepCopy(),
			infraRef:             generateInfraClusterV1Beta1(true),
			contract:             "v1beta1",
			expectFailureDomains: newFailureDomain,
		},
		{
			name:      "expect error because InfraCluster uses v1beta2 but contract is v1beta1",
			cluster:   clusterWithOldFailureDomain.DeepCopy(),
			infraRef:  generateInfraCluster(true),
			contract:  "v1beta1",
			expectErr: true,
		},
		{
			name:      "expect error because InfraCluster uses v1beta1 but contract is v1beta2",
			cluster:   clusterWithOldFailureDomain.DeepCopy(),
			infraRef:  generateInfraClusterV1Beta1(true),
			contract:  "v1beta2",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			crd := builder.GenericInfrastructureClusterCRD.DeepCopy()
			crd.Labels = map[string]string{
				// Set contract label for tt.contract.
				fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, tt.contract): clusterv1.GroupVersionInfrastructure.Version,
			}

			objs := []client.Object{crd, tt.cluster}
			if tt.infraRef != nil {
				objs = append(objs, &unstructured.Unstructured{Object: tt.infraRef})
			}

			c := fake.NewClientBuilder().WithObjects(objs...).Build()
			r := &Reconciler{
				Client:   c,
				recorder: record.NewFakeRecorder(32),
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          c.Scheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}

			s := &scope{
				cluster: tt.cluster,
			}
			_, err := r.reconcileInfrastructure(ctx, s)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(tt.cluster.Status.FailureDomains).To(BeEquivalentTo(tt.expectFailureDomains))
		})
	}
}

func generateInfraCluster(withFailureDomain bool) map[string]interface{} {
	infraRef := map[string]interface{}{
		"kind":       "GenericInfrastructureCluster",
		"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
		"metadata": map[string]interface{}{
			"name":              "test",
			"namespace":         "test-namespace",
			"deletionTimestamp": "sometime",
		},
		"status": map[string]interface{}{
			"ready": true,
		},
	}

	if withFailureDomain {
		infraRef["status"] = map[string]interface{}{
			"failureDomains": []interface{}{
				map[string]interface{}{
					"name":         "newdomain",
					"controlPlane": false,
					"attributes": map[string]interface{}{
						"attribute1": "value1",
					},
				},
			},
			"ready": true,
		}
	}

	return infraRef
}

func generateInfraClusterV1Beta1(withFailureDomain bool) map[string]interface{} {
	infraRef := map[string]interface{}{
		"kind":       "GenericInfrastructureCluster",
		"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
		"metadata": map[string]interface{}{
			"name":              "test",
			"namespace":         "test-namespace",
			"deletionTimestamp": "sometime",
		},
		"status": map[string]interface{}{
			"ready": true,
		},
	}

	if withFailureDomain {
		infraRef["status"] = map[string]interface{}{
			"failureDomains": map[string]interface{}{
				"newdomain": map[string]interface{}{
					"controlPlane": false,
					"attributes": map[string]interface{}{
						"attribute1": "value1",
					},
				},
			},
			"ready": true,
		}
	}

	return infraRef
}
