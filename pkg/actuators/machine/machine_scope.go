package machine

import (
	"context"
	"fmt"
	"strconv"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	machineapierrors "github.com/openshift/machine-api-operator/pkg/controller/machine"
	clientpkg "github.com/openshift/machine-api-provider-nutanix/pkg/client"

	nutanixClient "github.com/nutanix-cloud-native/prism-go-client/pkg/nutanix"
	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/pkg/nutanix/v3"
	corev1 "k8s.io/api/core/v1"
	apimachineryerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	userDataSecretKey = "userData"
)

// machineScopeParams defines the input parameters used to create a new MachineScope.
type machineScopeParams struct {
	context.Context

	//nutanixClient nutanixClientV3.Client
	// api server controller runtime client
	client runtimeclient.Client
	// machine resource
	machine *machinev1beta1.Machine
	// api server controller runtime client for the openshift-config-managed namespace
	configManagedClient runtimeclient.Client
}

type machineScope struct {
	context.Context

	// client for interacting with Nutanix PC APIs
	nutanixClient *nutanixClientV3.Client
	// api server controller runtime client
	client runtimeclient.Client
	// machine resource
	machine            *machinev1beta1.Machine
	machineToBePatched runtimeclient.Patch
	providerSpec       *machinev1.NutanixMachineProviderConfig
	providerStatus     *machinev1.NutanixMachineProviderStatus
}

func newMachineScope(params machineScopeParams) (*machineScope, error) {
	if params.Context == nil || params.machine == nil {
		return nil, fmt.Errorf("context and machine should not be nil")
	}

	providerSpec, err := NutanixMachineProviderSpecFromRawExtension(params.machine.Spec.ProviderSpec.Value)
	if err != nil {
		return nil, machineapierrors.InvalidMachineConfiguration("failed to get machine provider config: %v", err)
	}

	providerStatus, err := NutanixMachineProviderStatusFromRawExtension(params.machine.Status.ProviderStatus)
	if err != nil {
		return nil, machineapierrors.InvalidMachineConfiguration("failed to get machine provider status: %v", err.Error())
	}

	mscp := &machineScope{
		Context:            params.Context,
		client:             params.client,
		machine:            params.machine,
		machineToBePatched: runtimeclient.MergeFrom(params.machine.DeepCopy()),
		providerSpec:       providerSpec,
		providerStatus:     providerStatus,
	}

	clientOptions, err := mscp.getNutanixClientOptions()
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint and/or credentials to access the Nutanix PC: %w", err)
	}

	nutanixClient, err := clientpkg.Client(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create nutanix client: %w", err)
	}

	mscp.nutanixClient = nutanixClient
	return mscp, nil
}

func (s *machineScope) getNutanixClientOptions() (*clientpkg.ClientOptions, error) {

	clientOptions := &clientpkg.ClientOptions{
		Credentials: &nutanixClient.Credentials{},
		Debug:       true,
	}

	// Get the PC endpoint/port from the Infrastructure CR
	infra := &configv1.Infrastructure{}
	infraKey := runtimeclient.ObjectKey{
		Name: globalInfrastuctureName,
	}
	err := s.client.Get(s.Context, infraKey, infra)
	if err != nil {
		err1 := fmt.Errorf("Could not find the Infrastruture object %q: %w", infraKey.Name, err)
		klog.Errorf("Machine %q: %w", s.machine.Name, err1)
		return nil, err1
	}

	pcEndpoint := infra.Spec.PlatformSpec.Nutanix.PrismCentral.Address
	pcPort := infra.Spec.PlatformSpec.Nutanix.PrismCentral.Port
	if len(pcEndpoint) == 0 {
		return nil, fmt.Errorf("The prismCentralEndpoint field is not set in the Infrastreucture CR")
	}
	clientOptions.Credentials.Endpoint = pcEndpoint

	if pcPort < 1 || pcPort > 65535 {
		return nil, fmt.Errorf("The pcPort field is not set right in the Infrastreucture CR: %d", pcPort)
	}
	clientOptions.Credentials.Port = strconv.Itoa(int(pcPort))

	if s.providerSpec.CredentialsSecret == nil || len(s.providerSpec.CredentialsSecret.Name) == 0 {
		return nil, fmt.Errorf("The nutanix providerSpec credentialsSecret reference is not set.")
	}
	credsSecretName := s.providerSpec.CredentialsSecret.Name
	credsSecret := &corev1.Secret{}
	credsSecretKey := runtimeclient.ObjectKey{
		Namespace: s.machine.Namespace,
		Name:      credsSecretName,
	}
	err = s.client.Get(s.Context, credsSecretKey, credsSecret)
	if err != nil {
		err1 := fmt.Errorf("Could not find the local credentials secret %s: %w", credsSecretKey.Name, err)
		klog.Errorf("Machine %q: %w", s.machine.Name, err1)
		return nil, err1
	}

	if username, ok := credsSecret.Data[clientpkg.NutanixUserKey]; ok {
		clientOptions.Credentials.Username = string(username)
	} else {
		return nil, fmt.Errorf("The PC username is not available from the local secret %s", credsSecret.Name)
	}

	if password, ok := credsSecret.Data[clientpkg.NutanixPasswordKey]; ok {
		clientOptions.Credentials.Password = string(password)
	} else {
		return nil, fmt.Errorf("The PC password is not available from the local secret %s", credsSecret.Name)
	}

	return clientOptions, nil
}

