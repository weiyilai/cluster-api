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

package machineset

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestCalculateTemplatesInUse(t *testing.T) {
	t.Run("Calculate templates in use with regular MachineDeployment and MachineSet", func(t *testing.T) {
		g := NewWithT(t)

		md := builder.MachineDeployment(metav1.NamespaceDefault, "md").
			WithBootstrapTemplate(builder.BootstrapTemplate(metav1.NamespaceDefault, "mdBT").Build()).
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT").Build()).
			Build()
		ms := builder.MachineSet(metav1.NamespaceDefault, "ms").
			WithBootstrapTemplate(builder.BootstrapTemplate(metav1.NamespaceDefault, "msBT").Build()).
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()).
			Build()

		actual, err := CalculateTemplatesInUse(md, []*clusterv1.MachineSet{ms})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actual).To(HaveLen(4))

		g.Expect(actual).To(HaveKey(templateRefID(md.Spec.Template.Spec.Bootstrap.ConfigRef)))
		g.Expect(actual).To(HaveKey(templateRefID(md.Spec.Template.Spec.InfrastructureRef)))

		g.Expect(actual).To(HaveKey(templateRefID(ms.Spec.Template.Spec.Bootstrap.ConfigRef)))
		g.Expect(actual).To(HaveKey(templateRefID(ms.Spec.Template.Spec.InfrastructureRef)))
	})

	t.Run("Calculate templates in use with MachineDeployment and MachineSet without BootstrapTemplate", func(t *testing.T) {
		g := NewWithT(t)

		mdWithoutBootstrapTemplate := builder.MachineDeployment(metav1.NamespaceDefault, "md").
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT").Build()).
			Build()
		msWithoutBootstrapTemplate := builder.MachineSet(metav1.NamespaceDefault, "ms").
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()).
			Build()

		actual, err := CalculateTemplatesInUse(mdWithoutBootstrapTemplate, []*clusterv1.MachineSet{msWithoutBootstrapTemplate})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actual).To(HaveLen(2))

		g.Expect(actual).To(HaveKey(templateRefID(mdWithoutBootstrapTemplate.Spec.Template.Spec.InfrastructureRef)))

		g.Expect(actual).To(HaveKey(templateRefID(msWithoutBootstrapTemplate.Spec.Template.Spec.InfrastructureRef)))
	})

	t.Run("Calculate templates in use with MachineDeployment and MachineSet ignore templates when resources in deleting", func(t *testing.T) {
		g := NewWithT(t)

		deletionTimeStamp := metav1.Now()

		mdInDeleting := builder.MachineDeployment(metav1.NamespaceDefault, "md").
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT").Build()).
			Build()
		mdInDeleting.SetDeletionTimestamp(&deletionTimeStamp)

		msInDeleting := builder.MachineSet(metav1.NamespaceDefault, "ms").
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()).
			Build()
		msInDeleting.SetDeletionTimestamp(&deletionTimeStamp)

		actual, err := CalculateTemplatesInUse(mdInDeleting, []*clusterv1.MachineSet{msInDeleting})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actual).To(BeEmpty())

		g.Expect(actual).ToNot(HaveKey(templateRefID(mdInDeleting.Spec.Template.Spec.InfrastructureRef)))

		g.Expect(actual).ToNot(HaveKey(templateRefID(msInDeleting.Spec.Template.Spec.InfrastructureRef)))
	})

	t.Run("Calculate templates in use without MachineDeployment and with MachineSet", func(t *testing.T) {
		g := NewWithT(t)

		ms := builder.MachineSet(metav1.NamespaceDefault, "ms").
			WithBootstrapTemplate(builder.BootstrapTemplate(metav1.NamespaceDefault, "msBT").Build()).
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()).
			Build()

		actual, err := CalculateTemplatesInUse(nil, []*clusterv1.MachineSet{ms})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actual).To(HaveLen(2))

		g.Expect(actual).To(HaveKey(templateRefID(ms.Spec.Template.Spec.Bootstrap.ConfigRef)))
		g.Expect(actual).To(HaveKey(templateRefID(ms.Spec.Template.Spec.InfrastructureRef)))
	})
}
