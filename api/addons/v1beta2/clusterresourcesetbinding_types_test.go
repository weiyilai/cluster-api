/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestIsResourceApplied(t *testing.T) {
	resourceRefApplyFailed := ResourceRef{
		Name: "applyFailed",
		Kind: "Secret",
	}
	resourceRefApplySucceeded := ResourceRef{
		Name: "ApplySucceeded",
		Kind: "Secret",
	}
	resourceRefNotExist := ResourceRef{
		Name: "notExist",
		Kind: "Secret",
	}
	CRSBinding := &ResourceSetBinding{
		ClusterResourceSetName: "test-clusterResourceSet",
		Resources: []ResourceBinding{
			{
				ResourceRef:     resourceRefApplySucceeded,
				Applied:         ptr.To(true),
				Hash:            "xyz",
				LastAppliedTime: metav1.Time{Time: time.Now().UTC()},
			},
			{
				ResourceRef:     resourceRefApplyFailed,
				Applied:         ptr.To(false),
				Hash:            "",
				LastAppliedTime: metav1.Time{Time: time.Now().UTC()},
			},
		},
	}

	tests := []struct {
		name               string
		resourceSetBinding *ResourceSetBinding
		resourceRef        ResourceRef
		isApplied          bool
	}{
		{
			name:               "should return true if the resource is applied successfully",
			resourceSetBinding: CRSBinding,
			resourceRef:        resourceRefApplySucceeded,
			isApplied:          true,
		},
		{
			name:               "should return false if the resource apply failed",
			resourceSetBinding: CRSBinding,
			resourceRef:        resourceRefApplyFailed,
			isApplied:          false,
		},
		{
			name:               "should return false if the resource does not exist",
			resourceSetBinding: CRSBinding,
			resourceRef:        resourceRefNotExist,
			isApplied:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)
			gs.Expect(tt.resourceSetBinding.IsApplied(tt.resourceRef)).To(BeEquivalentTo(tt.isApplied))
		})
	}
}

func TestResourceSetBindingGetResourceBinding(t *testing.T) {
	resourceRefApplyFailed := ResourceRef{
		Name: "applyFailed",
		Kind: "Secret",
	}
	resourceRefApplySucceeded := ResourceRef{
		Name: "ApplySucceeded",
		Kind: "Secret",
	}
	resourceRefNotExist := ResourceRef{
		Name: "notExist",
		Kind: "Secret",
	}

	resourceRefApplyFailedBinding := ResourceBinding{
		ResourceRef:     resourceRefApplyFailed,
		Applied:         ptr.To(false),
		Hash:            "",
		LastAppliedTime: metav1.Time{Time: time.Now().UTC()},
	}
	crsBinding := &ResourceSetBinding{
		ClusterResourceSetName: "test-clusterResourceSet",
		Resources: []ResourceBinding{
			{
				ResourceRef:     resourceRefApplySucceeded,
				Applied:         ptr.To(true),
				Hash:            "xyz",
				LastAppliedTime: metav1.Time{Time: time.Now().UTC()},
			},
			resourceRefApplyFailedBinding,
		},
	}

	tests := []struct {
		name               string
		resourceSetBinding *ResourceSetBinding
		resourceRef        ResourceRef
		want               *ResourceBinding
	}{
		{
			name:               "ResourceRef doesn't exist",
			resourceSetBinding: crsBinding,
			resourceRef:        resourceRefNotExist,
			want:               nil,
		},
		{
			name:               "ResourceRef exists",
			resourceSetBinding: crsBinding,
			resourceRef:        resourceRefApplyFailed,
			want:               &resourceRefApplyFailedBinding,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)
			gs.Expect(tt.resourceSetBinding.GetResource(tt.resourceRef)).To(BeComparableTo(tt.want))
		})
	}
}

func TestSetResourceBinding(t *testing.T) {
	resourceRefApplyFailed := ResourceRef{
		Name: "applyFailed",
		Kind: "Secret",
	}

	CRSBinding := &ResourceSetBinding{
		ClusterResourceSetName: "test-clusterResourceSet",
		Resources: []ResourceBinding{
			{
				ResourceRef:     resourceRefApplyFailed,
				Applied:         ptr.To(false),
				Hash:            "",
				LastAppliedTime: metav1.Time{Time: time.Now().UTC()},
			},
		},
	}
	updateFailedResourceBinding := ResourceBinding{
		ResourceRef:     resourceRefApplyFailed,
		Applied:         ptr.To(true),
		Hash:            "xyz",
		LastAppliedTime: metav1.Time{Time: time.Now().UTC()},
	}

	newResourceBinding := ResourceBinding{
		ResourceRef: ResourceRef{
			Name: "newBinding",
			Kind: "Secret",
		},
		Applied:         ptr.To(false),
		Hash:            "xyz",
		LastAppliedTime: metav1.Time{Time: time.Now().UTC()},
	}

	tests := []struct {
		name               string
		resourceSetBinding *ResourceSetBinding
		resourceBinding    ResourceBinding
	}{
		{
			name:               "should update resourceSetBinding with new resource binding if not exist",
			resourceSetBinding: CRSBinding,
			resourceBinding:    newResourceBinding,
		},
		{
			name:               "should update Applied if resource failed before",
			resourceSetBinding: CRSBinding,
			resourceBinding:    updateFailedResourceBinding,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)
			tt.resourceSetBinding.SetBinding(tt.resourceBinding)
			exist := false
			for _, b := range tt.resourceSetBinding.Resources {
				if cmp.Equal(b.ResourceRef, tt.resourceBinding.ResourceRef) {
					gs.Expect(tt.resourceBinding.Applied).To(BeEquivalentTo(b.Applied))
					exist = true
				}
			}
			gs.Expect(exist).To(BeTrue())
		})
	}
}
