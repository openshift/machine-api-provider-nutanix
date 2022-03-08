//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AWSPartitionPlacement) DeepCopyInto(out *AWSPartitionPlacement) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AWSPartitionPlacement.
func (in *AWSPartitionPlacement) DeepCopy() *AWSPartitionPlacement {
	if in == nil {
		return nil
	}
	out := new(AWSPartitionPlacement)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AWSPlacementGroup) DeepCopyInto(out *AWSPlacementGroup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AWSPlacementGroup.
func (in *AWSPlacementGroup) DeepCopy() *AWSPlacementGroup {
	if in == nil {
		return nil
	}
	out := new(AWSPlacementGroup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AWSPlacementGroup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AWSPlacementGroupList) DeepCopyInto(out *AWSPlacementGroupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]AWSPlacementGroup, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AWSPlacementGroupList.
func (in *AWSPlacementGroupList) DeepCopy() *AWSPlacementGroupList {
	if in == nil {
		return nil
	}
	out := new(AWSPlacementGroupList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AWSPlacementGroupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AWSPlacementGroupManagementSpec) DeepCopyInto(out *AWSPlacementGroupManagementSpec) {
	*out = *in
	if in.Managed != nil {
		in, out := &in.Managed, &out.Managed
		*out = new(ManagedAWSPlacementGroup)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AWSPlacementGroupManagementSpec.
func (in *AWSPlacementGroupManagementSpec) DeepCopy() *AWSPlacementGroupManagementSpec {
	if in == nil {
		return nil
	}
	out := new(AWSPlacementGroupManagementSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AWSPlacementGroupSpec) DeepCopyInto(out *AWSPlacementGroupSpec) {
	*out = *in
	in.ManagementSpec.DeepCopyInto(&out.ManagementSpec)
	if in.CredentialsSecret != nil {
		in, out := &in.CredentialsSecret, &out.CredentialsSecret
		*out = new(LocalSecretReference)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AWSPlacementGroupSpec.
func (in *AWSPlacementGroupSpec) DeepCopy() *AWSPlacementGroupSpec {
	if in == nil {
		return nil
	}
	out := new(AWSPlacementGroupSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AWSPlacementGroupStatus) DeepCopyInto(out *AWSPlacementGroupStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ExpiresAt != nil {
		in, out := &in.ExpiresAt, &out.ExpiresAt
		*out = (*in).DeepCopy()
	}
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	in.ObservedConfiguration.DeepCopyInto(&out.ObservedConfiguration)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AWSPlacementGroupStatus.
func (in *AWSPlacementGroupStatus) DeepCopy() *AWSPlacementGroupStatus {
	if in == nil {
		return nil
	}
	out := new(AWSPlacementGroupStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AlibabaCloudMachineProviderConfig) DeepCopyInto(out *AlibabaCloudMachineProviderConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.DataDisks != nil {
		in, out := &in.DataDisks, &out.DataDisks
		*out = make([]DataDiskProperties, len(*in))
		copy(*out, *in)
	}
	if in.SecurityGroups != nil {
		in, out := &in.SecurityGroups, &out.SecurityGroups
		*out = make([]AlibabaResourceReference, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	out.Bandwidth = in.Bandwidth
	out.SystemDisk = in.SystemDisk
	in.VSwitch.DeepCopyInto(&out.VSwitch)
	in.ResourceGroup.DeepCopyInto(&out.ResourceGroup)
	if in.UserDataSecret != nil {
		in, out := &in.UserDataSecret, &out.UserDataSecret
		*out = new(corev1.LocalObjectReference)
		**out = **in
	}
	if in.CredentialsSecret != nil {
		in, out := &in.CredentialsSecret, &out.CredentialsSecret
		*out = new(corev1.LocalObjectReference)
		**out = **in
	}
	if in.Tags != nil {
		in, out := &in.Tags, &out.Tags
		*out = make([]Tag, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AlibabaCloudMachineProviderConfig.
func (in *AlibabaCloudMachineProviderConfig) DeepCopy() *AlibabaCloudMachineProviderConfig {
	if in == nil {
		return nil
	}
	out := new(AlibabaCloudMachineProviderConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AlibabaCloudMachineProviderConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AlibabaCloudMachineProviderConfigList) DeepCopyInto(out *AlibabaCloudMachineProviderConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]AlibabaCloudMachineProviderConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AlibabaCloudMachineProviderConfigList.
func (in *AlibabaCloudMachineProviderConfigList) DeepCopy() *AlibabaCloudMachineProviderConfigList {
	if in == nil {
		return nil
	}
	out := new(AlibabaCloudMachineProviderConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AlibabaCloudMachineProviderConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AlibabaCloudMachineProviderStatus) DeepCopyInto(out *AlibabaCloudMachineProviderStatus) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.InstanceID != nil {
		in, out := &in.InstanceID, &out.InstanceID
		*out = new(string)
		**out = **in
	}
	if in.InstanceState != nil {
		in, out := &in.InstanceState, &out.InstanceState
		*out = new(string)
		**out = **in
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AlibabaCloudMachineProviderStatus.
func (in *AlibabaCloudMachineProviderStatus) DeepCopy() *AlibabaCloudMachineProviderStatus {
	if in == nil {
		return nil
	}
	out := new(AlibabaCloudMachineProviderStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AlibabaCloudMachineProviderStatus) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AlibabaResourceReference) DeepCopyInto(out *AlibabaResourceReference) {
	*out = *in
	if in.ID != nil {
		in, out := &in.ID, &out.ID
		*out = new(string)
		**out = **in
	}
	if in.Name != nil {
		in, out := &in.Name, &out.Name
		*out = new(string)
		**out = **in
	}
	if in.Tags != nil {
		in, out := &in.Tags, &out.Tags
		*out = new([]Tag)
		if **in != nil {
			in, out := *in, *out
			*out = make([]Tag, len(*in))
			copy(*out, *in)
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AlibabaResourceReference.
func (in *AlibabaResourceReference) DeepCopy() *AlibabaResourceReference {
	if in == nil {
		return nil
	}
	out := new(AlibabaResourceReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BandwidthProperties) DeepCopyInto(out *BandwidthProperties) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BandwidthProperties.
func (in *BandwidthProperties) DeepCopy() *BandwidthProperties {
	if in == nil {
		return nil
	}
	out := new(BandwidthProperties)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DataDiskProperties) DeepCopyInto(out *DataDiskProperties) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DataDiskProperties.
func (in *DataDiskProperties) DeepCopy() *DataDiskProperties {
	if in == nil {
		return nil
	}
	out := new(DataDiskProperties)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LocalSecretReference) DeepCopyInto(out *LocalSecretReference) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LocalSecretReference.
func (in *LocalSecretReference) DeepCopy() *LocalSecretReference {
	if in == nil {
		return nil
	}
	out := new(LocalSecretReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ManagedAWSPlacementGroup) DeepCopyInto(out *ManagedAWSPlacementGroup) {
	*out = *in
	if in.Partition != nil {
		in, out := &in.Partition, &out.Partition
		*out = new(AWSPartitionPlacement)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ManagedAWSPlacementGroup.
func (in *ManagedAWSPlacementGroup) DeepCopy() *ManagedAWSPlacementGroup {
	if in == nil {
		return nil
	}
	out := new(ManagedAWSPlacementGroup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NutanixMachineProviderConfig) DeepCopyInto(out *NutanixMachineProviderConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.ClusterReference != nil {
		in, out := &in.ClusterReference, &out.ClusterReference
		*out = new(NutanixReference)
		**out = **in
	}
	if in.ImageReference != nil {
		in, out := &in.ImageReference, &out.ImageReference
		*out = new(NutanixReference)
		**out = **in
	}
	if in.SubnetReference != nil {
		in, out := &in.SubnetReference, &out.SubnetReference
		*out = new(NutanixReference)
		**out = **in
	}
	out.MemorySize = in.MemorySize.DeepCopy()
	out.DiskSize = in.DiskSize.DeepCopy()
	if in.UserDataSecret != nil {
		in, out := &in.UserDataSecret, &out.UserDataSecret
		*out = new(corev1.LocalObjectReference)
		**out = **in
	}
	if in.CredentialsSecret != nil {
		in, out := &in.CredentialsSecret, &out.CredentialsSecret
		*out = new(corev1.LocalObjectReference)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NutanixMachineProviderConfig.
func (in *NutanixMachineProviderConfig) DeepCopy() *NutanixMachineProviderConfig {
	if in == nil {
		return nil
	}
	out := new(NutanixMachineProviderConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NutanixMachineProviderConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NutanixMachineProviderStatus) DeepCopyInto(out *NutanixMachineProviderStatus) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.VmState != nil {
		in, out := &in.VmState, &out.VmState
		*out = new(string)
		**out = **in
	}
	if in.VmUUID != nil {
		in, out := &in.VmUUID, &out.VmUUID
		*out = new(string)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NutanixMachineProviderStatus.
func (in *NutanixMachineProviderStatus) DeepCopy() *NutanixMachineProviderStatus {
	if in == nil {
		return nil
	}
	out := new(NutanixMachineProviderStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NutanixMachineProviderStatus) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NutanixReference) DeepCopyInto(out *NutanixReference) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NutanixReference.
func (in *NutanixReference) DeepCopy() *NutanixReference {
	if in == nil {
		return nil
	}
	out := new(NutanixReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SystemDiskProperties) DeepCopyInto(out *SystemDiskProperties) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SystemDiskProperties.
func (in *SystemDiskProperties) DeepCopy() *SystemDiskProperties {
	if in == nil {
		return nil
	}
	out := new(SystemDiskProperties)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Tag) DeepCopyInto(out *Tag) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Tag.
func (in *Tag) DeepCopy() *Tag {
	if in == nil {
		return nil
	}
	out := new(Tag)
	in.DeepCopyInto(out)
	return out
}