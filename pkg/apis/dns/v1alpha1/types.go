package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDNSList contains a list of ClusterDNS
type ClusterDNSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ClusterDNS `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDNS is the Schema for the clusterdnses API
// +k8s:openapi-gen=true
type ClusterDNS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClusterDNSSpec   `json:"spec"`
	Status            ClusterDNSStatus `json:"status,omitempty"`
}

type ClusterDNSSpec struct {
}

// ClusterDNSStatus defines the observed state of ClusterDNS
type ClusterDNSStatus struct {
	// ClusterIP is the service IP reserved for cluster DNS service
	ClusterIP string `json:"clusterIP"`
	// ClusterDomain is the internal domain used in the cluster
	ClusterDomain string `json:"clusterDomain"`
}
