// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

import (
	configv1 "github.com/openshift/api/config/v1"
)

// TLSSecurityProfileApplyConfiguration represents a declarative configuration of the TLSSecurityProfile type for use
// with apply.
type TLSSecurityProfileApplyConfiguration struct {
	Type         *configv1.TLSProfileType            `json:"type,omitempty"`
	Old          *configv1.OldTLSProfile             `json:"old,omitempty"`
	Intermediate *configv1.IntermediateTLSProfile    `json:"intermediate,omitempty"`
	Modern       *configv1.ModernTLSProfile          `json:"modern,omitempty"`
	Custom       *CustomTLSProfileApplyConfiguration `json:"custom,omitempty"`
}

// TLSSecurityProfileApplyConfiguration constructs a declarative configuration of the TLSSecurityProfile type for use with
// apply.
func TLSSecurityProfile() *TLSSecurityProfileApplyConfiguration {
	return &TLSSecurityProfileApplyConfiguration{}
}

// WithType sets the Type field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Type field is set to the value of the last call.
func (b *TLSSecurityProfileApplyConfiguration) WithType(value configv1.TLSProfileType) *TLSSecurityProfileApplyConfiguration {
	b.Type = &value
	return b
}

// WithOld sets the Old field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Old field is set to the value of the last call.
func (b *TLSSecurityProfileApplyConfiguration) WithOld(value configv1.OldTLSProfile) *TLSSecurityProfileApplyConfiguration {
	b.Old = &value
	return b
}

// WithIntermediate sets the Intermediate field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Intermediate field is set to the value of the last call.
func (b *TLSSecurityProfileApplyConfiguration) WithIntermediate(value configv1.IntermediateTLSProfile) *TLSSecurityProfileApplyConfiguration {
	b.Intermediate = &value
	return b
}

// WithModern sets the Modern field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Modern field is set to the value of the last call.
func (b *TLSSecurityProfileApplyConfiguration) WithModern(value configv1.ModernTLSProfile) *TLSSecurityProfileApplyConfiguration {
	b.Modern = &value
	return b
}

// WithCustom sets the Custom field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Custom field is set to the value of the last call.
func (b *TLSSecurityProfileApplyConfiguration) WithCustom(value *CustomTLSProfileApplyConfiguration) *TLSSecurityProfileApplyConfiguration {
	b.Custom = value
	return b
}
