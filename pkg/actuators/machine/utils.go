package machine

import (
	"encoding/json"
	"fmt"

	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/pkg/nutanix/v3"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"
	clientpkg "github.com/openshift/machine-api-provider-nutanix/pkg/client"
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
		return nil, fmt.Errorf("error marshalling providerSpec: %w", err)
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
		return nil, fmt.Errorf("error marshalling providerStatus: %w", err)
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
		return nil, fmt.Errorf("error unmarshalling providerSpec: %w", err)
	}

	klog.V(5).Infof("Got provider Spec from raw extension: %+v", *spec)
	return spec, nil
}

// NutanixMachineProviderStatusFromRawExtension unmarshals a raw extension into an NutanixMachineProviderStatus type
func NutanixMachineProviderStatusFromRawExtension(rawExtension *runtime.RawExtension) (*machinev1.NutanixMachineProviderStatus, error) {
	if rawExtension == nil {
		return &machinev1.NutanixMachineProviderStatus{}, nil
	}

	providerStatus := new(machinev1.NutanixMachineProviderStatus)
	if err := json.Unmarshal(rawExtension.Raw, providerStatus); err != nil {
		return nil, fmt.Errorf("error unmarshalling providerStatus: %w", err)
	}

	klog.V(5).Infof("Got provider Status from raw extension: %+v", providerStatus)
	return providerStatus, nil
}

// The expected category (key/value) for the Nutanix resources (ex. vms, images)
// created for the cluster
const (
	// Expected NutanixCategory Key format: "openshift-<cluster-id>"
	NutanixCategoryKeyPrefix = "kubernetes-io-cluster-"
	NutanixCategoryValue     = "owned"
)

// Add the category for installer clueanup the Machine VM at cluster torn-down time
// if the category exists in PC.
func addCategory(mscp *machineScope, vmMetadata *nutanixClientV3.Metadata) error {
	var err error
	clusterID, ok := getClusterID(mscp.machine)
	if !ok || clusterID == "" {
		err = fmt.Errorf("%s: failed to get the clusterID", mscp.machine.Name)
		klog.Error(err.Error())
		return err
	}

	categoryKey := fmt.Sprintf("%s%s", NutanixCategoryKeyPrefix, clusterID)
	_, err = mscp.nutanixClient.V3.GetCategoryValue(categoryKey, NutanixCategoryValue)
	if err != nil {
		klog.Errorf("%s: failed to find the category with key %q and value %q. error: %v", mscp.machine.Name, categoryKey, NutanixCategoryValue, err)
		return err
	}

	// Add the category to the vm metadata
	if vmMetadata.Categories == nil {
		vmMetadata.Categories = make(map[string]string, 1)
	}
	vmMetadata.Categories[categoryKey] = NutanixCategoryValue

	return nil
}

// Nutanix Credentials
type CredentialType string

const (
	BasicAuthCredentialType CredentialType = "basic_auth"
)

type NutanixCredentials struct {
	Credentials []Credential `json:"credentials"`
}

type Credential struct {
	Type CredentialType        `json:"type"`
	Data *runtime.RawExtension `json:"data"`
}

type BasicAuthCredential struct {
	// The Basic Auth (username, password) for the Prism Central
	PrismCentral PrismCentralBasicAuth `json:"prismCentral"`

	// The Basic Auth (username, password) for the Prism Elements (clusters).
	// Currently only one Prism Element (cluster) is used for each openshift cluster.
	// Later this may spread to multiple Prism Element (cluster).
	PrismElements []PrismElementBasicAuth `json:"prismElements"`
}

type PrismCentralBasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type PrismElementBasicAuth struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// setClientCredentials sets the Prism Central credentials to the clientOptions with the given credentials data
func setClientCredentials(credsData []byte, clientOptions *clientpkg.ClientOptions) error {
	creds := &NutanixCredentials{}
	err := json.Unmarshal(credsData, &creds.Credentials)
	if err != nil {
		err1 := fmt.Errorf("Failed to unmarshal the credentials data. %w", err)
		klog.Errorf(err1.Error())
		return err1
	}

	// Currently we only support the "basic_auth" credentials type.
	for _, cred := range creds.Credentials {
		switch cred.Type {
		case BasicAuthCredentialType:
			basicAuthCreds := BasicAuthCredential{}
			if err := json.Unmarshal(cred.Data.Raw, &basicAuthCreds); err != nil {
				return fmt.Errorf("Failed to unmarshal the basic-auth data. %w", err)
			}
			if basicAuthCreds.PrismCentral.Username == "" || basicAuthCreds.PrismCentral.Password == "" {
				return fmt.Errorf("The PrismCentral credentials data is not set.")
			}

			clientOptions.Credentials.Username = basicAuthCreds.PrismCentral.Username
			clientOptions.Credentials.Password = basicAuthCreds.PrismCentral.Password
			klog.Infof("Successfully set the PrismCentral credentials to the clientOptions to create the Prism Central client.")
			return nil

		default:
			return fmt.Errorf("Unsupported credentials type: %v", cred.Type)
		}
	}

	return fmt.Errorf("The PrismCentral credentials data is not availaible.")
}