// Patch patches the machine spec and machine status after reconciling.
func (s *machineScope) patchMachine() error {
	klog.V(3).Infof("%s: patching machine", s.machine.GetName())

	providerStatus, err := RawExtensionFromNutanixMachineProviderStatus(s.providerStatus)
	if err != nil {
		return machineapierrors.InvalidMachineConfiguration("failed to get machine provider status: %v", err.Error())
	}
	s.machine.Status.ProviderStatus = providerStatus

	statusCopy := *s.machine.Status.DeepCopy()

	// patch machine
	if err := s.client.Patch(s.Context, s.machine, s.machineToBePatched); err != nil {
		e1 := fmt.Errorf("Failed to patch machine %q: %w", s.machine.GetName(), err)
		klog.Error(e1.Error())
		return e1
	}

	s.machine.Status = statusCopy

	// patch status
	if err := s.client.Status().Patch(context.Background(), s.machine, s.machineToBePatched); err != nil {
		e1 := fmt.Errorf("%s: failed to patch machine status. %w", s.machine.GetName(), err)
		klog.Error(e1.Error())
		return e1
	}

	return nil
}

// getUserData fetches the user-data from the secret referenced in the Machine's
// provider spec, if one is set.
func (s *machineScope) getUserData() ([]byte, error) {
	if s.providerSpec == nil || s.providerSpec.UserDataSecret == nil {
		return nil, nil
	}

	userDataSecret := &corev1.Secret{}

	objKey := runtimeclient.ObjectKey{
		Namespace: s.machine.Namespace,
		Name:      s.providerSpec.UserDataSecret.Name,
	}

	if err := s.client.Get(s.Context, objKey, userDataSecret); err != nil {
		return nil, err
	}

	userData, exists := userDataSecret.Data[userDataSecretKey]
	if !exists {
		return nil, fmt.Errorf("The userData secret %s missing %s key", objKey, userDataSecretKey)
	}

	return userData, nil
}

func (s *machineScope) setProviderStatus(vm *nutanixClientV3.VMIntentResponse, condition metav1.Condition) error {

	klog.Infof("%s: Updating providerStatus", s.machine.Name)

	if vm == nil {
		s.providerStatus.Conditions = setNutanixProviderConditions([]metav1.Condition{condition}, s.providerStatus.Conditions)
		return nil
	}

	// update the Machine providerStatus
	s.providerStatus.VmUUID = vm.Metadata.UUID

	// update machine.status.addresses
	addresses := s.machine.Status.Addresses
	addr := getExistingAddress(addresses, corev1.NodeInternalIP)
	if addr != nil {
		addr.Address = *vm.Status.Resources.NicList[0].IPEndpointList[0].IP
	} else {
		addresses = append(addresses, corev1.NodeAddress{
			Type:    corev1.NodeInternalIP,
			Address: *vm.Status.Resources.NicList[0].IPEndpointList[0].IP,
		})
	}
	addr = getExistingAddress(addresses, corev1.NodeInternalDNS)
	if addr != nil {
		addr.Address = *vm.Spec.Name
	} else {
		addresses = append(addresses, corev1.NodeAddress{
			Type:    corev1.NodeInternalDNS,
			Address: *vm.Spec.Name,
		})
	}
	s.machine.Status.Addresses = addresses

	s.providerStatus.Conditions = setNutanixProviderConditions([]metav1.Condition{condition}, s.providerStatus.Conditions)

	return nil
}

func (s *machineScope) isNodeLinked() bool {
	return s.machine.Status.NodeRef != nil && s.machine.Status.NodeRef.Name != ""
}

func (s *machineScope) getNode() (*corev1.Node, error) {
	var node corev1.Node
	if !s.isNodeLinked() {
		return nil, fmt.Errorf("[Machine: %s] NodeRef empty, unable to get related Node", s.machine.Name)
	}

	nodeName := s.machine.Status.NodeRef.Name
	nodeKey := runtimeclient.ObjectKey{
		Name: nodeName,
	}

	err := s.client.Get(s.Context, nodeKey, &node)
	if err != nil {
		if apimachineryerrors.IsNotFound(err) {
			klog.Infof("%s: Node %q not found", s.machine.Name, nodeName)
			return nil, err
		}
		klog.Errorf("%s: failed to get node %q. %w", s.machine.Name, nodeName, err)
		return nil, err
	}

	return &node, nil
}

// nodeHasVolumesAttached returns true if node status still have volumes attached
// pod deletion and volume detach happen asynchronously, so pod could be deleted before volume detached from the node
// this could cause issue for some storage provisioner, because if the node is deleted before detach success,
// then the underline VMDK will be deleted together with the Machine so after node draining we need to check
// if all volumes are detached before deleting the node.
func (s *machineScope) nodeHasVolumesAttached() (bool, error) {
	node, err := s.getNode()
	if err != nil {
		// do not return error if node object not found, treat it as unreachable
		if apimachineryerrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return len(node.Status.VolumesAttached) != 0, nil
}
