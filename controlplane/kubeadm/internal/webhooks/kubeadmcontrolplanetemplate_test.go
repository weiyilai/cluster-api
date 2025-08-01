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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
)

func TestKubeadmControlPlaneTemplateValidationFeatureGateEnabled(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	t.Run("create kubeadmcontrolplanetemplate should pass if gate enabled and valid kubeadmcontrolplanetemplate", func(t *testing.T) {
		testnamespace := "test"
		g := NewWithT(t)
		kcpTemplate := &controlplanev1.KubeadmControlPlaneTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubeadmcontrolplanetemplate-test",
				Namespace: testnamespace,
			},
			Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
				Template: controlplanev1.KubeadmControlPlaneTemplateResource{
					Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
						MachineTemplate: controlplanev1.KubeadmControlPlaneTemplateMachineTemplate{
							Spec: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateSpec{
								Deletion: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateDeletionSpec{
									NodeDrainTimeoutSeconds: ptr.To(int32(1)),
								},
							},
						},
					},
				},
			},
		}
		webhook := &KubeadmControlPlaneTemplate{}
		warnings, err := webhook.ValidateCreate(ctx, kcpTemplate)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(warnings).To(BeEmpty())
	})
}

func TestKubeadmControlPlaneTemplateValidationFeatureGateDisabled(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create KubeadmControlPlaneTemplate.
	t.Run("create kubeadmcontrolplanetemplate should not pass if gate disabled and valid kubeadmcontrolplanetemplate", func(t *testing.T) {
		testnamespace := "test"
		g := NewWithT(t)
		kcpTemplate := &controlplanev1.KubeadmControlPlaneTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubeadmcontrolplanetemplate-test",
				Namespace: testnamespace,
			},
			Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
				Template: controlplanev1.KubeadmControlPlaneTemplateResource{
					Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
						MachineTemplate: controlplanev1.KubeadmControlPlaneTemplateMachineTemplate{
							Spec: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateSpec{
								Deletion: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateDeletionSpec{
									NodeDrainTimeoutSeconds: ptr.To(int32(1)),
								},
							},
						},
					},
				},
			},
		}
		webhook := &KubeadmControlPlaneTemplate{}
		warnings, err := webhook.ValidateCreate(ctx, kcpTemplate)
		g.Expect(err).To(HaveOccurred())
		g.Expect(warnings).To(BeEmpty())
	})
}

func TestKubeadmControlPlaneTemplateValidationMetadata(t *testing.T) {
	t.Run("create kubeadmcontrolplanetemplate should not pass if metadata is invalid", func(t *testing.T) {
		g := NewWithT(t)
		kcpTemplate := &controlplanev1.KubeadmControlPlaneTemplate{
			Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
				Template: controlplanev1.KubeadmControlPlaneTemplateResource{
					ObjectMeta: clusterv1.ObjectMeta{
						Labels: map[string]string{
							"foo":          "$invalid-key",
							"bar":          strings.Repeat("a", 64) + "too-long-value",
							"/invalid-key": "foo",
						},
						Annotations: map[string]string{
							"/invalid-key": "foo",
						},
					},
					Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
						MachineTemplate: controlplanev1.KubeadmControlPlaneTemplateMachineTemplate{
							ObjectMeta: clusterv1.ObjectMeta{
								Labels: map[string]string{
									"foo":          "$invalid-key",
									"bar":          strings.Repeat("a", 64) + "too-long-value",
									"/invalid-key": "foo",
								},
								Annotations: map[string]string{
									"/invalid-key": "foo",
								},
							},
						},
					},
				},
			},
		}
		webhook := &KubeadmControlPlaneTemplate{}
		warnings, err := webhook.ValidateCreate(ctx, kcpTemplate)
		g.Expect(err).To(HaveOccurred())
		g.Expect(warnings).To(BeEmpty())
	})
}

