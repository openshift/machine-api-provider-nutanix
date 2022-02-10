package machine

import (
	machinev1 "github.com/openshift/api/machine/v1beta1"
	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nutanixv1 "github.com/nutanix-cloud-native/machine-api-provider-nutanix/pkg/apis/nutanixprovider/v1beta1"
)

// upstreamMachineClusterIDLabel is the label that a machine must have to identify the cluster to which it belongs
const (
	upstreamMachineClusterIDLabel = "sigs.k8s.io/cluster-api-cluster"
	globalInfrastuctureName       = "cluster"
	openshiftConfigNamespace      = "openshift-config"
)

// setNutanixProviderCondition sets the condition for the machine and
// returns the new slice of conditions.
// If the machine does not already have a condition with the specified type,
// a condition will be added to the slice
// If the machine does already have a condition with the specified type,
// the condition will be updated if either of the following are true.
func setNutanixProviderCondition(condition nutanixv1.NutanixMachineProviderCondition,
	conditions []nutanixv1.NutanixMachineProviderCondition) []nutanixv1.NutanixMachineProviderCondition {

	now := metav1.Now()

	existingCondition := findProviderCondition(conditions, condition.Type)
	if existingCondition == nil {
		condition.LastProbeTime = now
		condition.LastTransitionTime = now
		conditions = append(conditions, condition)
	} else {
		updateExistingCondition(&condition, existingCondition)
	}

	return conditions
}

func findProviderCondition(conditions []nutanixv1.NutanixMachineProviderCondition, conditionType nutanixv1.NutanixMachineProviderConditionType) *nutanixv1.NutanixMachineProviderCondition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func updateExistingCondition(newCondition, existingCondition *nutanixv1.NutanixMachineProviderCondition) {
	if !shouldUpdateCondition(newCondition, existingCondition) {
		return
	}

	if existingCondition.Status != newCondition.Status {
		existingCondition.LastTransitionTime = metav1.Now()
	}
	existingCondition.Status = newCondition.Status
	existingCondition.Reason = newCondition.Reason
	existingCondition.Message = newCondition.Message
	existingCondition.LastProbeTime = newCondition.LastProbeTime
}

func shouldUpdateCondition(newCondition, existingCondition *nutanixv1.NutanixMachineProviderCondition) bool {
	return newCondition.Status != existingCondition.Status ||
		newCondition.Reason != existingCondition.Reason ||
		newCondition.Message != existingCondition.Message
}

func conditionSuccess(condType nutanixv1.NutanixMachineProviderConditionType) nutanixv1.NutanixMachineProviderCondition {
	var reason nutanixv1.NutanixMachineProviderConditionReason
	switch condType {
	case nutanixv1.MachineCreation:
		reason = nutanixv1.MachineCreationSucceeded
	case nutanixv1.MachineUpdate:
		reason = nutanixv1.MachineUpdateSucceeded
	case nutanixv1.MachineDeletion:
		reason = nutanixv1.MachineDeletionSucceeded
	}

	return nutanixv1.NutanixMachineProviderCondition{
		Type:    condType,
		Status:  corev1.ConditionTrue,
		Reason:  reason,
		Message: string(condType) + " succeeded",
	}
}

func conditionFailed(condType nutanixv1.NutanixMachineProviderConditionType, errMsg string) nutanixv1.NutanixMachineProviderCondition {
	var reason nutanixv1.NutanixMachineProviderConditionReason
	switch condType {
	case nutanixv1.MachineCreation:
		reason = nutanixv1.MachineCreationFailed
	case nutanixv1.MachineUpdate:
		reason = nutanixv1.MachineUpdateFailed
	case nutanixv1.MachineDeletion:
		reason = nutanixv1.MachineDeletionFailed
	}

	return nutanixv1.NutanixMachineProviderCondition{
		Type:    condType,
		Status:  corev1.ConditionFalse,
		Reason:  reason,
		Message: errMsg,
	}
}

// validateMachine check the label that a machine must have to identify the cluster to which it belongs is present.
func validateMachine(machine machinev1.Machine) error {
	if machine.Labels[machinev1.MachineClusterIDLabel] == "" {
		return machinecontroller.InvalidMachineConfiguration("%v: missing %q label", machine.GetName(), machinev1.MachineClusterIDLabel)
	}

	return nil
}

// getClusterID get cluster ID by machine.openshift.io/cluster-api-cluster label
func getClusterID(machine *machinev1.Machine) (string, bool) {
	clusterID, ok := machine.Labels[machinev1.MachineClusterIDLabel]
	if !ok {
		clusterID, ok = machine.Labels[upstreamMachineClusterIDLabel]
	}
	return clusterID, ok
}

func getExistingAddress(addresses []corev1.NodeAddress, addrType corev1.NodeAddressType) *corev1.NodeAddress {
	for _, addr := range addresses {
		if addr.Type == addrType {
			return &addr
		}
	}

	return nil
}
