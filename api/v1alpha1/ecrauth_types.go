/*
Copyright 2026.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ECRAuthSpec defines the desired state of ECRAuth.
type ECRAuthSpec struct {
	// SecretName is the managed pull-secret name that will be created/updated in
	// the same namespace as this resource.
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`

	// Region is the AWS region used for ECR token retrieval.
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// RefreshInterval controls how often credentials are refreshed.
	// +kubebuilder:default:="11h"
	RefreshInterval *metav1.Duration `json:"refreshInterval,omitempty"`

	// RoleARN is an optional IAM role to assume before requesting an ECR token.
	// +optional
	RoleARN string `json:"roleArn,omitempty"`
}

// ECRAuthStatus defines the observed state of ECRAuth.
type ECRAuthStatus struct {
	// Conditions reports the current controller status.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ManagedSecretName is the target secret currently managed by this resource.
	ManagedSecretName string `json:"managedSecretName,omitempty"`

	// LastSuccessfulRefreshTime is the last time credentials were refreshed.
	LastSuccessfulRefreshTime *metav1.Time `json:"lastSuccessfulRefreshTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ecrauth
// +kubebuilder:printcolumn:name="Secret",type=string,JSONPath=`.status.managedSecretName`
// +kubebuilder:printcolumn:name="LastRefresh",type=date,JSONPath=`.status.lastSuccessfulRefreshTime`

// ECRAuth is the Schema for the ecrauths API.
type ECRAuth struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ECRAuthSpec   `json:"spec,omitempty"`
	Status ECRAuthStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ECRAuthList contains a list of ECRAuth.
type ECRAuthList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ECRAuth `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ECRAuth{}, &ECRAuthList{})
}
