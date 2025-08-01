/*
Copyright 2021 The Kubernetes Authors.

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
	"maps"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/internal/topology/selectors"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestGetCurrentState(t *testing.T) {
	testGetCurrentState(t, "v1beta1")
	testGetCurrentState(t, "v1beta2")
}

func testGetCurrentState(t *testing.T, controlPlaneContractVersion string) {
	t.Helper()

	crds := []client.Object{
		builder.GenericInfrastructureClusterCRD,
		builder.GenericControlPlaneTemplateCRD,
		builder.GenericInfrastructureClusterTemplateCRD,
		builder.GenericBootstrapConfigCRD,
		builder.GenericBootstrapConfigTemplateCRD,
		builder.GenericInfrastructureMachineTemplateCRD,
		builder.GenericInfrastructureMachineCRD,
		builder.GenericInfrastructureMachinePoolTemplateCRD,
		builder.GenericInfrastructureMachinePoolCRD,
	}
	if controlPlaneContractVersion == "v1beta1" {
		crd := builder.GenericControlPlaneCRD.DeepCopy()
		crd.Labels = map[string]string{
			// Set label to signal that ControlPlane implements v1beta1 contract.
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, "v1beta1"): clusterv1.GroupVersionControlPlane.Version,
		}
		crds = append(crds, crd)
	} else {
		crd := builder.GenericControlPlaneCRD.DeepCopy()
		crd.Labels = map[string]string{
			// Set label to signal that ControlPlane implements v1beta1 contract.
			// Note: This is identical to how GenericControlPlaneCRD is defined, but setting this here for clarity
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, "v1beta2"): clusterv1.GroupVersionControlPlane.Version,
		}
		crds = append(crds, crd)
	}

	// The following is a block creating a number of objects for use in the test cases.

	// InfrastructureCluster objects.
	infraCluster := builder.InfrastructureCluster(metav1.NamespaceDefault, "infraOne").
		Build()
	infraCluster.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	infraClusterNotTopologyOwned := builder.InfrastructureCluster(metav1.NamespaceDefault, "infraOne").
		Build()
	infraClusterTemplate := builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infraTemplateOne").
		Build()

	// ControlPlane and ControlPlaneInfrastructureMachineTemplate objects.
	controlPlaneInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfraTemplate").
		Build()
	controlPlaneInfrastructureMachineTemplate.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	controlPlaneInfrastructureMachineTemplateNotTopologyOwned := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfraTemplate").
		Build()
	controlPlaneTemplateWithInfrastructureMachine := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cpTemplateWithInfra1").
		Build()
	controlPlaneTemplateWithInfrastructureMachineNotTopologyOwned := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cpTemplateWithInfra1").
		Build()
	controlPlane := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		Build()
	controlPlane.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	controlPlaneWithInfra := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate, controlPlaneContractVersion).
		Build()
	controlPlaneWithInfra.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	controlPlaneWithInfraNotTopologyOwned := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplateNotTopologyOwned, controlPlaneContractVersion).
		Build()
	controlPlaneNotTopologyOwned := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		Build()

	// ClusterClass  objects.
	clusterClassWithControlPlaneInfra := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
		WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()
	clusterClassWithControlPlaneInfraNotTopologyOwned := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachineNotTopologyOwned).
		WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplateNotTopologyOwned).
		Build()
	clusterClassWithNoControlPlaneInfra := builder.ClusterClass(metav1.NamespaceDefault, "class2").
		Build()

	// MachineDeployment and related objects.
	emptyMachineDeployments := make(map[string]*scope.MachineDeploymentState)

	machineDeploymentInfrastructure := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").
		Build()
	machineDeploymentInfrastructure.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	machineDeploymentBootstrap := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").
		Build()
	machineDeploymentBootstrap.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})

	machineDeployment := builder.MachineDeployment(metav1.NamespaceDefault, "md1").
		WithLabels(map[string]string{
			clusterv1.ClusterNameLabel:                          "cluster1",
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: "md1",
		}).
		WithBootstrapTemplate(machineDeploymentBootstrap).
		WithInfrastructureTemplate(machineDeploymentInfrastructure).
		Build()
	machineDeploymentWithDeletionTimestamp := machineDeployment.DeepCopy()
	machineDeploymentWithDeletionTimestamp.Finalizers = []string{clusterv1.MachineDeploymentFinalizer} // required by fake client
	machineDeploymentWithDeletionTimestamp.DeletionTimestamp = ptr.To(metav1.Now())
	machineDeployment2 := builder.MachineDeployment(metav1.NamespaceDefault, "md2").
		WithLabels(map[string]string{
			clusterv1.ClusterNameLabel:                          "cluster1",
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: "md2",
		}).
		WithBootstrapTemplate(machineDeploymentBootstrap).
		WithInfrastructureTemplate(machineDeploymentInfrastructure).
		Build()

	// MachineHealthChecks for the MachineDeployment and the ControlPlane.
	machineHealthCheckForMachineDeployment := builder.MachineHealthCheck(machineDeployment.Namespace, machineDeployment.Name).
		WithSelector(*selectors.ForMachineDeploymentMHC(machineDeployment)).
		WithUnhealthyNodeConditions([]clusterv1.UnhealthyNodeCondition{
			{
				Type:           corev1.NodeReady,
				Status:         corev1.ConditionUnknown,
				TimeoutSeconds: ptr.To(int32(5 * 60)),
			},
			{
				Type:           corev1.NodeReady,
				Status:         corev1.ConditionFalse,
				TimeoutSeconds: ptr.To(int32(5 * 60)),
			},
		}).
		WithClusterName("cluster1").
		Build()

	machineHealthCheckForControlPlane := builder.MachineHealthCheck(controlPlane.GetNamespace(), controlPlane.GetName()).
		WithSelector(*selectors.ForControlPlaneMHC()).
		WithUnhealthyNodeConditions([]clusterv1.UnhealthyNodeCondition{
			{
				Type:           corev1.NodeReady,
				Status:         corev1.ConditionUnknown,
				TimeoutSeconds: ptr.To(int32(5 * 60)),
			},
			{
				Type:           corev1.NodeReady,
				Status:         corev1.ConditionFalse,
				TimeoutSeconds: ptr.To(int32(5 * 60)),
			},
		}).
		WithClusterName("cluster1").
		Build()

	// MachinePool and related objects.
	emptyMachinePools := make(map[string]*scope.MachinePoolState)

	machinePoolInfrastructureTemplate := builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra2").
		Build()
	machinePoolInfrastructure := builder.InfrastructureMachinePool(metav1.NamespaceDefault, "infra2").
		Build()
	machinePoolInfrastructure.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	machinePoolBootstrapTemplate := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap2").
		Build()
	machinePoolBootstrap := builder.BootstrapConfig(metav1.NamespaceDefault, "bootstrap2").
		Build()
	machinePoolBootstrap.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})

	machinePool := builder.MachinePool(metav1.NamespaceDefault, "mp1").
		WithLabels(map[string]string{
			clusterv1.ClusterNameLabel:                    "cluster1",
			clusterv1.ClusterTopologyOwnedLabel:           "",
			clusterv1.ClusterTopologyMachinePoolNameLabel: "mp1",
		}).
		WithBootstrap(machinePoolBootstrap).
		WithInfrastructure(machinePoolInfrastructure).
		Build()
	machinePool2 := builder.MachinePool(metav1.NamespaceDefault, "mp2").
		WithLabels(map[string]string{
			clusterv1.ClusterNameLabel:                    "cluster1",
			clusterv1.ClusterTopologyOwnedLabel:           "",
			clusterv1.ClusterTopologyMachinePoolNameLabel: "mp2",
		}).
		WithBootstrap(machinePoolBootstrap).
		WithInfrastructure(machinePoolInfrastructure).
		Build()

	tests := []struct {
		name      string
		cluster   *clusterv1.Cluster
		blueprint *scope.ClusterBlueprint
		objects   []client.Object
		want      *scope.ClusterState
		wantErr   bool
	}{
		{
			name:      "Should read a Cluster when being processed by the topology controller for the first time (without references)",
			cluster:   builder.Cluster(metav1.NamespaceDefault, "cluster1").Build(),
			blueprint: &scope.ClusterBlueprint{},
			// Expecting valid return with no ControlPlane or Infrastructure state defined and empty MachineDeployment state list
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					// No InfrastructureCluster or ControlPlane references!
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: nil,
				MachineDeployments:    emptyMachineDeployments,
				MachinePools:          emptyMachinePools,
			},
		},
		{
			name: "Fails if the Cluster references an InfrastructureCluster that does not exist",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				InfrastructureClusterTemplate: infraClusterTemplate,
			},
			objects: []client.Object{
				// InfrastructureCluster is missing!
			},
			wantErr: true, // this test fails as partial reconcile is undefined.
		},
		{
			name: "Fails if the Cluster references an InfrastructureCluster that is not topology owned",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraClusterNotTopologyOwned).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				InfrastructureClusterTemplate: infraClusterTemplate,
			},
			objects: []client.Object{
				infraClusterNotTopologyOwned,
			},
			wantErr: true, // this test fails as partial reconcile is undefined.
		},
		{
			name: "Fails if the Cluster references an Control Plane that is not topology owned",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlaneNotTopologyOwned).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfra,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplateWithInfrastructureMachine,
				},
			},
			objects: []client.Object{
				controlPlaneNotTopologyOwned,
			},
			wantErr: true, // this test fails as partial reconcile is undefined.
		},
		{
			name: "Should read  a partial Cluster (with InfrastructureCluster only)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				// No ControlPlane reference!
				Build(),
			blueprint: &scope.ClusterBlueprint{
				InfrastructureClusterTemplate: infraClusterTemplate,
			},
			objects: []client.Object{
				infraCluster,
			},
			// Expecting valid return with no ControlPlane, MachineDeployment, or MachinePool state defined but with a valid Infrastructure state.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: infraCluster,
				MachineDeployments:    emptyMachineDeployments,
				MachinePools:          emptyMachinePools,
			},
		},
		{
			name: "Should read a partial Cluster (with InfrastructureCluster, ControlPlane, but without workers)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlane).
				WithInfrastructureCluster(infraCluster).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithNoControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplateWithInfrastructureMachine,
				},
			},
			objects: []client.Object{
				controlPlane,
				infraCluster,
				clusterClassWithNoControlPlaneInfra,
				// Workers are missing!
			},
			// Expecting valid return with ControlPlane, no ControlPlane Infrastructure state, InfrastructureCluster state, and no defined MachineDeployment or MachinePool state.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithControlPlane(controlPlane).
					WithInfrastructureCluster(infraCluster).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlane, InfrastructureMachineTemplate: nil},
				InfrastructureCluster: infraCluster,
				MachineDeployments:    emptyMachineDeployments,
				MachinePools:          emptyMachinePools,
			},
		},
		{
			name: "Fails if the ClusterClass requires InfrastructureMachine for the ControlPlane, but the ControlPlane object does not have a reference for it",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlane).
				WithInfrastructureCluster(infraCluster).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplateWithInfrastructureMachine,
				},
			},
			objects: []client.Object{
				controlPlane,
				infraCluster,
				clusterClassWithControlPlaneInfra,
			},
			// Expecting error from ControlPlane having no valid ControlPlane Infrastructure with ClusterClass requiring ControlPlane Infrastructure.
			wantErr: true,
		},
		{
			name: "Should read a partial Cluster (with ControlPlane and ControlPlane InfrastructureMachineTemplate, but without InfrastructureCluster and workers)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				// No InfrastructureCluster!
				WithControlPlane(controlPlaneWithInfra).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfra,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				controlPlaneWithInfra,
				controlPlaneInfrastructureMachineTemplate,
				// Workers are missing!
			},
			// Expecting valid return with valid ControlPlane state, but no ControlPlane Infrastructure, InfrastructureCluster, MachineDeployment, or MachinePool state defined.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithControlPlane(controlPlaneWithInfra).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: nil,
				MachineDeployments:    emptyMachineDeployments,
				MachinePools:          emptyMachinePools,
			},
		},
		{
			name: "Should read a partial Cluster (with InfrastructureCluster ControlPlane and ControlPlane InfrastructureMachineTemplate, but without workers)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				WithControlPlane(controlPlaneWithInfra).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				// Workers are missing!
			},
			// Expecting valid return with valid ControlPlane state, ControlPlane Infrastructure state and InfrastructureCluster state, but no defined MachineDeployment or MachinePool state.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: infraCluster,
				MachineDeployments:    emptyMachineDeployments,
				MachinePools:          emptyMachinePools,
			},
		},
		{
			name: "Should read a Cluster (with InfrastructureCluster, ControlPlane and ControlPlane InfrastructureMachineTemplate, workers)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					WithMachinePool(clusterv1.MachinePoolTopology{
						Class: "mpClass",
						Name:  "mp1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
				MachinePools: map[string]*scope.MachinePoolBlueprint{
					"mpClass": {
						BootstrapTemplate:                 machinePoolBootstrap,
						InfrastructureMachinePoolTemplate: machinePoolInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeployment,
				machinePoolInfrastructure,
				machinePoolBootstrap,
				machinePool,
			},
			// Expecting valid return with valid ControlPlane, ControlPlane Infrastructure, InfrastructureCluster, MachineDeployment and MachinePool state.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithMachineDeployment(clusterv1.MachineDeploymentTopology{
							Class: "mdClass",
							Name:  "md1",
						}).
						WithMachinePool(clusterv1.MachinePoolTopology{
							Class: "mpClass",
							Name:  "mp1",
						}).
						Build()).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: nil,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {Object: machineDeployment, BootstrapTemplate: machineDeploymentBootstrap, InfrastructureMachineTemplate: machineDeploymentInfrastructure},
				},
				MachinePools: map[string]*scope.MachinePoolState{
					"mp1": {Object: machinePool, BootstrapObject: machinePoolBootstrap, InfrastructureMachinePoolObject: machinePoolInfrastructure},
				},
			},
		},
		{
			name: "Fails if the ControlPlane references an InfrastructureMachineTemplate that is not topology owned",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				// No InfrastructureCluster!
				WithControlPlane(controlPlaneWithInfraNotTopologyOwned).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfraNotTopologyOwned,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				controlPlaneWithInfraNotTopologyOwned,
				controlPlaneInfrastructureMachineTemplateNotTopologyOwned,
			},
			wantErr: true,
		},
		{
			name: "Fails if the ControlPlane references an InfrastructureMachineTemplate that does not exist",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlane).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfra,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				controlPlane,
				// InfrastructureMachineTemplate is missing!
			},
			// Expecting error as ClusterClass references ControlPlane Infrastructure, but ControlPlane Infrastructure is missing in the cluster.
			wantErr: true,
		},
		{
			name: "Should ignore unmanaged MachineDeployments/MachinePools and MachineDeployments/MachinePools belonging to other clusters",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				Build(),
			blueprint: &scope.ClusterBlueprint{ClusterClass: clusterClassWithControlPlaneInfra},
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				builder.MachineDeployment(metav1.NamespaceDefault, "no-managed-label").
					WithLabels(map[string]string{
						clusterv1.ClusterNameLabel: "cluster1",
						// topology.cluster.x-k8s.io/owned label is missing (unmanaged)!
					}).
					WithBootstrapTemplate(machineDeploymentBootstrap).
					WithInfrastructureTemplate(machineDeploymentInfrastructure).
					Build(),
				builder.MachineDeployment(metav1.NamespaceDefault, "wrong-cluster-label").
					WithLabels(map[string]string{
						clusterv1.ClusterNameLabel:                          "another-cluster",
						clusterv1.ClusterTopologyOwnedLabel:                 "",
						clusterv1.ClusterTopologyMachineDeploymentNameLabel: "md1",
					}).
					WithBootstrapTemplate(machineDeploymentBootstrap).
					WithInfrastructureTemplate(machineDeploymentInfrastructure).
					Build(),
				builder.MachinePool(metav1.NamespaceDefault, "no-managed-label").
					WithLabels(map[string]string{
						clusterv1.ClusterNameLabel: "cluster1",
						// topology.cluster.x-k8s.io/owned label is missing (unmanaged)!
					}).
					WithBootstrap(machinePoolBootstrap).
					WithInfrastructure(machinePoolInfrastructure).
					Build(),
				builder.MachinePool(metav1.NamespaceDefault, "wrong-cluster-label").
					WithLabels(map[string]string{
						clusterv1.ClusterNameLabel:                    "another-cluster",
						clusterv1.ClusterTopologyOwnedLabel:           "",
						clusterv1.ClusterTopologyMachinePoolNameLabel: "mp1",
					}).
					WithBootstrap(machinePoolBootstrap).
					WithInfrastructure(machinePoolInfrastructure).
					Build(),
			},
			// Expect valid return with empty MachineDeployments and MachinePools properly filtered by label.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: nil,
				MachineDeployments:    emptyMachineDeployments,
				MachinePools:          emptyMachinePools,
			},
		},
		{
			name: "Fails if there are MachineDeployments without the topology.cluster.x-k8s.io/deployment-name",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				Build(),
			blueprint: &scope.ClusterBlueprint{ClusterClass: clusterClassWithControlPlaneInfra},
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				builder.MachineDeployment(metav1.NamespaceDefault, "missing-topology-md-labelName").
					WithLabels(map[string]string{
						clusterv1.ClusterNameLabel:          "cluster1",
						clusterv1.ClusterTopologyOwnedLabel: "",
						// topology.cluster.x-k8s.io/deployment-name label is missing!
					}).
					WithBootstrapTemplate(machineDeploymentBootstrap).
					WithInfrastructureTemplate(machineDeploymentInfrastructure).
					Build(),
			},
			// Expect error to be thrown as no managed MachineDeployment is reconcilable unless it has a ClusterTopologyMachineDeploymentNameLabel.
			wantErr: true,
		},
		{
			name: "Fails if there are MachineDeployments with the same topology.cluster.x-k8s.io/deployment-name",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfra,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeployment,
				builder.MachineDeployment(metav1.NamespaceDefault, "duplicate-labels").
					WithLabels(machineDeployment.Labels). // Another machine deployment with the same labels.
					WithBootstrapTemplate(machineDeploymentBootstrap).
					WithInfrastructureTemplate(machineDeploymentInfrastructure).
					Build(),
			},
			// Expect error as two MachineDeployments with the same ClusterTopologyOwnedLabel should not exist for one cluster
			wantErr: true,
		},
		{
			name: "Fails if there are MachinePools without the topology.cluster.x-k8s.io/pool-name",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				Build(),
			blueprint: &scope.ClusterBlueprint{ClusterClass: clusterClassWithControlPlaneInfra},
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				builder.MachinePool(metav1.NamespaceDefault, "missing-topology-mp-labelName").
					WithLabels(map[string]string{
						clusterv1.ClusterNameLabel:          "cluster1",
						clusterv1.ClusterTopologyOwnedLabel: "",
						// topology.cluster.x-k8s.io/pool-name label is missing!
					}).
					WithBootstrap(machinePoolBootstrap).
					WithInfrastructure(machinePoolInfrastructure).
					Build(),
			},
			// Expect error to be thrown as no managed MachinePool is reconcilable unless it has a ClusterTopologyMachinePoolNameLabel.
			wantErr: true,
		},
		{
			name: "Fails if there are MachinePools with the same topology.cluster.x-k8s.io/pool-name",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachinePool(clusterv1.MachinePoolTopology{
						Class: "mpClass",
						Name:  "mp1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfra,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				machinePoolInfrastructure,
				machinePoolBootstrap,
				machinePool,
				builder.MachinePool(metav1.NamespaceDefault, "duplicate-labels").
					WithLabels(machinePool.Labels). // Another machine pool with the same labels.
					WithBootstrap(machinePoolBootstrap).
					WithInfrastructure(machinePoolInfrastructure).
					Build(),
			},
			// Expect error as two MachinePools with the same ClusterTopologyOwnedLabel should not exist for one cluster
			wantErr: true,
		},
		{
			name: "Should read a full Cluster (With InfrastructureCluster, ControlPlane and ControlPlane Infrastructure, MachineDeployments, MachinePools)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				WithControlPlane(controlPlaneWithInfra).
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					WithMachinePool(clusterv1.MachinePoolTopology{
						Class: "mpClass",
						Name:  "mp1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
				MachinePools: map[string]*scope.MachinePoolBlueprint{
					"mpClass": {
						BootstrapTemplate:                 machinePoolBootstrapTemplate,
						InfrastructureMachinePoolTemplate: machinePoolInfrastructureTemplate,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeployment,
				machinePoolInfrastructure,
				machinePoolBootstrap,
				machinePool,
			},
			// Expect valid return of full ClusterState with MachineDeployments and MachinePools properly filtered by label.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					WithTopology(builder.ClusterTopology().
						WithMachineDeployment(clusterv1.MachineDeploymentTopology{
							Class: "mdClass",
							Name:  "md1",
						}).
						WithMachinePool(clusterv1.MachinePoolTopology{
							Class: "mpClass",
							Name:  "mp1",
						}).
						Build()).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: infraCluster,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {
						Object:                        machineDeployment,
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
				MachinePools: map[string]*scope.MachinePoolState{
					"mp1": {
						Object:                          machinePool,
						BootstrapObject:                 machinePoolBootstrap,
						InfrastructureMachinePoolObject: machinePoolInfrastructure,
					},
				},
			},
		},
		{
			name: "Should read a full Cluster, even if a MachineDeployment & MachinePool topology has been deleted and the MachineDeployment & MachinePool still exists",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				WithControlPlane(controlPlaneWithInfra).
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					WithMachinePool(clusterv1.MachinePoolTopology{
						Class: "mpClass",
						Name:  "mp1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
				MachinePools: map[string]*scope.MachinePoolBlueprint{
					"mpClass": {
						BootstrapTemplate:                 machinePoolBootstrapTemplate,
						InfrastructureMachinePoolTemplate: machinePoolInfrastructureTemplate,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeployment,
				machineDeployment2,
				machinePoolInfrastructure,
				machinePoolBootstrap,
				machinePool,
				machinePool2,
			},
			// Expect valid return of full ClusterState with MachineDeployments and MachinePools properly filtered by label.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					WithTopology(builder.ClusterTopology().
						WithMachineDeployment(clusterv1.MachineDeploymentTopology{
							Class: "mdClass",
							Name:  "md1",
						}).
						WithMachinePool(clusterv1.MachinePoolTopology{
							Class: "mpClass",
							Name:  "mp1",
						}).
						Build()).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: infraCluster,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {
						Object:                        machineDeployment,
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
					"md2": {
						Object:                        machineDeployment2,
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
				MachinePools: map[string]*scope.MachinePoolState{
					"mp1": {
						Object:                          machinePool,
						BootstrapObject:                 machinePoolBootstrap,
						InfrastructureMachinePoolObject: machinePoolInfrastructure,
					},
					"mp2": {
						Object:                          machinePool2,
						BootstrapObject:                 machinePoolBootstrap,
						InfrastructureMachinePoolObject: machinePoolInfrastructure,
					},
				},
			},
		},
		{
			name: "Fails if a Cluster has a MachineDeployment without the Bootstrap Template ref",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				builder.MachineDeployment(metav1.NamespaceDefault, "no-bootstrap").
					WithLabels(machineDeployment.Labels).
					// No BootstrapConfigTemplate reference!
					WithInfrastructureTemplate(machineDeploymentInfrastructure).
					Build(),
			},
			// Expect error as Bootstrap Template not defined for MachineDeployments relevant to the Cluster.
			wantErr: true,
		},
		{
			name: "Fails if a Cluster has a MachinePool without the Bootstrap Template ref",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachinePool(clusterv1.MachinePoolTopology{
						Class: "mpClass",
						Name:  "mp1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachinePools: map[string]*scope.MachinePoolBlueprint{
					"mpClass": {
						BootstrapTemplate:                 machinePoolBootstrapTemplate,
						InfrastructureMachinePoolTemplate: machinePoolInfrastructureTemplate,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machinePoolInfrastructure,
				builder.MachinePool(metav1.NamespaceDefault, "no-bootstrap").
					WithLabels(machinePool.Labels).
					// No BootstrapConfig reference!
					WithInfrastructure(machinePoolInfrastructure).
					Build(),
			},
			// Expect error as BootstrapConfig not defined for MachinePools relevant to the Cluster.
			wantErr: true,
		},
		{
			name: "Fails if a Cluster has a MachineDeployments without the InfrastructureMachineTemplate ref",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentBootstrap,
				builder.MachineDeployment(metav1.NamespaceDefault, "no-infra").
					WithLabels(machineDeployment.Labels).
					WithBootstrapTemplate(machineDeploymentBootstrap).
					// No InfrastructureMachineTemplate reference!
					Build(),
			},
			// Expect error as Infrastructure Template not defined for MachineDeployment relevant to the Cluster.
			wantErr: true,
		},
		{
			name: "Fails if a Cluster has a MachinePools without the InfrastructureMachinePool ref",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachinePool(clusterv1.MachinePoolTopology{
						Class: "mpClass",
						Name:  "mp1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachinePools: map[string]*scope.MachinePoolBlueprint{
					"mpClass": {
						BootstrapTemplate:                 machinePoolBootstrapTemplate,
						InfrastructureMachinePoolTemplate: machinePoolInfrastructureTemplate,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machinePoolBootstrap,
				builder.MachinePool(metav1.NamespaceDefault, "no-infra").
					WithLabels(machinePool.Labels).
					WithBootstrap(machinePoolBootstrap).
					// No InfrastructureMachinePool reference!
					Build(),
			},
			// Expect error as InfrastructureMachinePool not defined for MachinePool relevant to the Cluster.
			wantErr: true,
		},
		{
			name: "Pass reading a full Cluster with MachineHealthChecks for ControlPlane and MachineDeployment)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				WithControlPlane(controlPlaneWithInfra).
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeployment,
				machineHealthCheckForMachineDeployment,
				machineHealthCheckForControlPlane,
			},
			// Expect valid return of full ClusterState with MachineHealthChecks for both ControlPlane and MachineDeployment.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					WithTopology(builder.ClusterTopology().
						WithMachineDeployment(clusterv1.MachineDeploymentTopology{
							Class: "mdClass",
							Name:  "md1",
						}).
						Build()).
					Build(),
				ControlPlane: &scope.ControlPlaneState{
					Object:                        controlPlaneWithInfra,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
					MachineHealthCheck:            machineHealthCheckForControlPlane,
				},
				InfrastructureCluster: infraCluster,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {
						Object:                        machineDeployment,
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
						MachineHealthCheck:            machineHealthCheckForMachineDeployment,
					},
				},
				MachinePools: emptyMachinePools,
			},
		},
		{
			name: "Pass reading a full Cluster with a deleting MachineDeployment",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				WithControlPlane(controlPlaneWithInfra).
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentWithDeletionTimestamp,
				machineHealthCheckForMachineDeployment,
				machineHealthCheckForControlPlane,
			},
			// Expect valid return of full ClusterState with MachineDeployment without corresponding templates.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					WithTopology(builder.ClusterTopology().
						WithMachineDeployment(clusterv1.MachineDeploymentTopology{
							Class: "mdClass",
							Name:  "md1",
						}).
						Build()).
					Build(),
				ControlPlane: &scope.ControlPlaneState{
					Object:                        controlPlaneWithInfra,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
					MachineHealthCheck:            machineHealthCheckForControlPlane,
				},
				InfrastructureCluster: infraCluster,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {
						Object:                        machineDeploymentWithDeletionTimestamp,
						BootstrapTemplate:             nil,
						InfrastructureMachineTemplate: nil,
						MachineHealthCheck:            machineHealthCheckForMachineDeployment,
					},
				},
				MachinePools: emptyMachinePools,
			},
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s (control plane contract %s)", tt.name, controlPlaneContractVersion), func(t *testing.T) {
			g := NewWithT(t)

			// Sets up a scope with a Blueprint.
			s := scope.New(tt.cluster)
			s.Blueprint = tt.blueprint

			// Sets up the fakeClient for the test case.
			objs := []client.Object{}
			objs = append(objs, crds...)
			objs = append(objs, tt.objects...)
			if tt.cluster != nil {
				objs = append(objs, tt.cluster)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(objs...).
				Build()

			// Calls getCurrentState.
			r := &Reconciler{
				Client:    fakeClient,
				APIReader: fakeClient,
			}
			got, err := r.getCurrentState(ctx, s)

			// Checks the return error.
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}

			// Don't compare the deletionTimestamps as there are some minor differences in how they are stored pre/post fake client.
			for _, md := range append(slices.Collect(maps.Values(got.MachineDeployments)), slices.Collect(maps.Values(tt.want.MachineDeployments))...) {
				md.Object.DeletionTimestamp = nil
			}

			// Use EqualObject where the compared object is passed through the fakeClient. Elsewhere the Equal method is
			// good enough to establish equality.
			g.Expect(got.Cluster).To(EqualObject(tt.want.Cluster, IgnoreAutogeneratedMetadata))
			g.Expect(got.InfrastructureCluster).To(EqualObject(tt.want.InfrastructureCluster))
			g.Expect(got.ControlPlane).To(BeComparableTo(tt.want.ControlPlane), cmp.Diff(got.ControlPlane, tt.want.ControlPlane))
			g.Expect(got.MachineDeployments).To(BeComparableTo(tt.want.MachineDeployments), cmp.Diff(got.MachineDeployments, tt.want.MachineDeployments))
			g.Expect(got.MachinePools).To(BeComparableTo(tt.want.MachinePools), cmp.Diff(got.MachinePools, tt.want.MachinePools))
		})
	}
}

func TestAlignRefAPIVersion(t *testing.T) {
	tests := []struct {
		name                     string
		templateFromClusterClass *unstructured.Unstructured
		currentRef               clusterv1.ContractVersionedObjectReference
		isCurrentTemplate        bool
		objs                     []client.Object
		want                     *corev1.ObjectReference
		wantErr                  bool
	}{
		{
			name: "Use apiVersion from ClusterClass: group and kind is the same (+/- Template suffix)",
			templateFromClusterClass: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"kind":       "DockerClusterTemplate",
			}},
			currentRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "DockerCluster",
				Name:     "my-cluster-abc",
			},
			isCurrentTemplate: false,
			want: &corev1.ObjectReference{
				// Group & kind is the same (+/- Template suffix) => apiVersion is taken from ClusterClass.
				APIVersion: clusterv1.GroupVersionInfrastructure.String(),
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
		{
			name: "Use apiVersion from ClusterClass: group and kind is the same",
			templateFromClusterClass: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
				"kind":       builder.GenericBootstrapConfigKind,
			}},
			currentRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionBootstrap.Group,
				Kind:     builder.GenericBootstrapConfigKind,
				Name:     "my-cluster-abc",
			},
			isCurrentTemplate: true,
			want: &corev1.ObjectReference{
				// Group & kind is the same => apiVersion is taken from ClusterClass.
				APIVersion: clusterv1.GroupVersionBootstrap.String(),
				Kind:       builder.GenericBootstrapConfigKind,
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
		{
			name: "Use apiVersion from CRD: kind is different",
			templateFromClusterClass: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
				"kind":       "DifferentConfigTemplate",
			}},
			currentRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionBootstrap.Group,
				Kind:     builder.GenericBootstrapConfigKind,
				Name:     "my-cluster-abc",
			},
			isCurrentTemplate: true,
			objs:              []client.Object{builder.GenericBootstrapConfigCRD},
			want: &corev1.ObjectReference{
				// kind is different => apiVersion is taken from CRD.
				APIVersion: clusterv1.GroupVersionBootstrap.String(),
				Kind:       builder.GenericBootstrapConfigKind,
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
		{
			name: "Use apiVersion from CRD: group is different",
			templateFromClusterClass: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "different.bootstrap.cluster.x-k8s.io/v1beta2",
				"kind":       builder.GenericBootstrapConfigKind,
			}},
			currentRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionBootstrap.Group,
				Kind:     builder.GenericBootstrapConfigKind,
				Name:     "my-cluster-abc",
			},
			isCurrentTemplate: true,
			objs:              []client.Object{builder.GenericBootstrapConfigCRD},
			want: &corev1.ObjectReference{
				// group is different => apiVersion is taken from CRD.
				APIVersion: clusterv1.GroupVersionBootstrap.String(),
				Kind:       builder.GenericBootstrapConfigKind,
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			scheme := runtime.NewScheme()
			g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objs...).Build()

			got, err := alignRefAPIVersion(t.Context(), c, tt.templateFromClusterClass, tt.currentRef, metav1.NamespaceDefault, tt.isCurrentTemplate)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(BeComparableTo(tt.want))
		})
	}
}
