// Copyright 2022 Authors of spidernet-io
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// EgressClusterInfoList contains a list of EgressClusterStatus
// +kubebuilder:object:root=true
type EgressClusterInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []EgressClusterInfo `json:"items"`
}

// EgressClusterInfo describes the status of cluster
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories={egressclusterinfo},path="egressclusterinfos",singular="egressclusterinfo",scope="Cluster",shortName={eci}
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +genclient
// +genclient:nonNamespaced
type EgressClusterInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	// +kubebuilder:validation:Optional
	Spec EgressClusterStatusSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Optional
	Status EgressClusterStatus `json:"status,omitempty"`
}

type EgressClusterStatusSpec struct{}

type EgressClusterStatus struct {
	// +kubebuilder:validation:Optional
	EgressIgnoreCIDR EgressIgnoreCIDR `json:"egressIgnoreCIDR,omitempty"`
}

type EgressIgnoreCIDR struct {
	// +kubebuilder:validation:Optional
	NodeIP IPListPair `json:"nodeIP,omitempty"`
	// +kubebuilder:validation:Optional
	ClusterIP IPListPair `json:"clusterIP,omitempty"`
	// +kubebuilder:validation:Optional
	PodCIDR IPListPair `json:"podCIDR,omitempty"`
}

type IPListPair struct {
	// +kubebuilder:validation:Optional
	IPv4 []string `json:"ipv4,omitempty"`
	// +kubebuilder:validation:Optional
	IPv6 []string `json:"ipv6,omitempty"`
}

func init() {
	SchemeBuilder.Register(&EgressClusterInfo{}, &EgressClusterInfoList{})
}