func TestKubeadmControlPlaneTemplateUpdateValidation(t *testing.T) {
	t.Run("update KubeadmControlPlaneTemplate should pass if only defaulted fields are different", func(t *testing.T) {
		g := NewWithT(t)
		oldKCPTemplate := &controlplanev1.KubeadmControlPlaneTemplate{
			Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
				Template: controlplanev1.KubeadmControlPlaneTemplateResource{
					Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
						MachineTemplate: controlplanev1.KubeadmControlPlaneTemplateMachineTemplate{
							Spec: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateSpec{
								Deletion: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateDeletionSpec{
									NodeDrainTimeoutSeconds: ptr.To(int32(10 * 60)),
								},
							},
						},
					},
				},
			},
		}
		newKCPTemplate := &controlplanev1.KubeadmControlPlaneTemplate{
			Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
				Template: controlplanev1.KubeadmControlPlaneTemplateResource{
					Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
						KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
							// Only this field is different, but defaulting will set it as well, so this should pass the immutability check.
							Format: bootstrapv1.CloudConfig,
						},
						MachineTemplate: controlplanev1.KubeadmControlPlaneTemplateMachineTemplate{
							Spec: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateSpec{
								Deletion: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateDeletionSpec{
									NodeDrainTimeoutSeconds: ptr.To(int32(10 * 60)),
								},
							},
						},
					},
				},
			},
		}
		webhook := &KubeadmControlPlaneTemplate{}
		warnings, err := webhook.ValidateUpdate(ctx, oldKCPTemplate, newKCPTemplate)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(warnings).To(BeEmpty())
	})
	t.Run("update kubeadmcontrolplanetemplate should not pass if fields are different", func(t *testing.T) {
		g := NewWithT(t)
		oldKCPTemplate := &controlplanev1.KubeadmControlPlaneTemplate{
			Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
				Template: controlplanev1.KubeadmControlPlaneTemplateResource{
					Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
						MachineTemplate: controlplanev1.KubeadmControlPlaneTemplateMachineTemplate{
							Spec: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateSpec{
								Deletion: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateDeletionSpec{
									NodeDrainTimeoutSeconds: ptr.To(int32(10 * 60)),
								},
							},
						},
					},
				},
			},
		}
		newKCPTemplate := &controlplanev1.KubeadmControlPlaneTemplate{
			Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
				Template: controlplanev1.KubeadmControlPlaneTemplateResource{
					Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
						KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
							// Defaulting will set this field as well.
							Format: bootstrapv1.CloudConfig,
							// This will fail the immutability check.
							PreKubeadmCommands: []string{
								"new-cmd",
							},
						},
						MachineTemplate: controlplanev1.KubeadmControlPlaneTemplateMachineTemplate{
							Spec: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateSpec{
								Deletion: controlplanev1.KubeadmControlPlaneTemplateMachineTemplateDeletionSpec{
									NodeDrainTimeoutSeconds: ptr.To(int32(10 * 60)),
								},
							},
						},
					},
				},
			},
		}
		webhook := &KubeadmControlPlaneTemplate{}
		warnings, err := webhook.ValidateUpdate(ctx, oldKCPTemplate, newKCPTemplate)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("KubeadmControlPlaneTemplate spec.template.spec field is immutable"))
		g.Expect(warnings).To(BeEmpty())
	})
	t.Run("update KubeadmControlPlaneTemplate should pass when transitioning from the previously defaulted rolloutStrategy to an unset rolloutStrategy", func(t *testing.T) {
		g := NewWithT(t)
		oldKCPTemplate := &controlplanev1.KubeadmControlPlaneTemplate{
			Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
				Template: controlplanev1.KubeadmControlPlaneTemplateResource{
					Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
						Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{
							Strategy: controlplanev1.KubeadmControlPlaneRolloutStrategy{
								Type: controlplanev1.RollingUpdateStrategyType,
								RollingUpdate: controlplanev1.KubeadmControlPlaneRolloutStrategyRollingUpdate{
									MaxSurge: ptr.To(intstr.FromInt32(1)),
								},
							},
						},
					},
				},
			},
		}
		newKCPTemplate := &controlplanev1.KubeadmControlPlaneTemplate{
			Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
				Template: controlplanev1.KubeadmControlPlaneTemplateResource{
					Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
						Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{},
					},
				},
			},
		}
		webhook := &KubeadmControlPlaneTemplate{}
		warnings, err := webhook.ValidateUpdate(ctx, oldKCPTemplate, newKCPTemplate)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(warnings).To(BeEmpty())
	})
}
