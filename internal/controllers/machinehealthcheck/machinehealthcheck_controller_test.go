/*
Copyright 2020 The Kubernetes Authors.

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

package machinehealthcheck

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/internal/webhooks"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMachineHealthCheck_Reconcile(t *testing.T) {
	ns, err := env.CreateNamespace(ctx, "test-mhc")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := env.Delete(ctx, ns); err != nil {
			t.Fatal(err)
		}
	}()

	t.Run("it should ensure the correct cluster-name label when no existing labels exist", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Labels = map[string]string{}

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		g.Eventually(func() map[string]string {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return mhc.GetLabels()
		}).Should(HaveKeyWithValue(clusterv1.ClusterNameLabel, cluster.Name))
	})

	t.Run("it should ensure the correct cluster-name label when the label has the wrong value", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Labels = map[string]string{
			clusterv1.ClusterNameLabel: "wrong-cluster",
		}

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		g.Eventually(func() map[string]string {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return mhc.GetLabels()
		}).Should(HaveKeyWithValue(clusterv1.ClusterNameLabel, cluster.Name))
	})

	t.Run("it should ensure the correct cluster-name label when other labels are present", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Labels = map[string]string{
			"extra-label": "1",
		}

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		g.Eventually(func() map[string]string {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return mhc.GetLabels()
		}).Should(And(
			HaveKeyWithValue(clusterv1.ClusterNameLabel, cluster.Name),
			HaveKeyWithValue("extra-label", "1"),
			HaveLen(2),
		))
	})

	t.Run("it should ensure an owner reference is present when no existing ones exist", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.OwnerReferences = []metav1.OwnerReference{}

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		g.Eventually(func() []metav1.OwnerReference {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				fmt.Printf("error cannot retrieve mhc in ctx: %v", err)
				return nil
			}
			return mhc.GetOwnerReferences()
		}, timeout, 100*time.Millisecond).Should(And(
			HaveLen(1),
			ContainElement(ownerReferenceForCluster(ctx, g, cluster)),
		))
	})

	t.Run("it should ensure an owner reference is present when modifying existing ones", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.OwnerReferences = []metav1.OwnerReference{
			{Kind: "Foo", APIVersion: "foo.bar.baz/v1", Name: "Bar", UID: "12345"},
		}

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		g.Eventually(func() []metav1.OwnerReference {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return mhc.GetOwnerReferences()
		}, timeout, 100*time.Millisecond).Should(And(
			ContainElements(
				metav1.OwnerReference{Kind: "Foo", APIVersion: "foo.bar.baz/v1", Name: "Bar", UID: "12345"},
				ownerReferenceForCluster(ctx, g, cluster)),
			HaveLen(2),
		))
	})

	t.Run("it ignores Machines not matching the label selector", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines matching the MHC's label selector.
		_, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(2),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Healthy nodes and machines NOT matching the MHC's label selector.
		_, _, cleanup2 := createMachinesWithNodes(g, cluster,
			count(2),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
		)
		defer cleanup2()

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}, 5*time.Second, 100*time.Millisecond).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](2),
			CurrentHealthy:      ptr.To[int32](2),
			RemediationsAllowed: ptr.To[int32](2),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))
	})

	t.Run("it doesn't mark anything unhealthy when cluster infrastructure is not ready", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		patchHelper, err := patch.NewHelper(cluster, env.Client)
		g.Expect(err).ToNot(HaveOccurred())

		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterInfrastructureReadyCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterControlPlaneNotInitializedReason,
		})
		g.Expect(patchHelper.Patch(ctx, cluster)).To(Succeed())

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(2),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](2),
			CurrentHealthy:      ptr.To[int32](2),
			RemediationsAllowed: ptr.To[int32](2),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 0)
	})

	t.Run("it doesn't mark anything unhealthy when all Machines are healthy", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(2),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](2),
			CurrentHealthy:      ptr.To[int32](2),
			RemediationsAllowed: ptr.To[int32](2),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 0)
	})

	t.Run("it marks unhealthy machines for remediation when there is one unhealthy Machine and skips deleting machines", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(2),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Unhealthy nodes and machines.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionUnknown),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup2()
		// Unhealthy nodes and machines but already in deletion.
		// Note: deletionTimestamp gets set by deletion below which also removes the skip remediation annotation.
		_, unhealthyMachinesDeleting, cleanup3 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionUnknown),
			machineLabels(mhc.Spec.Selector.MatchLabels),
			machineFinalizers("test.cluster.io/delete-protection"),
			machineAnnotations(map[string]string{clusterv1.MachineSkipRemediationAnnotation: ""}),
		)
		defer cleanup3()
		// Mark machines for deletion and drop skip remediation annotation
		// Note: without the skip remediation annotation the MHC controller might already reconcile the condition leading to a flaky test.
		for _, m := range unhealthyMachinesDeleting {
			g.Expect(env.Delete(ctx, m)).To(Succeed())
			g.Expect(env.Get(ctx, client.ObjectKeyFromObject(m), m)).To(Succeed())
			deletingMachinePatchHelper, err := patch.NewHelper(m, env.GetClient())
			g.Expect(err).ToNot(HaveOccurred())
			m.Annotations = map[string]string{}
			g.Expect(deletingMachinePatchHelper.Patch(ctx, m)).To(Succeed())
		}
		machines = append(append(machines, unhealthyMachines...), unhealthyMachinesDeleting...)
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](4),
			CurrentHealthy:      ptr.To[int32](2),
			RemediationsAllowed: ptr.To[int32](2),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 2)
		assertMachinesOwnerRemediated(g, mhc, 1)
	})

	t.Run("it marks unhealthy machines for remediation when there a Machine has a failure reason", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(2),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Machine with failure reason.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
			machineAnnotations(map[string]string{clusterv1.RemediateMachineAnnotation: ""}),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](3),
			CurrentHealthy:      ptr.To[int32](2),
			RemediationsAllowed: ptr.To[int32](2),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		g.Eventually(func() (unhealthy int) {
			machines := &clusterv1.MachineList{}
			err := env.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if !conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSucceededCondition) {
					continue
				}

				unhealthy++
			}
			return
		}).Should(Equal(1))

		g.Eventually(func() (ownerRemediated int) {
			machines := &clusterv1.MachineList{}
			err := env.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if !conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSucceededCondition) {
					continue
				}
				if !conditions.Has(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) {
					continue
				}

				ownerRemediated++
			}
			return
		}).Should(Equal(1))
	})

	t.Run("it marks unhealthy machines for remediation when there a Machine has a failure message", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(2),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Machine with failure message.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
			machineAnnotations(map[string]string{clusterv1.RemediateMachineAnnotation: ""}),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](3),
			CurrentHealthy:      ptr.To[int32](2),
			RemediationsAllowed: ptr.To[int32](2),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		g.Eventually(func() (unhealthy int) {
			machines := &clusterv1.MachineList{}
			err := env.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if !conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSucceededCondition) {
					continue
				}

				unhealthy++
			}
			return
		}).Should(Equal(1))

		g.Eventually(func() (ownerRemediated int) {
			machines := &clusterv1.MachineList{}
			err := env.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}

			for i := range machines.Items {
				if !conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSucceededCondition) {
					continue
				}
				if !conditions.Has(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) {
					continue
				}

				ownerRemediated++
			}
			return
		}).Should(Equal(1))
	})

	t.Run("it marks unhealthy machines as unhealthy but not for remediation when the unhealthy Machines exceed UnhealthyLessThanOrEqualTo", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		maxUnhealthy := intstr.Parse("40%")
		mhc.Spec.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo = &maxUnhealthy

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(1),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Unhealthy nodes and machines.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(2),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionUnknown),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](3),
			CurrentHealthy:      ptr.To[int32](1),
			RemediationsAllowed: ptr.To[int32](0),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:     clusterv1.RemediationAllowedV1Beta1Condition,
							Status:   corev1.ConditionFalse,
							Severity: clusterv1.ConditionSeverityWarning,
							Reason:   clusterv1.TooManyUnhealthyV1Beta1Reason,
							Message:  "Remediation is not allowed, the number of not started or unhealthy machines exceeds maxUnhealthy (total: 3, unhealthy: 2, maxUnhealthy: 40%)",
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineHealthCheckTooManyUnhealthyReason,
					Message: "Remediation is not allowed, the number of not started or unhealthy machines exceeds maxUnhealthy (total: 3, unhealthy: 2, maxUnhealthy: 40%)",
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 2)
		assertMachinesOwnerRemediated(g, mhc, 0)
	})

	t.Run("it marks unhealthy machines for remediation when number of unhealthy machines is within unhealthyRange", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		unhealthyRange := "[1-3]"
		mhc.Spec.Remediation.TriggerIf.UnhealthyInRange = unhealthyRange

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(2),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Unhealthy nodes and machines.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionUnknown),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](3),
			CurrentHealthy:      ptr.To[int32](2),
			RemediationsAllowed: ptr.To[int32](2),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 1)
		assertMachinesOwnerRemediated(g, mhc, 1)
	})

	t.Run("it marks unhealthy machines as unhealthy but not for remediation when the unhealthy Machines is not within UnhealthyInRange", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		unhealthyRange := "[3-5]"
		mhc.Spec.Remediation.TriggerIf.UnhealthyInRange = unhealthyRange

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(1),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Unhealthy nodes and machines.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(2),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionUnknown),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](3),
			CurrentHealthy:      ptr.To[int32](1),
			RemediationsAllowed: ptr.To[int32](0),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:     clusterv1.RemediationAllowedV1Beta1Condition,
							Status:   corev1.ConditionFalse,
							Severity: clusterv1.ConditionSeverityWarning,
							Reason:   clusterv1.TooManyUnhealthyV1Beta1Reason,
							Message:  "Remediation is not allowed, the number of not started or unhealthy machines does not fall within the range (total: 3, unhealthy: 2, unhealthyRange: [3-5])",
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineHealthCheckTooManyUnhealthyReason,
					Message: "Remediation is not allowed, the number of not started or unhealthy machines does not fall within the range (total: 3, unhealthy: 2, unhealthyRange: [3-5])",
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 2)
		assertMachinesOwnerRemediated(g, mhc, 0)
	})

	t.Run("when a Machine has no Node ref for less than the NodeStartupTimeoutSeconds", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		// After the cluster exists, we have to set the infrastructure ready condition; otherwise, MachineHealthChecks
		// will never fail when nodeStartupTimeout is exceeded.
		patchHelper, err := patch.NewHelper(cluster, env.GetClient())
		g.Expect(err).ToNot(HaveOccurred())

		v1beta1conditions.MarkTrue(cluster, clusterv1.InfrastructureReadyV1Beta1Condition)
		g.Expect(patchHelper.Patch(ctx, cluster)).To(Succeed())

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Spec.Checks.NodeStartupTimeoutSeconds = ptr.To(int32(5 * 60 * 60))

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(2),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Unhealthy nodes and machines.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(false),
			nodeStatus(corev1.ConditionUnknown),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](3),
			CurrentHealthy:      ptr.To[int32](2),
			RemediationsAllowed: ptr.To[int32](2),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 0)
		assertMachinesOwnerRemediated(g, mhc, 0)
	})

	t.Run("when a Machine has no Node ref for longer than the NodeStartupTimeoutSeconds", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Spec.Checks.NodeStartupTimeoutSeconds = ptr.To(int32(10))

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(2),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()
		// Unhealthy nodes and machines.
		_, unhealthyMachines, cleanup2 := createMachinesWithNodes(g, cluster,
			count(1),
			createNodeRefForMachine(false),
			nodeStatus(corev1.ConditionUnknown),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup2()
		machines = append(machines, unhealthyMachines...)

		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the MHC status matches. We have two healthy machines and
		// one unhealthy.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				fmt.Printf("error retrieving mhc: %v", err)
				return nil
			}
			return &mhc.Status
		}, timeout, 100*time.Millisecond).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](3),
			CurrentHealthy:      ptr.To[int32](2),
			RemediationsAllowed: ptr.To[int32](2),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 1)
		assertMachinesOwnerRemediated(g, mhc, 1)
	})

	t.Run("when a Machine's Node has gone away", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Spec.Checks.NodeStartupTimeoutSeconds = ptr.To(int32(10))

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		nodes, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(3),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Forcibly remove the last machine's node.
		g.Eventually(func() bool {
			nodeToBeRemoved := nodes[2]
			if err := env.Delete(ctx, nodeToBeRemoved); err != nil {
				return apierrors.IsNotFound(err)
			}
			return apierrors.IsNotFound(env.Get(ctx, util.ObjectKey(nodeToBeRemoved), nodeToBeRemoved))
		}).Should(BeTrue())

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](3),
			CurrentHealthy:      ptr.To[int32](2),
			RemediationsAllowed: ptr.To[int32](2),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 1)
		assertMachinesOwnerRemediated(g, mhc, 1)
	})

	t.Run("Machine's Node without conditions", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Spec.Checks.UnhealthyNodeConditions = nil

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		_, machines, cleanup1 := createMachinesWithNodes(g, cluster,
			count(3),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup1()

		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](3),
			CurrentHealthy:      ptr.To[int32](3),
			RemediationsAllowed: ptr.To[int32](3),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 0)
		assertMachinesOwnerRemediated(g, mhc, 0)
	})

	t.Run("should react when a Node transitions to unhealthy", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		nodes, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(1),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](1),
			CurrentHealthy:      ptr.To[int32](1),
			RemediationsAllowed: ptr.To[int32](1),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 0)
		assertMachinesOwnerRemediated(g, mhc, 0)

		// Transition the node to unhealthy.
		node := nodes[0]
		nodePatch := client.MergeFrom(node.DeepCopy())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:               corev1.NodeReady,
				Status:             corev1.ConditionUnknown,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			},
		}
		g.Expect(env.Status().Patch(ctx, node, nodePatch)).To(Succeed())

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](1),
			CurrentHealthy:      ptr.To[int32](0),
			RemediationsAllowed: ptr.To[int32](0),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 1)
		assertMachinesOwnerRemediated(g, mhc, 1)
	})

	t.Run("when in a MachineSet, unhealthy machines should be deleted", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		// Create 1 control plane machine so MHC can proceed
		_, _, cleanup := createMachinesWithNodes(g, cluster,
			count(1),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
		)
		defer cleanup()

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		// Create infrastructure template resource.
		infraResource := map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
			"metadata":   map[string]interface{}{},
			"spec": map[string]interface{}{
				"size": "3xlarge",
			},
		}
		infraTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": infraResource,
				},
			},
		}
		infraTmpl.SetKind("GenericInfrastructureMachineTemplate")
		infraTmpl.SetAPIVersion(clusterv1.GroupVersionInfrastructure.String())
		infraTmpl.SetGenerateName("mhc-ms-template-")
		infraTmpl.SetNamespace(mhc.Namespace)

		g.Expect(env.Create(ctx, infraTmpl)).To(Succeed())

		machineSet := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "mhc-ms-",
				Namespace:    mhc.Namespace,
			},
			Spec: clusterv1.MachineSetSpec{
				ClusterName: cluster.Name,
				Replicas:    ptr.To[int32](1),
				Selector:    mhc.Spec.Selector,
				Template: clusterv1.MachineTemplateSpec{
					ObjectMeta: clusterv1.ObjectMeta{
						Labels: mhc.Spec.Selector.MatchLabels,
					},
					Spec: clusterv1.MachineSpec{
						ClusterName: cluster.Name,
						Bootstrap: clusterv1.Bootstrap{
							DataSecretName: ptr.To("test-data-secret-name"),
						},
						InfrastructureRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionInfrastructure.Group,
							Kind:     "GenericInfrastructureMachineTemplate",
							Name:     infraTmpl.GetName(),
						},
					},
				},
			},
		}

		reqCtx := admission.NewContextWithRequest(ctx, admission.Request{})
		g.Expect((&webhooks.MachineSet{}).Default(reqCtx, machineSet)).Should(Succeed())
		g.Expect(env.Create(ctx, machineSet)).To(Succeed())

		// Ensure machines have been created.
		g.Eventually(func() int {
			machines := &clusterv1.MachineList{}
			err := env.List(ctx, machines, client.MatchingLabels{
				"selector": mhc.Spec.Selector.MatchLabels["selector"],
			})
			if err != nil {
				return -1
			}
			return len(machines.Items)
		}, timeout, 100*time.Millisecond).Should(Equal(1))

		// Create the MachineHealthCheck instance.
		mhc.Spec.Checks.NodeStartupTimeoutSeconds = ptr.To(int32(1))

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		// defer cleanup for all the objects that have been created
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc, infraTmpl, machineSet)

		// Pause the MachineSet reconciler to delay the deletion of the
		// Machine, because the MachineSet controller deletes the Machine when
		// it is marked unhealthy by MHC.
		machineSetPatch := client.MergeFrom(machineSet.DeepCopy())
		machineSet.Annotations = map[string]string{
			clusterv1.PausedAnnotation: "",
		}
		g.Expect(env.Patch(ctx, machineSet, machineSetPatch)).To(Succeed())

		assertMachinesNotHealthy(g, mhc, 1)
		assertMachinesOwnerRemediated(g, mhc, 1)

		var unhealthyMachine *clusterv1.Machine
		machines := &clusterv1.MachineList{}
		g.Expect(env.List(ctx, machines, client.MatchingLabels{
			"selector": mhc.Spec.Selector.MatchLabels["selector"],
		})).To(Succeed())

		for i := range machines.Items {
			if conditions.Get(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) != nil {
				unhealthyMachine = machines.Items[i].DeepCopy()
			}
		}

		// Unpause the MachineSet reconciler.
		machineSetPatch = client.MergeFrom(machineSet.DeepCopy())
		delete(machineSet.Annotations, clusterv1.PausedAnnotation)
		g.Expect(env.Patch(ctx, machineSet, machineSetPatch)).To(Succeed())

		// Make sure the Machine gets deleted.
		g.Eventually(func() bool {
			machine := unhealthyMachine.DeepCopy()
			err := env.Get(ctx, util.ObjectKey(unhealthyMachine), machine)
			return apierrors.IsNotFound(err) || !machine.DeletionTimestamp.IsZero()
		}, timeout, 100*time.Millisecond).Should(BeTrue())
	})

	t.Run("when a machine is paused", func(t *testing.T) {
		// TODO: Resolve flaky/failing test
		t.Skip("skipping until made stable")
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)

		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc)

		// Healthy nodes and machines.
		nodes, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(1),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:   ptr.To[int32](1),
			CurrentHealthy:     ptr.To[int32](1),
			ObservedGeneration: 1,
			Targets:            targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		// Pause the machine
		machinePatch := client.MergeFrom(machines[0].DeepCopy())
		machines[0].Annotations = map[string]string{
			clusterv1.PausedAnnotation: "",
		}
		g.Expect(env.Patch(ctx, machines[0], machinePatch)).To(Succeed())

		// Transition the node to unhealthy.
		node := nodes[0]
		nodePatch := client.MergeFrom(node.DeepCopy())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:               corev1.NodeReady,
				Status:             corev1.ConditionUnknown,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			},
		}
		g.Expect(env.Status().Patch(ctx, node, nodePatch)).To(Succeed())

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](1),
			CurrentHealthy:      ptr.To[int32](0),
			RemediationsAllowed: ptr.To[int32](0),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 1)
		assertMachinesOwnerRemediated(g, mhc, 0)
	})

	t.Run("When remediationTemplate is set and node transitions to unhealthy, new Remediation Request should be created", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		// Create remediation template resource.
		infraRemediationResource := map[string]interface{}{
			"kind":       "GenericExternalRemediation",
			"apiVersion": builder.RemediationGroupVersion.String(),
			"metadata":   map[string]interface{}{},
			"spec": map[string]interface{}{
				"size": "3xlarge",
			},
		}
		infraRemediationTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": infraRemediationResource,
				},
			},
		}
		infraRemediationTmpl.SetKind("GenericExternalRemediationTemplate")
		infraRemediationTmpl.SetAPIVersion(builder.RemediationGroupVersion.String())
		infraRemediationTmpl.SetGenerateName("remediation-template-name-")
		infraRemediationTmpl.SetNamespace(cluster.Namespace)
		g.Expect(env.Create(ctx, infraRemediationTmpl)).To(Succeed())

		remediationTemplate := clusterv1.MachineHealthCheckRemediationTemplateReference{
			APIVersion: builder.RemediationGroupVersion.String(),
			Kind:       "GenericExternalRemediationTemplate",
			Name:       infraRemediationTmpl.GetName(),
		}

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Spec.Remediation.TemplateRef = remediationTemplate
		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc, infraRemediationTmpl)

		// Healthy nodes and machines.
		nodes, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(1),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](1),
			CurrentHealthy:      ptr.To[int32](1),
			RemediationsAllowed: ptr.To[int32](1),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 0)
		assertMachinesOwnerRemediated(g, mhc, 0)

		// Transition the node to unhealthy.
		node := nodes[0]
		nodePatch := client.MergeFrom(node.DeepCopy())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:               corev1.NodeReady,
				Status:             corev1.ConditionUnknown,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			},
		}
		g.Expect(env.Status().Patch(ctx, node, nodePatch)).To(Succeed())

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](1),
			CurrentHealthy:      ptr.To[int32](0),
			RemediationsAllowed: ptr.To[int32](0),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 1)
		assertMachinesOwnerRemediated(g, mhc, 0)

		ref := corev1.ObjectReference{
			APIVersion: builder.RemediationGroupVersion.String(),
			Kind:       "GenericExternalRemediation",
		}

		obj := util.ObjectReferenceToUnstructured(ref)
		// Make sure the Remeditaion Request is created.
		g.Eventually(func() *unstructured.Unstructured {
			key := client.ObjectKey{
				Namespace: machines[0].Namespace,
				Name:      machines[0].Name,
			}
			err := env.Get(ctx, key, obj)
			if err != nil {
				return nil
			}
			return obj
		}, timeout, 100*time.Millisecond).ShouldNot(BeNil())
		g.Expect(obj.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(obj.GetOwnerReferences()[0].Name).To(Equal(machines[0].Name))
	})

	t.Run("When remediationTemplate is set and node transitions back to healthy, new Remediation Request should be deleted", func(t *testing.T) {
		g := NewWithT(t)
		cluster := createCluster(g, ns.Name)

		// Create remediation template resource.
		infraRemediationResource := map[string]interface{}{
			"kind":       "GenericExternalRemediation",
			"apiVersion": builder.RemediationGroupVersion.String(),
			"metadata":   map[string]interface{}{},
			"spec": map[string]interface{}{
				"size": "3xlarge",
			},
		}
		infraRemediationTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": infraRemediationResource,
				},
			},
		}
		infraRemediationTmpl.SetKind("GenericExternalRemediationTemplate")
		infraRemediationTmpl.SetAPIVersion(builder.RemediationGroupVersion.String())
		infraRemediationTmpl.SetGenerateName("remediation-template-name-")
		infraRemediationTmpl.SetNamespace(cluster.Namespace)
		g.Expect(env.Create(ctx, infraRemediationTmpl)).To(Succeed())

		remediationTemplate := clusterv1.MachineHealthCheckRemediationTemplateReference{
			APIVersion: builder.RemediationGroupVersion.String(),
			Kind:       "GenericExternalRemediationTemplate",
			Name:       infraRemediationTmpl.GetName(),
		}

		mhc := newMachineHealthCheck(cluster.Namespace, cluster.Name)
		mhc.Spec.Remediation.TemplateRef = remediationTemplate
		g.Expect(env.Create(ctx, mhc)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(cluster, mhc, infraRemediationTmpl)

		// Healthy nodes and machines.
		nodes, machines, cleanup := createMachinesWithNodes(g, cluster,
			count(1),
			firstMachineAsControlPlane(),
			createNodeRefForMachine(true),
			nodeStatus(corev1.ConditionTrue),
			machineLabels(mhc.Spec.Selector.MatchLabels),
		)
		defer cleanup()
		targetMachines := make([]string, len(machines))
		for i, m := range machines {
			targetMachines[i] = m.Name
		}
		sort.Strings(targetMachines)

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](1),
			CurrentHealthy:      ptr.To[int32](1),
			RemediationsAllowed: ptr.To[int32](1),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 0)
		assertMachinesOwnerRemediated(g, mhc, 0)

		// Transition the node to unhealthy.
		node := nodes[0]
		nodePatch := client.MergeFrom(node.DeepCopy())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:               corev1.NodeReady,
				Status:             corev1.ConditionUnknown,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			},
		}
		g.Expect(env.Status().Patch(ctx, node, nodePatch)).To(Succeed())

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](1),
			CurrentHealthy:      ptr.To[int32](0),
			RemediationsAllowed: ptr.To[int32](0),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 1)
		assertMachinesOwnerRemediated(g, mhc, 0)

		// Transition the node back to healthy.
		node = nodes[0]
		nodePatch = client.MergeFrom(node.DeepCopy())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:               corev1.NodeReady,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			},
		}
		g.Expect(env.Status().Patch(ctx, node, nodePatch)).To(Succeed())

		// Make sure the status matches.
		g.Eventually(func() *clusterv1.MachineHealthCheckStatus {
			err := env.Get(ctx, util.ObjectKey(mhc), mhc)
			if err != nil {
				return nil
			}
			return &mhc.Status
		}).Should(MatchMachineHealthCheckStatus(&clusterv1.MachineHealthCheckStatus{
			ExpectedMachines:    ptr.To[int32](1),
			CurrentHealthy:      ptr.To[int32](1),
			RemediationsAllowed: ptr.To[int32](1),
			ObservedGeneration:  1,
			Targets:             targetMachines,
			Deprecated: &clusterv1.MachineHealthCheckDeprecatedStatus{
				V1Beta1: &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.RemediationAllowedV1Beta1Condition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckRemediationAllowedCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineHealthCheckRemediationAllowedReason,
				},
			},
		}))

		assertMachinesNotHealthy(g, mhc, 0)
		assertMachinesOwnerRemediated(g, mhc, 0)

		ref := corev1.ObjectReference{
			APIVersion: builder.RemediationGroupVersion.String(),
			Kind:       "GenericExternalRemediation",
		}

		obj := util.ObjectReferenceToUnstructured(ref)
		// Make sure the Remediation Request is deleted.
		g.Eventually(func() *unstructured.Unstructured {
			key := client.ObjectKey{
				Namespace: machines[0].Namespace,
				Name:      machines[0].Name,
			}
			err := env.Get(ctx, key, obj)
			if err != nil {
				return nil
			}
			return obj
		}, timeout, 100*time.Millisecond).Should(BeNil())
	})
}

func assertMachinesNotHealthy(g *WithT, mhc *clusterv1.MachineHealthCheck, expectNotHealthy int) {
	g.Eventually(func() (unhealthy int) {
		machines := &clusterv1.MachineList{}
		err := env.List(ctx, machines, client.MatchingLabels{
			"selector": mhc.Spec.Selector.MatchLabels["selector"],
		})
		if err != nil {
			return -1
		}

		for i := range machines.Items {
			if !v1beta1conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSucceededV1Beta1Condition) {
				continue
			}
			if c := conditions.Get(&machines.Items[i], clusterv1.MachineHealthCheckSucceededCondition); c == nil || c.Status != metav1.ConditionFalse {
				continue
			}

			unhealthy++
		}
		return
	}).Should(Equal(expectNotHealthy))
}

func assertMachinesOwnerRemediated(g *WithT, mhc *clusterv1.MachineHealthCheck, expectOwnerRemediated int) {
	// Calculate how many Machines have health check succeeded = false.
	g.Eventually(func() (ownerRemediated int) {
		machines := &clusterv1.MachineList{}
		err := env.List(ctx, machines, client.MatchingLabels{
			"selector": mhc.Spec.Selector.MatchLabels["selector"],
		})
		if err != nil {
			return -1
		}

		for i := range machines.Items {
			if !v1beta1conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSucceededV1Beta1Condition) {
				continue
			}
			if !v1beta1conditions.Has(&machines.Items[i], clusterv1.MachineOwnerRemediatedV1Beta1Condition) {
				continue
			}

			if !conditions.IsFalse(&machines.Items[i], clusterv1.MachineHealthCheckSucceededCondition) {
				continue
			}
			if !conditions.Has(&machines.Items[i], clusterv1.MachineOwnerRemediatedCondition) {
				continue
			}

			ownerRemediated++
		}
		return
	}).Should(Equal(expectOwnerRemediated))
}

func TestClusterToMachineHealthCheck(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()

	r := &Reconciler{
		Client: fakeClient,
	}

	namespace := metav1.NamespaceDefault
	clusterName := testClusterName
	labels := make(map[string]string)

	mhc1 := newMachineHealthCheckWithLabels("mhc1", namespace, clusterName, labels)
	mhc1Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc1.Namespace, Name: mhc1.Name}}
	mhc2 := newMachineHealthCheckWithLabels("mhc2", namespace, clusterName, labels)
	mhc2Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc2.Namespace, Name: mhc2.Name}}
	mhc3 := newMachineHealthCheckWithLabels("mhc3", namespace, "othercluster", labels)
	mhc4 := newMachineHealthCheckWithLabels("mhc4", "othernamespace", clusterName, labels)
	cluster1 := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
	}

	testCases := []struct {
		name     string
		toCreate []clusterv1.MachineHealthCheck
		object   client.Object
		expected []reconcile.Request
	}{
		{
			name:     "when a MachineHealthCheck exists for the Cluster in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1},
			object:   cluster1,
			expected: []reconcile.Request{mhc1Req},
		},
		{
			name:     "when 2 MachineHealthChecks exists for the Cluster in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1, *mhc2},
			object:   cluster1,
			expected: []reconcile.Request{mhc1Req, mhc2Req},
		},
		{
			name:     "when a MachineHealthCheck exists for another Cluster in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc3},
			object:   cluster1,
			expected: []reconcile.Request{},
		},
		{
			name:     "when a MachineHealthCheck exists for another Cluster in another namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc4},
			object:   cluster1,
			expected: []reconcile.Request{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			for _, obj := range tc.toCreate {
				o := obj
				gs.Expect(r.Client.Create(ctx, &o)).To(Succeed())
				defer func() {
					gs.Expect(r.Client.Delete(ctx, &o)).To(Succeed())
				}()
				// Check the cache is populated
				getObj := func() error {
					return r.Client.Get(ctx, util.ObjectKey(&o), &clusterv1.MachineHealthCheck{})
				}
				gs.Eventually(getObj).Should(Succeed())
			}

			got := r.clusterToMachineHealthCheck(ctx, tc.object)
			gs.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestMachineToMachineHealthCheck(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()

	r := &Reconciler{
		Client: fakeClient,
	}

	namespace := metav1.NamespaceDefault
	clusterName := testClusterName
	nodeName := "node1"
	labels := map[string]string{"cluster": "foo", "nodepool": "bar"}

	mhc1 := newMachineHealthCheckWithLabels("mhc1", namespace, clusterName, labels)
	mhc1Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc1.Namespace, Name: mhc1.Name}}
	mhc2 := newMachineHealthCheckWithLabels("mhc2", namespace, clusterName, labels)
	mhc2Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc2.Namespace, Name: mhc2.Name}}
	mhc3 := newMachineHealthCheckWithLabels("mhc3", namespace, clusterName, map[string]string{"cluster": "foo", "nodepool": "other"})
	mhc4 := newMachineHealthCheckWithLabels("mhc4", "othernamespace", clusterName, labels)
	machine1 := newTestMachine("machine1", namespace, clusterName, nodeName, labels)

	testCases := []struct {
		name     string
		toCreate []clusterv1.MachineHealthCheck
		object   client.Object
		expected []reconcile.Request
	}{
		{
			name:     "when a MachineHealthCheck matches labels for the Machine in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1},
			object:   machine1,
			expected: []reconcile.Request{mhc1Req},
		},
		{
			name:     "when 2 MachineHealthChecks match labels for the Machine in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc1, *mhc2},
			object:   machine1,
			expected: []reconcile.Request{mhc1Req, mhc2Req},
		},
		{
			name:     "when a MachineHealthCheck does not match labels for the Machine in the same namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc3},
			object:   machine1,
			expected: []reconcile.Request{},
		},
		{
			name:     "when a MachineHealthCheck matches labels for the Machine in another namespace",
			toCreate: []clusterv1.MachineHealthCheck{*mhc4},
			object:   machine1,
			expected: []reconcile.Request{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			for _, obj := range tc.toCreate {
				o := obj
				gs.Expect(r.Client.Create(ctx, &o)).To(Succeed())
				defer func() {
					gs.Expect(r.Client.Delete(ctx, &o)).To(Succeed())
				}()
				// Check the cache is populated
				getObj := func() error {
					return r.Client.Get(ctx, util.ObjectKey(&o), &clusterv1.MachineHealthCheck{})
				}
				gs.Eventually(getObj).Should(Succeed())
			}

			got := r.machineToMachineHealthCheck(ctx, tc.object)
			gs.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestNodeToMachineHealthCheck(t *testing.T) {
	fakeClient := fake.NewClientBuilder().
		WithIndex(&clusterv1.Machine{}, index.MachineNodeNameField, index.MachineByNodeName).
		WithStatusSubresource(&clusterv1.MachineHealthCheck{}, &clusterv1.Machine{}).
		Build()

	r := &Reconciler{
		Client: fakeClient,
	}

	namespace := metav1.NamespaceDefault
	clusterName := testClusterName
	nodeName := "node1"
	labels := map[string]string{"cluster": "foo", "nodepool": "bar"}

	mhc1 := newMachineHealthCheckWithLabels("mhc1", namespace, clusterName, labels)
	mhc1Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc1.Namespace, Name: mhc1.Name}}
	mhc2 := newMachineHealthCheckWithLabels("mhc2", namespace, clusterName, labels)
	mhc2Req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: mhc2.Namespace, Name: mhc2.Name}}
	mhc3 := newMachineHealthCheckWithLabels("mhc3", namespace, "othercluster", labels)
	mhc4 := newMachineHealthCheckWithLabels("mhc4", "othernamespace", clusterName, labels)

	machine1 := newTestMachine("machine1", namespace, clusterName, nodeName, labels)
	machine2 := newTestMachine("machine2", namespace, clusterName, nodeName, labels)

	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
	}

	testCases := []struct {
		name        string
		mhcToCreate []clusterv1.MachineHealthCheck
		mToCreate   []clusterv1.Machine
		object      client.Object
		expected    []reconcile.Request
	}{
		{
			name:        "when no Machine exists for the Node",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1},
			mToCreate:   []clusterv1.Machine{},
			object:      node1,
			expected:    []reconcile.Request{},
		},
		{
			name:        "when two Machines exist for the Node",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1},
			mToCreate:   []clusterv1.Machine{*machine1, *machine2},
			object:      node1,
			expected:    []reconcile.Request{},
		},
		{
			name:        "when no MachineHealthCheck exists for the Node in the Machine's namespace",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc4},
			mToCreate:   []clusterv1.Machine{*machine1},
			object:      node1,
			expected:    []reconcile.Request{},
		},
		{
			name:        "when a MachineHealthCheck exists for the Node in the Machine's namespace",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1},
			mToCreate:   []clusterv1.Machine{*machine1},
			object:      node1,
			expected:    []reconcile.Request{mhc1Req},
		},
		{
			name:        "when two MachineHealthChecks exist for the Node in the Machine's namespace",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc1, *mhc2},
			mToCreate:   []clusterv1.Machine{*machine1},
			object:      node1,
			expected:    []reconcile.Request{mhc1Req, mhc2Req},
		},
		{
			name:        "when a MachineHealthCheck exists for the Node, but not in the Machine's cluster",
			mhcToCreate: []clusterv1.MachineHealthCheck{*mhc3},
			mToCreate:   []clusterv1.Machine{*machine1},
			object:      node1,
			expected:    []reconcile.Request{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			for _, obj := range tc.mhcToCreate {
				o := obj
				gs.Expect(r.Client.Create(ctx, &o)).To(Succeed())
				defer func() {
					gs.Expect(r.Client.Delete(ctx, &o)).To(Succeed())
				}()
				// Check the cache is populated
				key := util.ObjectKey(&o)
				getObj := func() error {
					return r.Client.Get(ctx, key, &clusterv1.MachineHealthCheck{})
				}
				gs.Eventually(getObj).Should(Succeed())
			}
			for _, obj := range tc.mToCreate {
				o := obj
				gs.Expect(r.Client.Create(ctx, &o)).To(Succeed())
				defer func() {
					gs.Expect(r.Client.Delete(ctx, &o)).To(Succeed())
				}()
				// Ensure the status is set (required for matching node to machine)
				o.Status = obj.Status
				gs.Expect(r.Client.Status().Update(ctx, &o)).To(Succeed())

				// Check the cache is up to date with the status update
				key := util.ObjectKey(&o)
				checkStatus := func() clusterv1.MachineStatus {
					m := &clusterv1.Machine{}
					err := r.Client.Get(ctx, key, m)
					if err != nil {
						return clusterv1.MachineStatus{}
					}
					return m.Status
				}
				gs.Eventually(checkStatus).Should(BeComparableTo(o.Status))
			}

			got := r.nodeToMachineHealthCheck(ctx, tc.object)
			gs.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestIsAllowedRemediation(t *testing.T) {
	testCases := []struct {
		name               string
		maxUnhealthy       *intstr.IntOrString
		expectedMachines   int32
		currentHealthy     int32
		allowed            bool
		observedGeneration int64
	}{
		{
			name:             "when maxUnhealthy is not set",
			maxUnhealthy:     nil,
			expectedMachines: int32(3),
			currentHealthy:   int32(0),
			allowed:          true,
		},
		{
			name:             "when maxUnhealthy is not an int or percentage",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.String, StrVal: "abcdef"},
			expectedMachines: int32(5),
			currentHealthy:   int32(2),
			allowed:          false,
		},
		{
			name:             "when maxUnhealthy is an int less than current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.Int, IntVal: int32(1)},
			expectedMachines: int32(3),
			currentHealthy:   int32(1),
			allowed:          false,
		},
		{
			name:             "when maxUnhealthy is an int equal to current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.Int, IntVal: int32(2)},
			expectedMachines: int32(3),
			currentHealthy:   int32(1),
			allowed:          true,
		},
		{
			name:             "when maxUnhealthy is an int greater than current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.Int, IntVal: int32(3)},
			expectedMachines: int32(3),
			currentHealthy:   int32(1),
			allowed:          true,
		},
		{
			name:             "when maxUnhealthy is a percentage less than current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
			expectedMachines: int32(5),
			currentHealthy:   int32(2),
			allowed:          false,
		},
		{
			name:             "when maxUnhealthy is a percentage equal to current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.String, StrVal: "60%"},
			expectedMachines: int32(5),
			currentHealthy:   int32(2),
			allowed:          true,
		},
		{
			name:             "when maxUnhealthy is a percentage greater than current unhealthy",
			maxUnhealthy:     &intstr.IntOrString{Type: intstr.String, StrVal: "70%"},
			expectedMachines: int32(5),
			currentHealthy:   int32(2),
			allowed:          true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			mhc := &clusterv1.MachineHealthCheck{
				Spec: clusterv1.MachineHealthCheckSpec{
					Checks: clusterv1.MachineHealthCheckChecks{
						NodeStartupTimeoutSeconds: ptr.To(int32(0)),
					},
					Remediation: clusterv1.MachineHealthCheckRemediation{
						TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
							UnhealthyLessThanOrEqualTo: tc.maxUnhealthy,
						},
					},
				},
				Status: clusterv1.MachineHealthCheckStatus{
					ExpectedMachines:   ptr.To(tc.expectedMachines),
					CurrentHealthy:     ptr.To(tc.currentHealthy),
					ObservedGeneration: tc.observedGeneration,
				},
			}

			remediationAllowed, _, _ := isAllowedRemediation(mhc)
			g.Expect(remediationAllowed).To(Equal(tc.allowed))
		})
	}
}

func TestGetMaxUnhealthy(t *testing.T) {
	testCases := []struct {
		name                 string
		maxUnhealthy         *intstr.IntOrString
		expectedMaxUnhealthy int
		actualMachineCount   int32
		expectedErr          error
	}{
		{
			name:                 "when maxUnhealthy is nil",
			maxUnhealthy:         nil,
			expectedMaxUnhealthy: 7,
			actualMachineCount:   7,
		},
		{
			name:                 "when maxUnhealthy is not an int or percentage",
			maxUnhealthy:         &intstr.IntOrString{Type: intstr.String, StrVal: "abcdef"},
			expectedMaxUnhealthy: 0,
			actualMachineCount:   3,
			expectedErr:          errors.New("invalid value for IntOrString: invalid type: string is not a percentage"),
		},
		{
			name:                 "when maxUnhealthy is an int",
			maxUnhealthy:         &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
			actualMachineCount:   2,
			expectedMaxUnhealthy: 3,
			expectedErr:          nil,
		},
		{
			name:                 "when maxUnhealthy is a 40% (of 5)",
			maxUnhealthy:         &intstr.IntOrString{Type: intstr.String, StrVal: "40%"},
			actualMachineCount:   5,
			expectedMaxUnhealthy: 2,
			expectedErr:          nil,
		},
		{
			name:                 "when maxUnhealthy is a 60% (of 7)",
			maxUnhealthy:         &intstr.IntOrString{Type: intstr.String, StrVal: "60%"},
			actualMachineCount:   7,
			expectedMaxUnhealthy: 4,
			expectedErr:          nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			mhc := &clusterv1.MachineHealthCheck{
				Spec: clusterv1.MachineHealthCheckSpec{
					Remediation: clusterv1.MachineHealthCheckRemediation{
						TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
							UnhealthyLessThanOrEqualTo: tc.maxUnhealthy,
						},
					},
				},
				Status: clusterv1.MachineHealthCheckStatus{
					ExpectedMachines: ptr.To(tc.actualMachineCount),
				},
			}

			maxUnhealthy, err := getMaxUnhealthy(mhc)
			if tc.expectedErr != nil {
				g.Expect(err).To(MatchError(tc.expectedErr.Error()))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(maxUnhealthy).To(Equal(tc.expectedMaxUnhealthy))
		})
	}
}

func ownerReferenceForCluster(ctx context.Context, g *WithT, c *clusterv1.Cluster) metav1.OwnerReference {
	// Fetch the cluster to populate the UID
	cc := &clusterv1.Cluster{}
	g.Expect(env.Get(ctx, util.ObjectKey(c), cc)).To(Succeed())

	return metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cc.Name,
		UID:        cc.UID,
	}
}

// createCluster creates a Cluster and KubeconfigSecret for that cluster in said namespace.
func createCluster(g *WithT, namespaceName string) *clusterv1.Cluster {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-cluster-",
			Namespace:    namespaceName,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.ControlPlaneGroupVersion.Group,
				Kind:     builder.GenericControlPlaneKind,
				Name:     "cp1",
			},
		},
	}

	g.Expect(env.CreateAndWait(ctx, cluster)).To(Succeed())

	g.Expect(env.CreateKubeconfigSecret(ctx, cluster)).To(Succeed())

	// This is required for MHC to perform checks
	patchHelper, err := patch.NewHelper(cluster, env.Client)
	g.Expect(err).ToNot(HaveOccurred())

	cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
	conditions.Set(cluster, metav1.Condition{
		Type:   clusterv1.ClusterInfrastructureReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterInfrastructureReadyReason,
	})

	conditions.Set(cluster, metav1.Condition{
		Type:   clusterv1.ClusterControlPlaneInitializedCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterControlPlaneInitializedReason,
	})
	g.Expect(patchHelper.Patch(ctx, cluster)).To(Succeed())

	// Wait for cluster in the cached client to be updated post-patch
	g.Eventually(func(g Gomega) {
		g.Expect(env.Get(ctx, util.ObjectKey(cluster), cluster)).To(Succeed())
		g.Expect(conditions.IsTrue(cluster, clusterv1.ClusterInfrastructureReadyCondition)).To(BeTrue())
	}, timeout, 100*time.Millisecond).Should(Succeed())

	return cluster
}

// newRunningMachine creates a Machine object with a Status.Phase == Running.
func newRunningMachine(c *clusterv1.Cluster, labels map[string]string) *clusterv1.Machine {
	return &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-mhc-machine-",
			Namespace:    c.Namespace,
			Labels:       labels,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: c.Name,
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: ptr.To("data-secret-name"),
			},
		},
		Status: clusterv1.MachineStatus{
			Initialization: clusterv1.MachineInitializationStatus{
				InfrastructureProvisioned:  ptr.To(true),
				BootstrapDataSecretCreated: ptr.To(true),
			},
			Phase:              string(clusterv1.MachinePhaseRunning),
			ObservedGeneration: 1,
		},
	}
}

func newInfraMachine(machine *clusterv1.Machine) (*unstructured.Unstructured, string) {
	providerID := fmt.Sprintf("test:////%v", uuid.NewUUID())
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
			"kind":       "GenericInfrastructureMachine",
			"metadata": map[string]interface{}{
				"generateName": "test-mhc-machine-infra-",
				"namespace":    machine.Namespace,
			},
			"spec": map[string]interface{}{
				"providerID": providerID,
			},
		},
	}, providerID
}

type machinesWithNodes struct {
	count                      int
	nodeStatus                 corev1.ConditionStatus
	createNodeRefForMachine    bool
	firstMachineAsControlPlane bool
	annotations                map[string]string
	labels                     map[string]string
	finalizers                 []string
}

type machineWithNodesOption func(m *machinesWithNodes)

func count(n int) machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.count = n
	}
}

func firstMachineAsControlPlane() machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.firstMachineAsControlPlane = true
	}
}

func nodeStatus(s corev1.ConditionStatus) machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.nodeStatus = s
	}
}

func createNodeRefForMachine(b bool) machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.createNodeRefForMachine = b
	}
}

func machineLabels(l map[string]string) machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.labels = l
	}
}

func machineAnnotations(a map[string]string) machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.annotations = a
	}
}

func machineFinalizers(f ...string) machineWithNodesOption {
	return func(m *machinesWithNodes) {
		m.finalizers = append(m.finalizers, f...)
	}
}

func createMachinesWithNodes(
	g *WithT,
	c *clusterv1.Cluster,
	opts ...machineWithNodesOption,
) ([]*corev1.Node, []*clusterv1.Machine, func()) {
	o := &machinesWithNodes{}
	for _, op := range opts {
		op(o)
	}

	nodes := make([]*corev1.Node, 0, o.count)
	machines := make([]*clusterv1.Machine, 0, o.count)
	infraMachines := make([]*unstructured.Unstructured, 0, o.count)

	for i := range o.count {
		machine := newRunningMachine(c, o.labels)
		if i == 0 && o.firstMachineAsControlPlane {
			if machine.Labels == nil {
				machine.Labels = make(map[string]string)
			}
			machine.Labels[clusterv1.MachineControlPlaneLabel] = ""
		}
		infraMachine, providerID := newInfraMachine(machine)
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		infraMachines = append(infraMachines, infraMachine)
		fmt.Printf("inframachine created: %s\n", infraMachine.GetName())
		// Patch the status of the InfraMachine and mark it as ready.
		// NB. Status cannot be set during object creation so we need to patch
		// it separately.
		infraMachinePatch := client.MergeFrom(infraMachine.DeepCopy())
		g.Expect(unstructured.SetNestedField(infraMachine.Object, true, "status", "initialization", "provisioned")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, infraMachine, infraMachinePatch)).To(Succeed())

		machine.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
			APIGroup: infraMachine.GroupVersionKind().Group,
			Kind:     infraMachine.GetKind(),
			Name:     infraMachine.GetName(),
		}
		if len(o.finalizers) > 0 {
			machine.Finalizers = o.finalizers
		}
		if o.annotations != nil {
			machine.Annotations = o.annotations
		}
		g.Expect(env.Create(ctx, machine)).To(Succeed())
		fmt.Printf("machine created: %s\n", machine.GetName())

		// Before moving on we want to ensure that the machine has a valid
		// status. That is, LastUpdated should be set.
		g.Eventually(func(g Gomega) {
			k := client.ObjectKey{
				Name:      machine.GetName(),
				Namespace: machine.GetNamespace(),
			}
			g.Expect(env.Get(ctx, k, machine)).To(Succeed())
			g.Expect(machine.Status.LastUpdated.IsZero()).To(BeFalse())
		}, timeout, 100*time.Millisecond).Should(Succeed())

		machinePatchHelper, err := patch.NewHelper(machine, env.Client)
		g.Expect(err).ToNot(HaveOccurred())

		if o.createNodeRefForMachine {
			// Create node
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-mhc-node-",
				},
				Spec: corev1.NodeSpec{
					ProviderID: providerID,
				},
			}

			g.Expect(env.Create(ctx, node)).To(Succeed())
			fmt.Printf("node created: %s\n", node.GetName())

			// Patch node status
			nodePatchHelper, err := patch.NewHelper(node, env.Client)
			g.Expect(err).ToNot(HaveOccurred())

			node.Status.Conditions = []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             o.nodeStatus,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
				},
			}

			g.Expect(nodePatchHelper.Patch(ctx, node)).To(Succeed())

			nodes = append(nodes, node)

			machine.Status.NodeRef = clusterv1.MachineNodeReference{
				Name: node.Name,
			}
		}

		// Adding one second to ensure there is a difference from the
		// original time so that the patch works. That is, ensure the
		// precision isn't lost during conversions.
		lastUp := metav1.NewTime(machine.Status.LastUpdated.Add(time.Second))
		machine.Status.LastUpdated = lastUp

		// Patch the machine to record the status changes
		g.Expect(machinePatchHelper.Patch(ctx, machine)).To(Succeed())

		machines = append(machines, machine)
	}

	cleanup := func() {
		fmt.Println("Cleaning up nodes, machines and infra machines.")
		for _, n := range nodes {
			if err := env.Delete(ctx, n); !apierrors.IsNotFound(err) {
				g.Expect(err).ToNot(HaveOccurred())
			}
		}
		for _, m := range machines {
			if m.DeletionTimestamp.IsZero() {
				g.Expect(env.Delete(ctx, m)).To(Succeed())
			}
			if len(m.Finalizers) > 1 {
				g.Expect(env.Get(ctx, util.ObjectKey(m), m)).To(Succeed())
				machinePatchHelper, err := patch.NewHelper(m, env.Client)
				g.Expect(err).ToNot(HaveOccurred())
				m.Finalizers = nil
				g.Expect(machinePatchHelper.Patch(ctx, m)).To(Succeed())
			}
		}
		for _, im := range infraMachines {
			if err := env.Delete(ctx, im); !apierrors.IsNotFound(err) {
				g.Expect(err).ToNot(HaveOccurred())
			}
		}
	}

	return nodes, machines, cleanup
}

func newMachineHealthCheckWithLabels(name, namespace, cluster string, labels map[string]string) *clusterv1.MachineHealthCheck {
	l := make(map[string]string, len(labels))
	for k, v := range labels {
		l[k] = v
	}
	l[clusterv1.ClusterNameLabel] = cluster

	mhc := newMachineHealthCheck(namespace, cluster)
	mhc.SetName(name)
	mhc.Labels = l
	mhc.Spec.Selector.MatchLabels = l

	return mhc
}

func newMachineHealthCheck(namespace, clusterName string) *clusterv1.MachineHealthCheck {
	maxUnhealthy := intstr.FromString("100%")
	return &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-mhc-",
			Namespace:    namespace,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			ClusterName: clusterName,
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"selector": string(uuid.NewUUID()),
				},
			},
			Checks: clusterv1.MachineHealthCheckChecks{
				NodeStartupTimeoutSeconds: ptr.To(int32(1)),
				UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
					{
						Type:           corev1.NodeReady,
						Status:         corev1.ConditionUnknown,
						TimeoutSeconds: ptr.To(int32(5 * 60)),
					},
				},
			},
			Remediation: clusterv1.MachineHealthCheckRemediation{
				TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
					UnhealthyLessThanOrEqualTo: &maxUnhealthy,
				},
			},
		},
	}
}

func TestPatchTargets(t *testing.T) {
	g := NewWithT(t)

	namespace := metav1.NamespaceDefault
	clusterName := testClusterName
	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
	}
	labels := map[string]string{"cluster": "foo", "nodepool": "bar"}

	mhc := newMachineHealthCheckWithLabels("mhc", namespace, clusterName, labels)
	machine1 := newTestMachine("machine1", namespace, clusterName, "nodeName", labels)
	machine1.ResourceVersion = "999"

	v1beta1conditions.MarkTrue(machine1, clusterv1.MachineHealthCheckSucceededV1Beta1Condition)
	conditions.Set(machine1, metav1.Condition{
		Type:   clusterv1.MachineHealthCheckSucceededCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineHealthCheckSucceededReason,
	})

	machine2 := machine1.DeepCopy()
	machine2.Name = "machine2"

	cl := fake.NewClientBuilder().WithObjects(
		machine1,
		machine2,
		mhc,
	).WithStatusSubresource(&clusterv1.MachineHealthCheck{}, &clusterv1.Machine{}).Build()
	r := &Reconciler{
		Client:       cl,
		recorder:     record.NewFakeRecorder(32),
		ClusterCache: clustercache.NewFakeClusterCache(cl, client.ObjectKey{Name: clusterName, Namespace: namespace}, "machinehealthcheck-watchClusterNodes"),
	}

	// To make the patch fail, create patchHelper with a different client.
	fakeMachine := machine1.DeepCopy()
	fakeMachine.Name = "fake"
	patchHelper, err := patch.NewHelper(fakeMachine, fake.NewClientBuilder().WithObjects(fakeMachine).Build())
	g.Expect(err).ToNot(HaveOccurred())
	// healthCheckTarget with fake patchHelper, patch should fail on this target.
	target1 := healthCheckTarget{
		MHC:         mhc,
		Machine:     machine1,
		patchHelper: patchHelper,
		Node:        &corev1.Node{},
	}

	// healthCheckTarget with correct patchHelper.
	patchHelper2, err := patch.NewHelper(machine2, cl)
	g.Expect(err).ToNot(HaveOccurred())
	target3 := healthCheckTarget{
		MHC:         mhc,
		Machine:     machine2,
		patchHelper: patchHelper2,
		Node:        &corev1.Node{},
	}

	// Target with wrong patch helper will fail but the other one will be patched.
	g.Expect(r.patchUnhealthyTargets(context.TODO(), logr.New(log.NullLogSink{}), []healthCheckTarget{target1, target3}, defaultCluster, mhc)).ToNot(BeEmpty())
	g.Expect(cl.Get(ctx, client.ObjectKey{Name: machine2.Name, Namespace: machine2.Namespace}, machine2)).ToNot(HaveOccurred())
	g.Expect(v1beta1conditions.Get(machine2, clusterv1.MachineOwnerRemediatedV1Beta1Condition).Status).To(Equal(corev1.ConditionFalse))
	g.Expect(conditions.Get(machine2, clusterv1.MachineOwnerRemediatedCondition).Status).To(Equal(metav1.ConditionFalse))

	// Target with wrong patch helper will fail but the other one will be patched.
	g.Expect(r.patchHealthyTargets(context.TODO(), logr.New(log.NullLogSink{}), []healthCheckTarget{target1, target3}, mhc)).ToNot(BeEmpty())
}