// Condition types for an Nutanix VM instance.
const (
	// machineCreation indicates whether the machine's VM has been created or not. If not,
	// it should include a reason and message for the failure.
	machineCreation string = "MachineCreation"

	// machineUpdate indicates whether the machine's VM has been updated or not. If not,
	// it should include a reason and message for the failure.
	machineUpdate string = "MachineUpdate"

	// machineDeletion indicates whether the machine's VM has been deleted or not. If not,
	// it should include a reason and message for the failure.
	machineDeletion string = "MachineDeletion"

	// machineInstanceReady indicates whether the machine's VM gets ready.
	machineInstanceReady string = "MachineInstanceReady"
)

// Condition reasons for the condition's last transition.
const (
	// machineCreationSucceeded indicates machine creation success.
	machineCreationSucceeded string = "MachineCreationSucceeded"
	// machineCreationFailed indicates machine creation failure.
	machineCreationFailed string = "MachineCreationFailed"
	// machineUpdateSucceeded indicates machine update success.
	machineUpdateSucceeded string = "MachineUpdateSucceeded"
	// machineUpdateFailed indicates machine update failure.
	machineUpdateFailed string = "MachineUpdateFailed"
	// machineDeletionSucceeded indicates machine deletion success.
	machineDeletionSucceeded string = "MachineDeletionSucceeded"
	// machineDeletionFailed indicates machine deletion failure.
	machineDeletionFailed string = "MachineDeletionFailed"
	// machineInstanceIsReady indicates the machine's VM gets ready
	machineInstanceIsReady string = "Machine instance is ready"
	// machineInstanceNotReady indicates the machine's VM is not ready
	machineInstanceNotReady string = "Machine instance is not ready"
	// machineInstanceReadyUnknown indicates unknown if the machine's VM is ready
	machineInstanceReadyUnknown string = "Unknown machine instance ready or not"
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
	case machineCreation:
		reason = machineCreationSucceeded
	case machineUpdate:
		reason = machineUpdateSucceeded
	case machineDeletion:
		reason = machineDeletionSucceeded
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
	case machineCreation:
		reason = machineCreationFailed
	case machineUpdate:
		reason = machineUpdateFailed
	case machineDeletion:
		reason = machineDeletionFailed
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
		reason = machineInstanceIsReady
		msg = machineInstanceIsReady
	case metav1.ConditionFalse:
		reason = machineInstanceNotReady
		msg = machineInstanceNotReady
	case metav1.ConditionUnknown:
		reason = machineInstanceReadyUnknown
		msg = machineInstanceReadyUnknown
	}

	return metav1.Condition{
		Type:    machineInstanceReady,
		Status:  status,
		Reason:  reason,
		Message: msg,
	}
}

// validateMachine checks the label that a machine must have to identify the cluster
// to which it belongs is present.
func validateMachine(machine machinev1beta1.Machine) error {
	if machine.Labels[machinev1beta1.MachineClusterIDLabel] == "" {
		return machinecontroller.InvalidMachineConfiguration("%v: missing %q label", machine.GetName(), machinev1beta1.MachineClusterIDLabel)
	}

	return nil
}

// getClusterID get cluster ID by machine.openshift.io/cluster-api-cluster label
func getClusterID(machine *machinev1beta1.Machine) (string, bool) {
	clusterID, ok := machine.Labels[machinev1beta1.MachineClusterIDLabel]
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

// GetMibValueOfQuantity returns the given quantity value in Mib
func GetMibValueOfQuantity(quantity resource.Quantity) int64 {
	return quantity.Value() / (1024 * 1024)
}
