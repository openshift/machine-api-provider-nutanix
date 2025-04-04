// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

import (
	configv1 "github.com/openshift/api/config/v1"
)

// PlatformStatusApplyConfiguration represents a declarative configuration of the PlatformStatus type for use
// with apply.
type PlatformStatusApplyConfiguration struct {
	Type         *configv1.PlatformType                        `json:"type,omitempty"`
	AWS          *AWSPlatformStatusApplyConfiguration          `json:"aws,omitempty"`
	Azure        *AzurePlatformStatusApplyConfiguration        `json:"azure,omitempty"`
	GCP          *GCPPlatformStatusApplyConfiguration          `json:"gcp,omitempty"`
	BareMetal    *BareMetalPlatformStatusApplyConfiguration    `json:"baremetal,omitempty"`
	OpenStack    *OpenStackPlatformStatusApplyConfiguration    `json:"openstack,omitempty"`
	Ovirt        *OvirtPlatformStatusApplyConfiguration        `json:"ovirt,omitempty"`
	VSphere      *VSpherePlatformStatusApplyConfiguration      `json:"vsphere,omitempty"`
	IBMCloud     *IBMCloudPlatformStatusApplyConfiguration     `json:"ibmcloud,omitempty"`
	Kubevirt     *KubevirtPlatformStatusApplyConfiguration     `json:"kubevirt,omitempty"`
	EquinixMetal *EquinixMetalPlatformStatusApplyConfiguration `json:"equinixMetal,omitempty"`
	PowerVS      *PowerVSPlatformStatusApplyConfiguration      `json:"powervs,omitempty"`
	AlibabaCloud *AlibabaCloudPlatformStatusApplyConfiguration `json:"alibabaCloud,omitempty"`
	Nutanix      *NutanixPlatformStatusApplyConfiguration      `json:"nutanix,omitempty"`
	External     *ExternalPlatformStatusApplyConfiguration     `json:"external,omitempty"`
}

// PlatformStatusApplyConfiguration constructs a declarative configuration of the PlatformStatus type for use with
// apply.
func PlatformStatus() *PlatformStatusApplyConfiguration {
	return &PlatformStatusApplyConfiguration{}
}

// WithType sets the Type field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Type field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithType(value configv1.PlatformType) *PlatformStatusApplyConfiguration {
	b.Type = &value
	return b
}

// WithAWS sets the AWS field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the AWS field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithAWS(value *AWSPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.AWS = value
	return b
}

// WithAzure sets the Azure field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Azure field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithAzure(value *AzurePlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.Azure = value
	return b
}

// WithGCP sets the GCP field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the GCP field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithGCP(value *GCPPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.GCP = value
	return b
}

// WithBareMetal sets the BareMetal field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the BareMetal field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithBareMetal(value *BareMetalPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.BareMetal = value
	return b
}

// WithOpenStack sets the OpenStack field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the OpenStack field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithOpenStack(value *OpenStackPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.OpenStack = value
	return b
}

// WithOvirt sets the Ovirt field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Ovirt field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithOvirt(value *OvirtPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.Ovirt = value
	return b
}

// WithVSphere sets the VSphere field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the VSphere field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithVSphere(value *VSpherePlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.VSphere = value
	return b
}

// WithIBMCloud sets the IBMCloud field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the IBMCloud field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithIBMCloud(value *IBMCloudPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.IBMCloud = value
	return b
}

// WithKubevirt sets the Kubevirt field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Kubevirt field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithKubevirt(value *KubevirtPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.Kubevirt = value
	return b
}

// WithEquinixMetal sets the EquinixMetal field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the EquinixMetal field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithEquinixMetal(value *EquinixMetalPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.EquinixMetal = value
	return b
}

// WithPowerVS sets the PowerVS field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the PowerVS field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithPowerVS(value *PowerVSPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.PowerVS = value
	return b
}

// WithAlibabaCloud sets the AlibabaCloud field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the AlibabaCloud field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithAlibabaCloud(value *AlibabaCloudPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.AlibabaCloud = value
	return b
}

// WithNutanix sets the Nutanix field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Nutanix field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithNutanix(value *NutanixPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.Nutanix = value
	return b
}

// WithExternal sets the External field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the External field is set to the value of the last call.
func (b *PlatformStatusApplyConfiguration) WithExternal(value *ExternalPlatformStatusApplyConfiguration) *PlatformStatusApplyConfiguration {
	b.External = value
	return b
}
