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

package check

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

type referencedObjectsCompatibilityTestCase struct {
	name    string
	current *unstructured.Unstructured
	desired *unstructured.Unstructured
	wantErr bool
}

var referencedObjectsCompatibilityTestCases = []referencedObjectsCompatibilityTestCase{
	{
		name: "Fails if group changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "foo/vx",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "bar/vx",
			},
		},
		wantErr: true,
	},
	{
		name: "Fails if kind changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": "bar",
			},
		},
		wantErr: true,
	},
	{
		name: "Pass if gvk remains the same",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Pass if version changes but group and kind remains the same",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/bar",
				"kind":       "foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Fails if namespace changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"namespace": "foo",
				},
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"namespace": "bar",
				},
			},
		},
		wantErr: true,
	},
}

func TestObjectsAreCompatible(t *testing.T) {
	for _, tt := range referencedObjectsCompatibilityTestCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := ObjectsAreCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestObjectsAreStrictlyCompatible(t *testing.T) {
	referencedObjectsStrictCompatibilityTestCases := append(referencedObjectsCompatibilityTestCases, []referencedObjectsCompatibilityTestCase{
		{
			name: "Fails if name changes",
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			desired: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "bar",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Pass if name remains the same",
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			desired: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			wantErr: false,
		},
	}...)

	for _, tt := range referencedObjectsStrictCompatibilityTestCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			allErrs := ObjectsAreStrictlyCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestClusterClassTemplateAreCompatible(t *testing.T) {
	templateRef := clusterv1.ClusterClassTemplateReference{
		Name:       "foo",
		Kind:       "bar",
		APIVersion: "test.group.io/versionone",
	}
	compatibleNameChangeTemplateRef := clusterv1.ClusterClassTemplateReference{
		Name:       "newFoo",
		Kind:       "bar",
		APIVersion: "test.group.io/versionone",
	}
	compatibleAPIVersionChangeTemplateRef := clusterv1.ClusterClassTemplateReference{
		Name:       "foo",
		Kind:       "bar",
		APIVersion: "test.group.io/versiontwo",
	}
	incompatibleAPIGroupChangeTemplateRef := clusterv1.ClusterClassTemplateReference{
		Name:       "foo",
		Kind:       "bar",
		APIVersion: "production.group.io/versionone",
	}
	incompatibleAPIKindChangeTemplateRef := clusterv1.ClusterClassTemplateReference{
		Name:       "foo",
		Kind:       "notabar",
		APIVersion: "test.group.io/versionone",
	}
	tests := []struct {
		name    string
		current clusterv1.ClusterClassTemplateReference
		desired clusterv1.ClusterClassTemplateReference
		wantErr bool
	}{
		{
			name:    "Allow change to template name",
			current: templateRef,
			desired: compatibleNameChangeTemplateRef,
			wantErr: false,
		},
		{
			name:    "Allow change to template APIVersion",
			current: templateRef,
			desired: compatibleAPIVersionChangeTemplateRef,
			wantErr: false,
		},
		{
			name:    "Block change to template API Group",
			current: templateRef,
			desired: incompatibleAPIGroupChangeTemplateRef,
			wantErr: true,
		},
		{
			name:    "Block change to template API Kind",
			current: templateRef,
			desired: incompatibleAPIKindChangeTemplateRef,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := ClusterClassTemplateAreCompatible(tt.current, tt.desired, field.NewPath("spec"))
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestClusterClassTemplateIsValid(t *testing.T) {
	pathPrefix := field.NewPath("this", "is", "a", "prefix")

	validTemplateRef := clusterv1.ClusterClassTemplateReference{
		Name:       "valid",
		Kind:       "barTemplate",
		APIVersion: "test.group.io/versionone",
	}

	emptyTemplateRef := clusterv1.ClusterClassTemplateReference{}

	emptyNameTemplateRef := clusterv1.ClusterClassTemplateReference{
		Name:       "",
		Kind:       "barTemplate",
		APIVersion: "test.group.io/versionone",
	}
	notTemplateKindTemplateRef := clusterv1.ClusterClassTemplateReference{
		Name:       "foo",
		Kind:       "bar",
		APIVersion: "test.group.io/versionone",
	}
	invalidAPIVersionTemplateRef := clusterv1.ClusterClassTemplateReference{
		Name:       "foo",
		Kind:       "barTemplate",
		APIVersion: "this/has/too/many/slashes",
	}
	emptyAPIVersionTemplateRef := clusterv1.ClusterClassTemplateReference{
		Name:       "foo",
		Kind:       "barTemplate",
		APIVersion: "",
	}

	tests := []struct {
		templateRef clusterv1.ClusterClassTemplateReference
		name        string
		wantErr     bool
	}{
		{
			name:        "No error with valid Template",
			templateRef: validTemplateRef,
			wantErr:     false,
		},

		{
			name:        "Invalid if ref is nil",
			templateRef: emptyTemplateRef,
			wantErr:     true,
		},
		{
			name:        "Invalid if name is empty",
			templateRef: emptyNameTemplateRef,
			wantErr:     true,
		},
		{
			name:        "Invalid if Kind doesn't have Template suffix",
			templateRef: notTemplateKindTemplateRef,
			wantErr:     true,
		},
		{
			name:        "Invalid if apiVersion is not valid",
			templateRef: invalidAPIVersionTemplateRef,
			wantErr:     true,
		},
		{
			name:        "Empty apiVersion is not valid",
			templateRef: emptyAPIVersionTemplateRef,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := ClusterClassTemplateIsValid(tt.templateRef, pathPrefix)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestClusterClassesAreCompatible(t *testing.T) {
	ref := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
	}
	incompatibleRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "baz",
	}
	compatibleRefOther := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/another-foo",
		Kind:       "barTemplate",
		Name:       "another-baz",
	}

	tests := []struct {
		name    string
		current *clusterv1.ClusterClass
		desired *clusterv1.ClusterClass
		wantErr bool
	}{
		{
			name:    "error if current is nil",
			current: nil,
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				Build(),
			wantErr: true,
		},
		{
			name: "error if desired is nil",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				Build(),
			desired: nil,
			wantErr: true,
		},

		{
			name: "pass for compatible clusterClasses (with different namespace)",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
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
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			desired: builder.ClusterClass("other", "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate("other", "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(compatibleRefOther)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(compatibleRefOther)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate("other", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("other", "bootstrap1").Build()).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate("other", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("other", "bootstrap1").Build()).
						Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "error if clusterClass has incompatible ControlPlane ref",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
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
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(incompatibleRef)).
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
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "pass for incompatible ref in MachineDeploymentClass bootstrapTemplate clusterClasses",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
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
							refToUnstructured(ref)).Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
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
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "pass for incompatible ref in MachinePoolClass bootstrapTemplate clusterClasses",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(ref)).Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "pass if machineDeploymentClass is removed from ClusterClass",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
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
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
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
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "pass if machinePoolClass is removed from ClusterClass",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
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
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		g := NewWithT(t)
		t.Run(tt.name, func(*testing.T) {
			allErrs := ClusterClassesAreCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestMachineDeploymentClassesAreCompatible(t *testing.T) {
	ref := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
	}
	compatibleRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/another-foo",
		Kind:       "barTemplate",
		Name:       "another-baz",
	}
	incompatibleRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "baz",
	}

	tests := []struct {
		name    string
		current *clusterv1.ClusterClass
		desired *clusterv1.ClusterClass
		wantErr bool
	}{
		{
			name: "pass if machineDeploymentClasses are compatible",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(compatibleRef)).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "pass if machineDeploymentClass is removed from ClusterClass",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
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
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
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
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "error if machineDeploymentClass has multiple incompatible references",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build(),
				).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(incompatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(incompatibleRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(incompatibleRef)).
						WithBootstrapTemplate(
							refToUnstructured(compatibleRef)).Build()).
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := MachineDeploymentClassesAreCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestMachinePoolClassesAreCompatible(t *testing.T) {
	ref := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
	}
	compatibleRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/another-foo",
		Kind:       "barTemplate",
		Name:       "another-baz",
	}
	incompatibleRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "baz",
	}

	tests := []struct {
		name    string
		current *clusterv1.ClusterClass
		desired *clusterv1.ClusterClass
		wantErr bool
	}{
		{
			name: "pass if MachinePoolClasses are compatible",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(compatibleRef)).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "pass if MachinePoolClass is removed from ClusterClass",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
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
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "error if MachinePoolClass has multiple incompatible references",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build(),
				).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(incompatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(incompatibleRef)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(incompatibleRef)).
						WithBootstrapTemplate(
							refToUnstructured(compatibleRef)).Build()).
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := MachinePoolClassesAreCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestMachineDeploymentClassesAreUnique(t *testing.T) {
	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		wantErr      bool
	}{
		{
			name: "pass if MachineDeploymentClasses are unique",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
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
			wantErr: false,
		},
		{
			name: "fail if MachineDeploymentClasses are duplicated",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "fail if multiple MachineDeploymentClasses are identical",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
				).
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := MachineDeploymentClassesAreUnique(tt.clusterClass)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestMachinePoolClassesAreUnique(t *testing.T) {
	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		wantErr      bool
	}{
		{
			name: "pass if MachinePoolClasses are unique",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
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
			wantErr: false,
		},
		{
			name: "fail if MachinePoolClasses are duplicated",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "fail if multiple MachinePoolClasses are identical",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
				).
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := MachinePoolClassesAreUnique(tt.clusterClass)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestMachineDeploymentTopologiesAreUniqueAndDefinedInClusterClass(t *testing.T) {
	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		cluster      *clusterv1.Cluster
		wantErr      bool
	}{
		{
			name: "fail if MachineDeploymentTopologies name is empty",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						// The name should not be empty.
						builder.MachineDeploymentTopology("").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if MachineDeploymentTopologies are unique and defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers1").
								WithClass("aa").
								Build()).
						Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "fail if MachineDeploymentTopologies are unique but not defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers1").
								WithClass("bb").
								Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "fail if MachineDeploymentTopologies are not unique but is defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers1").
								WithClass("aa").
								Build()).
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers1").
								WithClass("aa").
								Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if MachineDeploymentTopologies are unique and share a class that is defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers1").
								WithClass("aa").
								Build()).
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers2").
								WithClass("aa").
								Build()).
						Build()).
				Build(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := MachineDeploymentTopologiesAreValidAndDefinedInClusterClass(tt.cluster, tt.clusterClass)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestMachinePoolTopologiesAreUniqueAndDefinedInClusterClass(t *testing.T) {
	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		cluster      *clusterv1.Cluster
		wantErr      bool
	}{
		{
			name: "fail if MachinePoolTopologies name is empty",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						// The name should not be empty.
						builder.MachinePoolTopology("").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if MachinePoolTopologies are unique and defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachinePool(
							builder.MachinePoolTopology("workers1").
								WithClass("aa").
								Build()).
						Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "fail if MachinePoolTopologies are unique but not defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachinePool(
							builder.MachinePoolTopology("workers1").
								WithClass("bb").
								Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "fail if MachinePoolTopologies are not unique but is defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachinePool(
							builder.MachinePoolTopology("workers1").
								WithClass("aa").
								Build()).
						WithMachinePool(
							builder.MachinePoolTopology("workers1").
								WithClass("aa").
								Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if MachinePoolTopologies are unique and share a class that is defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachinePool(
							builder.MachinePoolTopology("workers1").
								WithClass("aa").
								Build()).
						WithMachinePool(
							builder.MachinePoolTopology("workers2").
								WithClass("aa").
								Build()).
						Build()).
				Build(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := MachinePoolTopologiesAreValidAndDefinedInClusterClass(tt.cluster, tt.clusterClass)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestClusterClassTemplatesAreValid(t *testing.T) {
	ref := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
	}
	invalidRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "",
	}

	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		wantErr      bool
	}{
		{
			name: "pass for clusterClass with valid references",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(ref)).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "error if clusterClass has multiple invalid md refs",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(invalidRef)).
				WithControlPlaneTemplate(
					refToUnstructured(invalidRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(invalidRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("a").
						WithInfrastructureTemplate(
							refToUnstructured(invalidRef)).
						WithBootstrapTemplate(
							refToUnstructured(invalidRef)).
						Build(),
					*builder.MachineDeploymentClass("b").
						WithInfrastructureTemplate(
							refToUnstructured(invalidRef)).
						WithBootstrapTemplate(
							refToUnstructured(invalidRef)).
						Build(),
					*builder.MachineDeploymentClass("c").
						WithInfrastructureTemplate(
							refToUnstructured(invalidRef)).
						WithBootstrapTemplate(
							refToUnstructured(invalidRef)).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "error if clusterClass has multiple invalid mp refs",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(invalidRef)).
				WithControlPlaneTemplate(
					refToUnstructured(invalidRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(invalidRef)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("a").
						WithInfrastructureTemplate(
							refToUnstructured(invalidRef)).
						WithBootstrapTemplate(
							refToUnstructured(invalidRef)).
						Build(),
					*builder.MachinePoolClass("b").
						WithInfrastructureTemplate(
							refToUnstructured(invalidRef)).
						WithBootstrapTemplate(
							refToUnstructured(invalidRef)).
						Build(),
					*builder.MachinePoolClass("c").
						WithInfrastructureTemplate(
							refToUnstructured(invalidRef)).
						WithBootstrapTemplate(
							refToUnstructured(invalidRef)).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "error if clusterClass has invalid ControlPlane ref",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(ref)).
				WithControlPlaneTemplate(
					refToUnstructured(invalidRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build()).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build()).
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := ClusterClassTemplatesAreValid(tt.clusterClass)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func refToUnstructured(ref *clusterv1.ClusterClassTemplateReference) *unstructured.Unstructured {
	gvk := ref.GroupVersionKind()
	output := &unstructured.Unstructured{}
	output.SetKind(gvk.Kind)
	output.SetAPIVersion(gvk.GroupVersion().String())
	output.SetName(ref.Name)
	return output
}
