/*
Copyright 2024.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HpaSpec defines the desired state of Hpa.
type HpaSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Hpa. Edit hpa_types.go to remove/update

	ScaleTarget ScaleTarget `json:"scaleTarget"`
	Jobs        []JobSpec   `json:"jobs"`
}

type JobSpec struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Size     int32  `json:"size"`
}

type ScaleTarget struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

// HpaStatus defines the observed state of Hpa.
type HpaStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	CurrentReplicas int32                  `json:"currentReplicas"`
	LastScaleTime   *metav1.Time           `json:"lastScaleTime"`
	LastRuntimes    map[string]metav1.Time `json:"lastRuntime"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.scaleTarget.name",description="目标工作负载"
// +kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=".spec.jobs[*].schedule",description="Cron 表达式"
// +kubebuilder:printcolumn:name="Target Size",type="integer",JSONPath=".spec.jobs[*].size",description="目标副本数"
// Hpa is the Schema for the hpas API.
type Hpa struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HpaSpec   `json:"spec,omitempty"`
	Status HpaStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HpaList contains a list of Hpa.
type HpaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Hpa `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Hpa{}, &HpaList{})
}
