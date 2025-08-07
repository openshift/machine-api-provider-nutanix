package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nutanix-cloud-native/prism-go-client/utils"
	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/v3"
	configv1 "github.com/openshift/api/config/v1"
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

	fiveMinutes = 5 * time.Minute  // 5m0s
	tenSeconds  = 10 * time.Second // 10s
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
	// Expected NutanixCategory Key format: "kubernetes-io-cluster-<cluster-id>"
	NutanixCategoryKeyPrefix = "kubernetes-io-cluster-"
	NutanixCategoryValue     = "owned"
)

// addCategories adds the category for installer clueanup the Machine VM
// at cluster torn-down time if the category exists in PC.
// Add the addtional categories to the Machine VM if configured
func addCategories(mscp *machineScope, vmMetadata *nutanixClientV3.Metadata) error {
	var err error
	if vmMetadata.Categories == nil {
		vmMetadata.Categories = make(map[string]string, len(mscp.providerSpecValidated.Categories)+1)
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 60*time.Second)
	defer cancel()

	// The addtional categories are already verified
	for _, category := range mscp.providerSpecValidated.Categories {
		vmMetadata.Categories[category.Key] = category.Value
	}

	clusterID, ok := getClusterID(mscp.machine)
	if !ok || clusterID == "" {
		err = fmt.Errorf("%s: failed to get the clusterID", mscp.machine.Name)
		klog.Error(err.Error())
		return err
	}

	categoryKey := fmt.Sprintf("%s%s", NutanixCategoryKeyPrefix, clusterID)
	_, err = mscp.nutanixClient.V3.GetCategoryValue(ctx, categoryKey, NutanixCategoryValue)
	if err != nil {
		klog.Errorf("%s: failed to find the category with key %q and value %q. error: %v", mscp.machine.Name, categoryKey, NutanixCategoryValue, err)
		return err
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
		klog.Errorf("%s", err1.Error())
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

// GetMibValueOfQuantity returns the given quantity value in Mib
func GetMibValueOfQuantity(quantity resource.Quantity) int64 {
	return quantity.Value() / (1024 * 1024)
}

// Ptr takes a pointer to the passed object.
func Ptr[T any](v T) *T {
	return &v
}

// getGPUList returns a list of VMGpus for the given list of GPU identifiers in the Prism Element (uuid)
func getGPUList(ctx context.Context, client *nutanixClientV3.Client, gpus []machinev1.NutanixGPU, peUUID string) ([]*nutanixClientV3.VMGpu, error) {
	vmGPUs := make([]*nutanixClientV3.VMGpu, 0)

	if len(gpus) == 0 {
		return vmGPUs, nil
	}

	peGPUs, err := getGPUsForPE(ctx, client, peUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve GPUs from the Prism Element cluster with UUID %s: %w", peUUID, err)
	}
	if len(peGPUs) == 0 {
		return nil, fmt.Errorf("no available GPUs found in Prism Element cluster with UUID %s", peUUID)
	}

	for _, gpu := range gpus {
		foundGPU, err := getGPUFromList(gpu, peGPUs)
		if err != nil {
			return nil, fmt.Errorf("not found GPU matching the required inputs: %w", err)
		}
		vmGPUs = append(vmGPUs, foundGPU)
	}

	return vmGPUs, nil
}

// getGPUFromList returns the VMGpu matching the input reqirements from the provided list of GPU devices
func getGPUFromList(gpu machinev1.NutanixGPU, gpuDevices []*nutanixClientV3.GPU) (*nutanixClientV3.VMGpu, error) {
	for _, gd := range gpuDevices {
		if gd.Status != "UNUSED" {
			continue
		}

		if (gpu.Type == machinev1.NutanixGPUIdentifierDeviceID && gd.DeviceID != nil && *gpu.DeviceID == int32(*gd.DeviceID)) ||
			(gpu.Type == machinev1.NutanixGPUIdentifierName && *gpu.Name == gd.Name) {
			return &nutanixClientV3.VMGpu{
				DeviceID: gd.DeviceID,
				Mode:     &gd.Mode,
				Vendor:   &gd.Vendor,
			}, nil
		}
	}

	return nil, fmt.Errorf("no available GPU found that matches required GPU inputs")
}

// getGPUsForPE returns all the GPU devices for the given Prism Element (uuid)
func getGPUsForPE(ctx context.Context, client *nutanixClientV3.Client, peUUID string) ([]*nutanixClientV3.GPU, error) {
	gpus := make([]*nutanixClientV3.GPU, 0)
	hosts, err := client.V3.ListAllHost(ctx)
	if err != nil {
		return gpus, err
	}

	for _, host := range hosts.Entities {
		if host == nil ||
			host.Status == nil ||
			host.Status.ClusterReference == nil ||
			host.Status.ClusterReference.UUID != peUUID ||
			host.Status.Resources == nil ||
			len(host.Status.Resources.GPUList) == 0 {
			continue
		}

		for _, peGpu := range host.Status.Resources.GPUList {
			if peGpu == nil {
				continue
			}
			gpus = append(gpus, peGpu)
		}
	}

	return gpus, nil
}

// getMachineResourceIdentifierFromFailureDomainConfig returns NutanixResourceIdentifier for machine's providerSpec from the corresponding configuration in the FailureDomain
func getMachineResourceIdentifierFromFailureDomainConfig(nri configv1.NutanixResourceIdentifier) machinev1.NutanixResourceIdentifier {
	ret := machinev1.NutanixResourceIdentifier{}
	switch nri.Type {
	case configv1.NutanixIdentifierUUID:
		ret.Type = machinev1.NutanixIdentifierUUID
	case configv1.NutanixIdentifierName:
		ret.Type = machinev1.NutanixIdentifierName
	}
	ret.UUID = nri.UUID
	ret.Name = nri.Name

	return ret
}

// waitForTask waits until a queued task has been finished or timeout has been reached.
func waitForTask(clientV3 nutanixClientV3.Service, taskUUID string) error {
	finished := false
	var err error
	for start := time.Now(); time.Since(start) < fiveMinutes; {
		finished, err = isTaskFinished(clientV3, taskUUID)
		if err != nil {
			return err
		}
		if finished {
			break
		}
		time.Sleep(tenSeconds)
	}

	if !finished {
		return fmt.Errorf("timeout while waiting for task UUID: %s", taskUUID)
	}

	return nil
}

func isTaskFinished(clientV3 nutanixClientV3.Service, taskUUID string) (bool, error) {
	isFinished := map[string]bool{
		"QUEUED":    false,
		"RUNNING":   false,
		"SUCCEEDED": true,
	}
	status, err := getTaskStatus(clientV3, taskUUID)
	if err != nil {
		return false, err
	}

	if val, ok := isFinished[status]; ok {
		return val, nil
	}

	return false, fmt.Errorf("retrieved unexpected task status: %s", status)
}

func getTaskStatus(clientV3 nutanixClientV3.Service, taskUUID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 60*time.Second)
	defer cancel()

	v, err := clientV3.GetTask(ctx, taskUUID)
	if err != nil {
		return "", err
	}

	if *v.Status == "INVALID_UUID" || *v.Status == "FAILED" {
		return *v.Status, fmt.Errorf("error_detail: %s, progress_message: %s", utils.StringValue(v.ErrorDetail), utils.StringValue(v.ProgressMessage))
	}
	return *v.Status, nil
}
