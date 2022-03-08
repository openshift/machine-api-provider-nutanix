package machine

import (
	"encoding/json"
	"fmt"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1b1 "github.com/openshift/api/machine/v1beta1"
	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

// upstreamMachineClusterIDLabel is the label that a machine must have to identify the cluster to which it belongs
const (
	upstreamMachineClusterIDLabel = "sigs.k8s.io/cluster-api-cluster"
	globalInfrastuctureName       = "cluster"
	openshiftConfigNamespace      = "openshift-config"
)

// RawExtensionFromNutanixMachineProviderSpec marshals the machine provider spec.
func RawExtensionFromNutanixMachineProviderSpec(spec *machinev1.NutanixMachineProviderConfig) (*runtime.RawExtension, error) {
	if spec == nil {
		return &runtime.RawExtension{}, nil
	}

	var rawBytes []byte
	var err error
	if rawBytes, err = json.Marshal(spec); err != nil {
		return nil, fmt.Errorf("error marshalling providerSpec: %v", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// RawExtensionFromNutanixMachineProviderStatus marshals the machine provider status
func RawExtensionFromNutanixMachineProviderStatus(status *machinev1.NutanixMachineProviderStatus) (*runtime.RawExtension, error) {
	if status == nil {
		return &runtime.RawExtension{}, nil
	}

	var rawBytes []byte
	var err error
	if rawBytes, err = json.Marshal(status); err != nil {
		return nil, fmt.Errorf("error marshalling providerStatus: %v", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// NutanixMachineProviderSpecFromRawExtension unmarshals a raw extension into an NutanixMachineProviderConfig type
func NutanixMachineProviderSpecFromRawExtension(rawExtension *runtime.RawExtension) (*machinev1.NutanixMachineProviderConfig, error) {
	if rawExtension == nil {
		return &machinev1.NutanixMachineProviderConfig{}, nil
	}

	spec := new(machinev1.NutanixMachineProviderConfig)
	if err := json.Unmarshal(rawExtension.Raw, &spec); err != nil {
		return nil, fmt.Errorf("error unmarshalling providerSpec: %v", err)
	}

	klog.V(5).Infof("Got provider Spec from raw extension: %+v", spec)
	return spec, nil
}

// NutanixMachineProviderStatusFromRawExtension unmarshals a raw extension into an NutanixMachineProviderStatus type
func NutanixMachineProviderStatusFromRawExtension(rawExtension *runtime.RawExtension) (*machinev1.NutanixMachineProviderStatus, error) {
	if rawExtension == nil {
		return &machinev1.NutanixMachineProviderStatus{}, nil
	}

	providerStatus := new(machinev1.NutanixMachineProviderStatus)
	if err := json.Unmarshal(rawExtension.Raw, providerStatus); err != nil {
		return nil, fmt.Errorf("error unmarshalling providerStatus: %v", err)
	}

	klog.V(5).Infof("Got provider Status from raw extension: %+v", providerStatus)
	return providerStatus, nil
}

// The expected resource description and category (key/value) for the Nutanix resources (ex. vms, images)
// created for the cluster
const (
	NutanixExpectedResourceDescription = "Created By OpenShift Installer"
	// ExpectedCategoryKey format: "openshift-<cluster-id>"
	NutanixExpectedCategoryKeyPrefix = "openshift-"
	NutanixExpectedCategoryValue     = "openshift-ipi-installations"
)

// NutanixExpectedCategory holds a category key/value for the Nutanix resources (ex. vms, images)
type NutanixExpectedCategory struct {
	Key   string
	Value string
}

func CreateNutanixExpectedCategory(infraID string) *NutanixExpectedCategory {
	return &NutanixExpectedCategory{
		Key:   fmt.Sprintf("%s%s", NutanixExpectedCategoryKeyPrefix, infraID),
		Value: NutanixExpectedCategoryValue,
	}
}

// Condition types for an Nutanix VM instance.
const (
	// MachineCreation indicates whether the machine's VM has been created or not. If not,
	// it should include a reason and message for the failure.
	MachineCreation string = "MachineCreation"

	// MachineUpdate indicates whether the machine's VM has been updated or not. If not,
	// it should include a reason and message for the failure.
	MachineUpdate string = "MachineUpdate"

	// MachineDeletion indicates whether the machine's VM has been deleted or not. If not,
	// it should include a reason and message for the failure.
	MachineDeletion string = "MachineDeletion"

	// MachineInstanceReady indicates whether the machine's VM gets ready.
	MachineInstanceReady string = "MachineInstanceReady"
)

// Condition reasons for the condition's last transition.
const (
	// MachineCreationSucceeded indicates machine creation success.
	MachineCreationSucceeded string = "MachineCreationSucceeded"
	// MachineCreationFailed indicates machine creation failure.
	MachineCreationFailed string = "MachineCreationFailed"
	// MachineUpdateSucceeded indicates machine update success.
	MachineUpdateSucceeded string = "MachineUpdateSucceeded"
	// MachineUpdateFailed indicates machine update failure.
	MachineUpdateFailed string = "MachineUpdateFailed"
	// MachineDeletionSucceeded indicates machine deletion success.
	MachineDeletionSucceeded string = "MachineDeletionSucceeded"
	// MachineDeletionFailed indicates machine deletion failure.
	MachineDeletionFailed string = "MachineDeletionFailed"
	// MachineInstanceIsReady indicates the machine's VM gets ready
	MachineInstanceIsReady string = "Machine instance is ready"
	// MachineInstanceNotReady indicates the machine's VM is not ready
	MachineInstanceNotReady string = "Machine instance is not ready"
	// MachineInstanceReadyUnknown indicates unknown if the machine's VM is ready
	MachineInstanceReadyUnknown string = "Unknown machine instance ready or not"
)

// setNutanixProviderConditions sets the conditions for the machine provider and
// returns the new slice of conditions.
// If the machine does not already have a condition with the specified type,
// a condition will be added to the slice
// If the machine does already have a condition with the specified type,
// the condition will be updated if either of the following are true.
func setNutanixProviderConditions(conditions []metav1.Condition, currConditions []metav1.Condition) []metav1.Condition {

	for _, cond := range conditions {
		existingCond := findProviderCondition(currConditions, cond.Type)
		if existingCond == nil {
			cond.LastTransitionTime = metav1.Now()
			currConditions = append(currConditions, cond)
		} else {
			updateExistingCondition(&cond, existingCond)
		}
	}

	return conditions
}

func findProviderCondition(currConditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range currConditions {
		if currConditions[i].Type == conditionType {
			return &currConditions[i]
		}
	}
	return nil
}

func updateExistingCondition(newCondition, existingCondition *metav1.Condition) {
	if !shouldUpdateCondition(newCondition, existingCondition) {
		return
	}

	if existingCondition.Status != newCondition.Status {
		existingCondition.LastTransitionTime = metav1.Now()
	}
	existingCondition.Status = newCondition.Status
	existingCondition.Reason = newCondition.Reason
	existingCondition.Message = newCondition.Message
}

func shouldUpdateCondition(newCondition, existingCondition *metav1.Condition) bool {
	return newCondition.Status != existingCondition.Status ||
		newCondition.Reason != existingCondition.Reason ||
		newCondition.Message != existingCondition.Message
}

func conditionSuccess(condType string) metav1.Condition {
	var reason string
	switch condType {
	case MachineCreation:
		reason = MachineCreationSucceeded
	case MachineUpdate:
		reason = MachineUpdateSucceeded
	case MachineDeletion:
		reason = MachineDeletionSucceeded
	default:
		reason = condType + " succeeded"
	}

	return metav1.Condition{
		Type:    condType,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: condType + " succeeded",
	}
}

func conditionFailed(condType string, errMsg string) metav1.Condition {
	var reason string
	switch condType {
	case MachineCreation:
		reason = MachineCreationFailed
	case MachineUpdate:
		reason = MachineUpdateFailed
	case MachineDeletion:
		reason = MachineDeletionFailed
	default:
		reason = condType + " failed"
	}

	return metav1.Condition{
		Type:    condType,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: errMsg,
	}
}

func conditionInstanceReady(status metav1.ConditionStatus) metav1.Condition {
	var reason, msg string
	switch status {
	case metav1.ConditionTrue:
		reason = MachineInstanceIsReady
		msg = MachineInstanceIsReady
	case metav1.ConditionFalse:
		reason = MachineInstanceNotReady
		msg = MachineInstanceNotReady
	case metav1.ConditionUnknown:
		reason = MachineInstanceReadyUnknown
		msg = MachineInstanceReadyUnknown
	}

	return metav1.Condition{
		Type:    MachineInstanceReady,
		Status:  status,
		Reason:  reason,
		Message: msg,
	}
}

// validateMachine checks the label that a machine must have to identify the cluster
// to which it belongs is present.
func validateMachine(machine machinev1b1.Machine) error {
	if machine.Labels[machinev1b1.MachineClusterIDLabel] == "" {
		return machinecontroller.InvalidMachineConfiguration("%v: missing %q label", machine.GetName(), machinev1b1.MachineClusterIDLabel)
	}

	return nil
}

// getClusterID get cluster ID by machine.openshift.io/cluster-api-cluster label
func getClusterID(machine *machinev1b1.Machine) (string, bool) {
	clusterID, ok := machine.Labels[machinev1b1.MachineClusterIDLabel]
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

func stringPointerDeref(stringPointer *string) string {
	if stringPointer != nil {
		return *stringPointer
	}
	return ""
}

// StringPtr returns a pointer to the string value passed in.
func stringPointer(str string) *string {
	if len(str) == 0 {
		return nil
	}
	return &str
}

// GetMibValueOfQuality returns the given quantity value in Mib
func GetMibValueOfQuality(quality resource.Quantity) int64 {
	return quality.Value() / (1024 * 1024)
}
