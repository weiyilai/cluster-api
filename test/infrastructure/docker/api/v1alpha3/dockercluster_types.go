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

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1alpha3 "sigs.k8s.io/cluster-api/internal/api/core/v1alpha3"
)

const (
	// ClusterFinalizer allows DockerClusterReconciler to clean up resources associated with DockerCluster before
	// removing it from the apiserver.
	ClusterFinalizer = "dockercluster.infrastructure.cluster.x-k8s.io"
)

// DockerClusterSpec defines the desired state of DockerCluster.
type DockerClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint"`

	// FailureDomains are not usulaly defined on the spec.
	// The docker provider is special since failure domains don't mean anything in a local docker environment.
	// Instead, the docker cluster controller will simply copy these into the Status and allow the Cluster API
	// controllers to do what they will with the defined failure domains.
	// +optional
	FailureDomains clusterv1alpha3.FailureDomains `json:"failureDomains,omitempty"`
}

// DockerClusterStatus defines the observed state of DockerCluster.
type DockerClusterStatus struct {
	// Ready denotes that the docker cluster (infrastructure) is ready.
	Ready bool `json:"ready"`

	// FailureDomains don't mean much in CAPD since it's all local, but we can see how the rest of cluster API
	// will use this if we populate it.
	FailureDomains clusterv1alpha3.FailureDomains `json:"failureDomains,omitempty"`

	// Conditions defines current service state of the DockerCluster.
	// +optional
	Conditions clusterv1alpha3.Conditions `json:"conditions,omitempty"`
}

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
	// Host is the hostname on which the API server is serving.
	Host string `json:"host"`

	// Port is the port on which the API server is serving.
	Port int `json:"port"`
}

// +kubebuilder:resource:path=dockerclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:unservedversion
// +kubebuilder:deprecatedversion

// DockerCluster is the Schema for the dockerclusters API.
//
// Deprecated: This type will be removed in one of the next releases.
type DockerCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DockerClusterSpec   `json:"spec,omitempty"`
	Status DockerClusterStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *DockerCluster) GetConditions() clusterv1alpha3.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *DockerCluster) SetConditions(conditions clusterv1alpha3.Conditions) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// DockerClusterList contains a list of DockerCluster.
//
// Deprecated: This type will be removed in one of the next releases.
type DockerClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DockerCluster `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &DockerCluster{}, &DockerClusterList{})
}
