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

package webhooks

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/version"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/topology/variables"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

var (
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = apiextensionsv1.AddToScheme(fakeScheme)
	_ = clusterv1.AddToScheme(fakeScheme)
}

func TestClusterClassValidationFeatureGated(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.

	tests := []struct {
		name      string
		in        *clusterv1.ClusterClass
		old       *clusterv1.ClusterClass
		expectErr bool
	}{
		{
			name: "creation should fail if feature flag is disabled, no matter the ClusterClass is valid(or not)",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate("", "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate("", "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate("", "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate("", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("", "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate("", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("", "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update should fail if feature flag is disabled, no matter the ClusterClass is valid(or not)",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate("", "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate("", "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate("", "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate("", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("", "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate("", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("", "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate("", "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate("", "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate("", "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate("", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("", "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate("", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("", "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			webhook := &ClusterClass{}
			err := webhook.validate(ctx, tt.old, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestClusterClassValidation(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	ref := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
	}
	refBadTemplate := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "bar",
		Name:       "baz",
	}
	refBadAPIVersion := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group/test.io/v1/foo",
		Kind:       "barTemplate",
		Name:       "baz",
	}
	incompatibleRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "baz",
	}
	compatibleRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/another-foo",
		Kind:       "barTemplate",
		Name:       "another-baz",
	}

	tests := []struct {
		name      string
		in        *clusterv1.ClusterClass
		old       *clusterv1.ClusterClass
		expectErr bool
	}{
		/*
			CREATE Tests
		*/

		{
			name: "create pass",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},

		// empty name in ref tests
		{
			name: "create fail infrastructureCluster has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail controlPlane class has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail control plane class machineInfrastructure has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail machineDeployment Bootstrap has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail machineDeployment Infrastructure has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail machinePool Bootstrap has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail machinePool Infrastructure has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap").Build()).
						Build()).
				Build(),
			expectErr: true,
		},

		// bad template in ref tests
		{
			name: "create fail if bad template in InfrastructureCluster",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(refBadTemplate)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in controlPlane machineInfrastructure",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(refBadTemplate)).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in ControlPlane",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(refBadTemplate)).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in machineDeployment Bootstrap",

			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(refBadTemplate)).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in machineDeployment Infrastructure",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(refBadTemplate)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in machinePool Bootstrap",

			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(refBadTemplate)).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in machinePool Infrastructure",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(refBadTemplate)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},

		// bad apiVersion in ref tests
		{
			name: "create fail with a bad APIVersion for template in InfrastructureCluster",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(refBadAPIVersion)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail with a bad APIVersion for template in controlPlane machineInfrastructure",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(refBadAPIVersion)).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail with a bad APIVersion for template in ControlPlane",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(refBadAPIVersion)).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail with a bad APIVersion for template in machineDeployment Bootstrap",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(refBadAPIVersion)).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail with a bad APIVersion for template in machineDeployment Infrastructure",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(refBadAPIVersion)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail with a bad APIVersion for template in machinePool Bootstrap",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(refBadAPIVersion)).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail with a bad APIVersion for template in machinePool Infrastructure",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(refBadAPIVersion)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},

		// create test
		{
			name: "create fail if duplicated machineDeploymentClasses",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail if duplicated machinePoolClasses",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).Build(),
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create pass if valid machineHealthCheck defined for ControlPlane with MachineInfrastructure set",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneClassHealthCheck{
					Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
						UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
							{
								Type:           corev1.NodeReady,
								Status:         corev1.ConditionUnknown,
								TimeoutSeconds: ptr.To(int32(5 * 60)),
							},
						},
						NodeStartupTimeoutSeconds: ptr.To(int32(60)),
					},
				}).
				Build(),
		},
		{
			name: "create fail if MachineHealthCheck defined for ControlPlane with MachineInfrastructure unset",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				// No ControlPlaneMachineInfrastructure makes this an invalid creation request.
				WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneClassHealthCheck{
					Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
						NodeStartupTimeoutSeconds: ptr.To(int32(60)),
					},
				}).
				Build(),
			expectErr: true,
		},
		{
			name: "create does not fail if ControlPlane MachineHealthCheck does not define UnhealthyNodeConditions",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneClassHealthCheck{
					Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
						NodeStartupTimeoutSeconds: ptr.To(int32(60)),
					},
				}).
				Build(),
			expectErr: false,
		},
		{
			name: "create pass if MachineDeployment MachineHealthCheck is valid",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
								NodeStartupTimeoutSeconds: ptr.To(int32(60)),
							},
						}).
						Build()).
				Build(),
		},
		{
			name: "create fail if MachineDeployment MachineHealthCheck NodeStartUpTimeout is too short",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
								// nodeStartupTimeout is too short here.
								NodeStartupTimeoutSeconds: ptr.To(int32(10)),
							},
						}).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create does not fail if MachineDeployment MachineHealthCheck does not define UnhealthyNodeConditions",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								NodeStartupTimeoutSeconds: ptr.To(int32(60)),
							},
						}).
						Build()).
				Build(),
			expectErr: false,
		},

		// update tests
		{
			name: "update pass in case of no changes",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "update pass if infrastructureCluster changes in a compatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(ref)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(compatibleRef)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "update fails if infrastructureCluster changes in an incompatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(ref)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(incompatibleRef)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update pass if controlPlane changes in a compatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(compatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(compatibleRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "update fails if controlPlane changes in an incompatible way (controlPlane template)",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(incompatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(compatibleRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update fails if controlPlane changes in an incompatible way (controlPlane infrastructureMachineTemplate)",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(compatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(incompatibleRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update pass if a machine deployment and machine pool changes in a compatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(compatibleRef)).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(compatibleRef)).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "update fails a machine deployment changes in an incompatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(incompatibleRef)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update fails a machine pool changes in an incompatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(incompatibleRef)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update pass if a machine deployment or machine pool class gets added",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("BB").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachinePoolClass("BB").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "update fails if a duplicated machine deployment class gets added",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update fails if a duplicated machine pool class gets added",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).Build(),
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should return error for invalid machine deployment labels and annotations",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithLabels(invalidLabels()).
						WithAnnotations(invalidAnnotations()).
						Build()).
				WithControlPlaneMetadata(invalidLabels(), invalidAnnotations()).
				Build(),
			expectErr: true,
		},
		{
			name: "should return error for invalid machine pool labels and annotations",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithLabels(invalidLabels()).
						WithAnnotations(invalidAnnotations()).
						Build()).
				WithControlPlaneMetadata(invalidLabels(), invalidAnnotations()).
				Build(),
			expectErr: true,
		},
		{
			name: "should not return error for valid naming.template",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithInfraClusterNaming(&clusterv1.InfrastructureClassNamingSpec{Template: "{{ .cluster.name }}-infra-{{ .random }}"}).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneNaming(&clusterv1.ControlPlaneClassNamingSpec{Template: "{{ .cluster.name }}-cp-{{ .random }}"}).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithNaming(&clusterv1.MachineDeploymentClassNamingSpec{Template: "{{ .cluster.name }}-md-{{ .machineDeployment.topologyName }}-{{ .random }}"}).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra2").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap2").Build()).
						WithNaming(&clusterv1.MachinePoolClassNamingSpec{Template: "{{ .cluster.name }}-md-{{ .machinePool.topologyName }}-{{ .random }}"}).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "should return error for invalid InfraCluster Infrastructure.Naming.template",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithInfraClusterNaming(&clusterv1.InfrastructureClassNamingSpec{Template: "template-infra-{{ .invalidkey }}"}).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should return error for invalid InfraCluster Infrastructure.Naming.template when the generated name does not conform to RFC 1123",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithInfraClusterNaming(&clusterv1.InfrastructureClassNamingSpec{Template: "template-infra-{{ .cluster.name }}-"}).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should return error for invalid ControlPlane naming.template",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneNaming(&clusterv1.ControlPlaneClassNamingSpec{Template: "template-cp-{{ .invalidkey }}"}).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should return error for ControlPlane naming.template when the generated name does not conform to RFC 1123",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneNaming(&clusterv1.ControlPlaneClassNamingSpec{Template: "template-cp-{{ .cluster.name }}-"}).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should return error for invalid MachineDeployment naming.template",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithNaming(&clusterv1.MachineDeploymentClassNamingSpec{Template: "template-md-{{ .cluster.name"}).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should return error for invalid MachineDeployment naming.template when the generated name does not conform to RFC 1123",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithNaming(&clusterv1.MachineDeploymentClassNamingSpec{Template: "template-md-{{ .cluster.name }}-"}).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should return error for invalid MachinePool naming.template",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra2").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap2").Build()).
						WithNaming(&clusterv1.MachinePoolClassNamingSpec{Template: "template-mp-{{ .cluster.name"}).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should return error for invalid MachinePool naming.template when the generated name does not conform to RFC 1123",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cp-infra1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra2").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap2").Build()).
						WithNaming(&clusterv1.MachinePoolClassNamingSpec{Template: "template-mp-{{ .cluster.name }}-"}).
						Build()).
				Build(),
			expectErr: true,
		},

		// CEL tests
		{
			name: "fail if x-kubernetes-validations has invalid rule: " +
				"new rule that uses opts that are not available with the current compatibility version",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithVariables(clusterv1.ClusterClassVariable{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								// Note: IP will be only available if the compatibility version is 1.30
								Rule: "ip(self).family() == 6",
							}},
						},
					},
				}).
				Build(),
			expectErr: true,
		},
		{
			name: "pass if x-kubernetes-validations has valid rule: " +
				"pre-existing rule that uses opts that are not available with the current compatibility version, but with the \"max\" env",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithVariables(clusterv1.ClusterClassVariable{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								// Note: IP will be only available if the compatibility version is 1.30
								Rule: "ip(self).family() == 6",
							}},
						},
					},
				}).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithVariables(clusterv1.ClusterClassVariable{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								// Note: IP will be only available if the compatibility version is 1.30
								Rule: "ip(self).family() == 6",
							}},
						},
					},
				}).
				Build(),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithIndex(&clusterv1.Cluster{}, index.ClusterClassRefPath, index.ClusterByClusterClassRef).
				Build()

			// Pin the compatibility version used in variable CEL validation to 1.29, so we don't have to continuously refactor
			// the unit tests that verify that compatibility is handled correctly.
			backupEnvSetVersion := variables.GetEnvSetVersion()
			defer func() {
				variables.SetEnvSetVersion(backupEnvSetVersion)
			}()
			variables.SetEnvSetVersion(version.MustParse("1.29"))

			// Create the webhook and add the fakeClient as its client.
			webhook := &ClusterClass{Client: fakeClient}
			err := webhook.validate(ctx, tt.old, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestClusterClassValidationWithClusterAwareChecks(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	tests := []struct {
		name            string
		oldClusterClass *clusterv1.ClusterClass
		newClusterClass *clusterv1.ClusterClass
		clusters        []client.Object
		expectErr       bool
	}{
		{
			name: "pass if a MachineDeploymentClass or MachinePoolClass not in use gets removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("bb").
									Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "error if a MachineDeploymentClass in use gets removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("bb").
									Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "error if a MachinePoolClass in use gets removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachinePool(
								builder.MachinePoolTopology("workers1").
									WithClass("bb").
									Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "error if many MachineDeploymentClasses, used in multiple Clusters using the modified ClusterClass, are removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("bb").
									Build(),
							).
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers2").
									WithClass("aa").
									Build(),
							).
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster2").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("aa").
									Build(),
							).
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers2").
									WithClass("aa").
									Build(),
							).
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster3").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("bb").
									Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "error if many MachinePoolClasses, used in multiple Clusters using the modified ClusterClass, are removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachinePool(
								builder.MachinePoolTopology("workers1").
									WithClass("bb").
									Build(),
							).
							WithMachinePool(
								builder.MachinePoolTopology("workers2").
									WithClass("aa").
									Build(),
							).
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster2").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachinePool(
								builder.MachinePoolTopology("workers1").
									WithClass("aa").
									Build(),
							).
							WithMachinePool(
								builder.MachinePoolTopology("workers2").
									WithClass("aa").
									Build(),
							).
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster3").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachinePool(
								builder.MachinePoolTopology("workers1").
									WithClass("bb").
									Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "error if a control plane MachineHealthCheck that is in use by a cluster is removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
							Enabled: ptr.To(true),
						}).
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneClassHealthCheck{
					Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
						UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
							{
								Type:           corev1.NodeReady,
								Status:         corev1.ConditionUnknown,
								TimeoutSeconds: ptr.To(int32(5 * 60)),
							},
						},
					},
				}).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "pass if a control plane MachineHealthCheck is removed but no cluster enforces it",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneClassHealthCheck{
					Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
						UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
							{
								Type:           corev1.NodeReady,
								Status:         corev1.ConditionUnknown,
								TimeoutSeconds: ptr.To(int32(5 * 60)),
							},
						},
					},
				}).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "pass if a control plane MachineHealthCheck is removed when clusters are not using it (clusters have overrides in topology)",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
							Enabled: ptr.To(true),
							Checks: clusterv1.ControlPlaneTopologyHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
							},
						}).
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneClassHealthCheck{
					Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
						UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
							{
								Type:           corev1.NodeReady,
								Status:         corev1.ConditionUnknown,
								TimeoutSeconds: ptr.To(int32(5 * 60)),
							},
						},
					},
				}).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "error if a MachineDeployment MachineHealthCheck that is in use by a cluster is removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						WithMachineDeployment(builder.MachineDeploymentTopology("md1").
							WithClass("mdclass1").
							WithMachineHealthCheck(clusterv1.MachineDeploymentTopologyHealthCheck{
								Enabled: ptr.To(true),
							}).
							Build()).
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
							},
						}).
						Build(),
				).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
				).
				Build(),
			expectErr: true,
		},
		{
			name: "pass if a MachineDeployment MachineHealthCheck is removed but no cluster enforces it",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						WithMachineDeployment(builder.MachineDeploymentTopology("md1").
							WithClass("mdclass1").
							Build()).
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
							},
						}).
						Build(),
				).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
				).
				Build(),
			expectErr: false,
		},
		{
			name: "pass if a MachineDeployment MachineHealthCheck is removed when clusters are not using it (clusters have overrides in topology)",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						WithMachineDeployment(builder.MachineDeploymentTopology("md1").
							WithClass("mdclass1").
							WithMachineHealthCheck(clusterv1.MachineDeploymentTopologyHealthCheck{
								Enabled: ptr.To(true),
								Checks: clusterv1.MachineDeploymentTopologyHealthCheckChecks{
									UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
										{
											Type:           corev1.NodeReady,
											Status:         corev1.ConditionUnknown,
											TimeoutSeconds: ptr.To(int32(5 * 60)),
										},
									},
								},
							}).
							Build()).
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
							},
						}).
						Build(),
				).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
				).
				Build(),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(tt.clusters...).
				WithIndex(&clusterv1.Cluster{}, index.ClusterClassRefPath, index.ClusterByClusterClassRef).
				Build()

			// Create the webhook and add the fakeClient as its client.
			webhook := &ClusterClass{Client: fakeClient}
			err := webhook.validate(ctx, tt.oldClusterClass, tt.newClusterClass)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestGetClustersUsingClusterClass(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	topology := builder.ClusterTopology().WithClass("class1")

	tests := []struct {
		name           string
		clusterClass   *clusterv1.ClusterClass
		clusters       []client.Object
		expectErr      bool
		expectClusters []client.Object
	}{
		{
			name:         "ClusterClass should return clusters referencing it",
			clusterClass: builder.ClusterClass("default", "class1").Build(),
			clusters: []client.Object{
				builder.Cluster("default", "cluster1").WithTopology(topology.Build()).Build(),
				builder.Cluster("default", "cluster2").Build(),
				builder.Cluster("other", "cluster2").WithTopology(topology.DeepCopy().WithClassNamespace("default").Build()).Build(),
				builder.Cluster("other", "cluster3").WithTopology(topology.Build()).Build(),
			},
			expectClusters: []client.Object{
				builder.Cluster("default", "cluster1").Build(),
				builder.Cluster("other", "cluster2").Build(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(tt.clusters...).
				WithIndex(&clusterv1.Cluster{}, index.ClusterClassRefPath, index.ClusterByClusterClassRef).
				Build()

			// Create the webhook and add the fakeClient as its client.
			webhook := &ClusterClass{Client: fakeClient}
			clusters, err := webhook.getClustersUsingClusterClass(ctx, tt.clusterClass)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			clusterKeys := []client.ObjectKey{}
			for _, c := range clusters {
				clusterKeys = append(clusterKeys, client.ObjectKeyFromObject(&c))
			}
			expectedKeys := []client.ObjectKey{}
			for _, c := range tt.expectClusters {
				expectedKeys = append(expectedKeys, client.ObjectKeyFromObject(c))
			}
			g.Expect(clusterKeys).To(Equal(expectedKeys))
		})
	}
}

func TestValidateAutoscalerAnnotationsForClusterClass(t *testing.T) {
	tests := []struct {
		name         string
		expectErr    bool
		clusters     []clusterv1.Cluster
		clusterClass *clusterv1.ClusterClass
	}{
		{
			name:      "replicas is set in one cluster, there is an autoscaler annotation on the matching ClusterClass MDC",
			expectErr: true,
			clusters: []clusterv1.Cluster{
				*builder.Cluster("ns", "cname1").Build(),
				*builder.Cluster("ns", "cname2").WithTopology(
					builder.ClusterTopology().
						WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
							WithClass("mdc1").
							WithReplicas(2).
							Build(),
						).
						Build()).
					Build(),
			},
			clusterClass: builder.ClusterClass("ns", "ccname1").
				WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("mdc1").
					WithAnnotations(map[string]string{
						clusterv1.AutoscalerMaxSizeAnnotation: "20",
					}).
					Build()).
				Build(),
		},
		{
			name:      "replicas is set in one cluster, there are no autoscaler annotation on the matching ClusterClass MDC",
			expectErr: false,
			clusters: []clusterv1.Cluster{
				*builder.Cluster("ns", "cname1").Build(),
				*builder.Cluster("ns", "cname2").WithTopology(
					builder.ClusterTopology().
						WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
							WithClass("mdc1").
							WithReplicas(2).
							Build(),
						).
						Build()).
					Build(),
			},
			clusterClass: builder.ClusterClass("ns", "ccname1").
				WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("mdc1").
					Build()).
				Build(),
		},
		{
			name:      "replicas is set in one cluster, but the ClusterClass has no annotations",
			expectErr: false,
			clusters: []clusterv1.Cluster{
				*builder.Cluster("ns", "cname1").Build(),
				*builder.Cluster("ns", "cname2").WithTopology(
					builder.ClusterTopology().
						WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
							WithClass("mdc1").
							WithReplicas(2).
							Build(),
						).
						Build()).
					Build(),
			},
			clusterClass: builder.ClusterClass("ns", "ccname1").Build(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := validateAutoscalerAnnotationsForClusterClass(tt.clusters, tt.clusterClass)
			if tt.expectErr {
				g.Expect(err).ToNot(BeEmpty())
			} else {
				g.Expect(err).To(BeEmpty())
			}
		})
	}
}

func invalidLabels() map[string]string {
	return map[string]string{
		"foo":          "$invalid-key",
		"bar":          strings.Repeat("a", 64) + "too-long-value",
		"/invalid-key": "foo",
	}
}

func invalidAnnotations() map[string]string {
	return map[string]string{
		"/invalid-key": "foo",
	}
}
