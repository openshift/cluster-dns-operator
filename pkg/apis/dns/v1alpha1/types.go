package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ClusterDNSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ClusterDNS `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ClusterDNS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClusterDNSSpec   `json:"spec"`
	Status            ClusterDNSStatus `json:"status,omitempty"`
}

type ClusterDNSSpec struct {
	ClusterIP *string `json:"clusterIP"`

	ClusterDomain *string `json:"clusterDomain"`
}
type ClusterDNSStatus struct {
	// Fill me
}
