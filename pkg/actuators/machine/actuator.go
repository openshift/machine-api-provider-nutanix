/*
Copyright 2021 Nutanix Inc.

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

package machine

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
)

const (
	scopeFailFmt      = "%s: failed to create scope for machine: %w"
	reconcilerFailFmt = "%s: reconciler failed to %s machine: %w"
	createEventAction = "Create"
	updateEventAction = "Update"
	deleteEventAction = "Delete"
	noEventAction     = ""
)

// Actuator is responsible for performing machine reconciliation.
type Actuator struct {
	client              runtimeclient.Client
	eventRecorder       record.EventRecorder
	configManagedClient runtimeclient.Client
}

// ActuatorParams holds parameter information for Actuator.
type ActuatorParams struct {
	Client              runtimeclient.Client
	EventRecorder       record.EventRecorder
	ConfigManagedClient runtimeclient.Client
}

// NewActuator returns an actuator.
func NewActuator(params ActuatorParams) *Actuator {
	return &Actuator{
		client:              params.Client,
		eventRecorder:       params.EventRecorder,
		configManagedClient: params.ConfigManagedClient,
	}
}

// Set corresponding event based on error. It also returns the original error
// for convenience, so callers can do "return handleMachineError(...)".
func (a *Actuator) handleMachineError(machine *machinev1beta1.Machine, err error, eventAction string) error {
	klog.Errorf("error: %v", err)
	if eventAction != noEventAction {
		a.eventRecorder.Eventf(machine, corev1.EventTypeWarning, "Failed"+eventAction, "%v", err)
	}
	return err
}

// Create creates a machine and is invoked by the machine controller.
func (a *Actuator) Create(ctx context.Context, machine *machinev1beta1.Machine) error {
	klog.Infof("%s: actuator creating machine", machine.GetName())

	scope, err := newMachineScope(machineScopeParams{
		Context:             ctx,
		client:              a.client,
		machine:             machine,
		configManagedClient: a.configManagedClient,
	})
	if err != nil {
		fmtErr := fmt.Errorf(scopeFailFmt, machine.GetName(), err)
		return a.handleMachineError(machine, fmtErr, createEventAction)
	}

	if err := newReconciler(scope).create(); err != nil {
		// Update providerStatus conditions
		cond1 := conditionFailed(machineCreation, err.Error())
		cond2 := conditionInstanceReady(metav1.ConditionFalse)
		scope.providerStatus.Conditions = setNutanixProviderConditions([]metav1.Condition{cond1, cond2}, scope.providerStatus.Conditions)

		if err := scope.patchMachine(); err != nil {
			return err
		}
		fmtErr := fmt.Errorf(reconcilerFailFmt, machine.GetName(), createEventAction, err)
		return a.handleMachineError(machine, fmtErr, createEventAction)
	}

	// Update providerStatus conditions
	cond1 := conditionSuccess(machineCreation)
	cond2 := conditionInstanceReady(metav1.ConditionTrue)
	scope.providerStatus.Conditions = setNutanixProviderConditions([]metav1.Condition{cond1, cond2}, scope.providerStatus.Conditions)

	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, createEventAction, "Created Machine %v", machine.GetName())
	return scope.patchMachine()
}

// Exists determines if the given machine currently exists.
// A machine which is not terminated is considered as existing.
func (a *Actuator) Exists(ctx context.Context, machine *machinev1beta1.Machine) (bool, error) {
	klog.Infof("%s: actuator checking if machine exists", machine.GetName())

	scope, err := newMachineScope(machineScopeParams{
		Context:             ctx,
		client:              a.client,
		machine:             machine,
		configManagedClient: a.configManagedClient,
	})
	if err != nil {
		return false, fmt.Errorf(scopeFailFmt, machine.GetName(), err)
	}
	return newReconciler(scope).exists()
}

// Update attempts to sync machine state with an existing instance.
func (a *Actuator) Update(ctx context.Context, machine *machinev1beta1.Machine) error {
	klog.Infof("%s: actuator updating machine", machine.GetName())

	scope, err := newMachineScope(machineScopeParams{
		Context:             ctx,
		client:              a.client,
		machine:             machine,
		configManagedClient: a.configManagedClient,
	})
	if err != nil {
		fmtErr := fmt.Errorf(scopeFailFmt, machine.GetName(), err)
		return a.handleMachineError(machine, fmtErr, updateEventAction)
	}

	if err := newReconciler(scope).update(); err != nil {
		// Update providerStatus conditions
		cond1 := conditionFailed(machineUpdate, err.Error())
		cond2 := conditionInstanceReady(metav1.ConditionUnknown)
		scope.providerStatus.Conditions = setNutanixProviderConditions([]metav1.Condition{cond1, cond2}, scope.providerStatus.Conditions)

		// Update machine and machine status in case it was modified
		if err := scope.patchMachine(); err != nil {
			return err
		}
		fmtErr := fmt.Errorf(reconcilerFailFmt, machine.GetName(), updateEventAction, err)
		return a.handleMachineError(machine, fmtErr, updateEventAction)
	}

	// Update providerStatus conditions
	cond1 := conditionSuccess(machineUpdate)
	cond2 := conditionInstanceReady(metav1.ConditionTrue)
	scope.providerStatus.Conditions = setNutanixProviderConditions([]metav1.Condition{cond1, cond2}, scope.providerStatus.Conditions)

	previousResourceVersion := scope.machine.ResourceVersion

	if err := scope.patchMachine(); err != nil {
		return err
	}

	currentResourceVersion := scope.machine.ResourceVersion
	if previousResourceVersion != currentResourceVersion {
		klog.Infof("Updated Machine %s: oldResourceVersion=%s, new=%s", machine.GetName(), previousResourceVersion, currentResourceVersion)
	}

	return nil
}

// Delete deletes a machine and updates its finalizer
func (a *Actuator) Delete(ctx context.Context, machine *machinev1beta1.Machine) error {
	klog.Infof("%s: actuator deleting machine", machine.GetName())

	scope, err := newMachineScope(machineScopeParams{
		Context:             ctx,
		client:              a.client,
		machine:             machine,
		configManagedClient: a.configManagedClient,
	})
	if err != nil {
		fmtErr := fmt.Errorf(scopeFailFmt, machine.GetName(), err)
		return a.handleMachineError(machine, fmtErr, deleteEventAction)
	}

	if err := newReconciler(scope).delete(); err != nil {
		// Update providerStatus conditions
		cond1 := conditionFailed(machineDeletion, err.Error())
		cond2 := conditionInstanceReady(metav1.ConditionUnknown)
		scope.providerStatus.Conditions = setNutanixProviderConditions([]metav1.Condition{cond1, cond2}, scope.providerStatus.Conditions)

		if err := scope.patchMachine(); err != nil {
			return err
		}

		fmtErr := fmt.Errorf(reconcilerFailFmt, machine.GetName(), deleteEventAction, err)
		return a.handleMachineError(machine, fmtErr, deleteEventAction)
	}

	// Update providerStatus conditions
	cond1 := conditionSuccess(machineDeletion)
	cond2 := conditionInstanceReady(metav1.ConditionFalse)
	scope.providerStatus.Conditions = setNutanixProviderConditions([]metav1.Condition{cond1, cond2}, scope.providerStatus.Conditions)

	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, deleteEventAction, "Deleted machine %v", machine.GetName())
	return scope.patchMachine()
}
