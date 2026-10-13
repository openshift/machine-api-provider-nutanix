package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"slices"
	"strconv"

	ignutil "github.com/coreos/ignition/v2/config/util"
	igntypes "github.com/coreos/ignition/v2/config/v3_2/types"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	machineapierrors "github.com/openshift/machine-api-operator/pkg/controller/machine"
	clientpkg "github.com/openshift/machine-api-provider-nutanix/pkg/client"
	dataurl "github.com/vincent-petithory/dataurl"

	nutanixClient "github.com/nutanix-cloud-native/prism-go-client"
	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/v3"
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
	// If the providerSpec configures the failureDomain reference,
	// we needed to validate that the Machine's providerSpec configuration is
	// consistent with that of the referenced failureDomain, before creating the Machine VM.
	failureDomain *configv1.NutanixFailureDomain
	// For Machine vm create/update use, take a copy of the providerSpec that we can mutate.
	// This must never be written back to the Machine itself.
	providerSpecValidated *machinev1.NutanixMachineProviderConfig
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
	}

	// Get the PC endpoint/port from the Infrastructure CR
	infra := &configv1.Infrastructure{}
	infraKey := runtimeclient.ObjectKey{
		Name: globalInfrastuctureName,
	}
	err := s.client.Get(s.Context, infraKey, infra)
	if err != nil {
		err1 := fmt.Errorf("Could not find the Infrastruture object %q: %w", infraKey.Name, err)
		klog.Errorf("Machine %q: %v", s.machine.Name, err1)
		return nil, err1
	}

	// If the providerSpec configures the failureDomain reference
	if s.providerSpec.FailureDomain != nil && s.providerSpec.FailureDomain.Name != "" {
		fdName := s.providerSpec.FailureDomain.Name
		// The Machine providerSpec has the failureDomain reference configured
		for _, fd := range infra.Spec.PlatformSpec.Nutanix.FailureDomains {
			if fd.Name == fdName {
				s.failureDomain = &fd
				klog.V(4).Infof("Machine %q: use the FailureDomain %q.", s.machine.Name, fd.Name)
				break
			}
		}

		// If the failureDomain name not found in the Infrastructure CR
		if s.failureDomain == nil {
			err := fmt.Errorf("Could not find the failureDomain with name %q configured in the Infrastructure resource.", fdName)
			klog.Errorf("Machine %q: %v", s.machine.Name, err)
			return nil, err
		}
	}

	// The default PC endpoint and credential secret name
	pcEndpoint := infra.Spec.PlatformSpec.Nutanix.PrismCentral.Address
	pcPort := infra.Spec.PlatformSpec.Nutanix.PrismCentral.Port

	if s.providerSpec.CredentialsSecret == nil || len(s.providerSpec.CredentialsSecret.Name) == 0 {
		return nil, fmt.Errorf("The nutanix providerSpec credentialsSecret reference is not set.")
	}
	credsSecretName := s.providerSpec.CredentialsSecret.Name

	if len(pcEndpoint) == 0 {
		return nil, fmt.Errorf("The PC endpoint address is not set correctly (empty) in the Infrastreucture CR")
	}
	clientOptions.Credentials.Endpoint = pcEndpoint

	if pcPort < 1 || pcPort > 65535 {
		return nil, fmt.Errorf("The pcPort field is not set right in the Infrastreucture CR: %d", pcPort)
	}
	clientOptions.Credentials.Port = strconv.Itoa(int(pcPort))

	// Retrieve the PC credentials from the referenced local credentials secret
	credsSecret := &corev1.Secret{}
	credsSecretKey := runtimeclient.ObjectKey{
		Namespace: s.machine.Namespace,
		Name:      credsSecretName,
	}
	err = s.client.Get(s.Context, credsSecretKey, credsSecret)
	if err != nil {
		err1 := fmt.Errorf("Could not find the local credentials secret %q: %w", credsSecretKey.Name, err)
		klog.Errorf("Machine %q: %v", s.machine.Name, err1)
		return nil, err1
	}

	credentialsData, ok := credsSecret.Data["credentials"]
	if !ok {
		return nil, fmt.Errorf("No credentials data found in the local secret %q", credsSecret.Name)
	}

	if err = setClientCredentials(credentialsData, clientOptions); err != nil {
		return nil, fmt.Errorf("Failed to get the credentials data to create the PC client. %w", err)
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

	// WMCO already takes case of setting the hostname to the same as the Machine name. So just return userData.
	if s.machine.Labels["machine.openshift.io/os-id"] == "Windows" {
		klog.V(3).Infof("%s: The windows-user-data: %s", s.machine.Name, string(userData))
		return userData, nil
	}

	// Add the /etc/hostname file with the content of the Machine name, to the ignition userData.
	ignConfig := &igntypes.Config{}
	err := json.Unmarshal(userData, ignConfig)
	if err != nil {
		return nil, fmt.Errorf("Failed to unmarshal userData to IgnitionConfig. %w", err)
	}

	hostnameFile := igntypes.File{
		Node: igntypes.Node{
			Path:      "/etc/hostname",
			Overwrite: ignutil.BoolToPtr(true),
		},
		FileEmbedded1: igntypes.FileEmbedded1{
			Mode: ignutil.IntToPtr(420),
			Contents: igntypes.Resource{
				Source: ignutil.StrToPtr(dataurl.EncodeBytes([]byte(s.machine.Name))),
			},
		},
	}
	if ignConfig.Storage.Files == nil {
		ignConfig.Storage.Files = make([]igntypes.File, 0)
	}
	ignConfig.Storage.Files = append(ignConfig.Storage.Files, hostnameFile)

	ignData, err := json.Marshal(ignConfig)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal ignition data. %w", err)
	}
	klog.V(3).Infof("%s: The ignition data: %s", s.machine.Name, string(ignData))

	return ignData, nil
}

func (s *machineScope) setProviderStatus(vm *nutanixClientV3.VMIntentResponse, condition metav1.Condition) error {

	klog.Infof("%s: Updating providerStatus", s.machine.Name)

	if vm == nil {
		s.providerStatus.Conditions = setNutanixProviderConditions([]metav1.Condition{condition}, s.providerStatus.Conditions)
		return nil
	}

	// update the Machine providerStatus
	s.providerStatus.VmUUID = vm.Metadata.UUID

	// update Addresses
	s.machine.Status.Addresses = s.buildNodeAddresses(vm)

	klog.V(3).Infof("%s: the machine status.addresses=%+v.", s.machine.Name, s.machine.Status.Addresses)

	s.providerStatus.Conditions = setNutanixProviderConditions([]metav1.Condition{condition}, s.providerStatus.Conditions)

	return nil
}

// buildNodeAddresses creates NodeAddreses from VM.
func (s *machineScope) buildNodeAddresses(vm *nutanixClientV3.VMIntentResponse) []corev1.NodeAddress {
	nodeAddresses := []corev1.NodeAddress{}
	ips := []string{}

	for _, nic := range vm.Status.Resources.NicList {
		for _, ipEndpoint := range nic.IPEndpointList {
			if ipEndpoint.IP != nil && *ipEndpoint.IP != "" {
				ip := *ipEndpoint.IP
				if !isLinkLocal(ip) { // Filter out link-local addresses
					ips = append(ips, ip)
				}
			}
		}
	}

	slices.Sort(ips)

	for _, ip := range ips {
		nodeAddresses = append(nodeAddresses, corev1.NodeAddress{
			Type:    corev1.NodeInternalIP,
			Address: ip,
		})
	}

	vmName := *vm.Spec.Name
	nodeAddresses = append(nodeAddresses,
		corev1.NodeAddress{Type: corev1.NodeInternalDNS, Address: vmName},
		corev1.NodeAddress{Type: corev1.NodeHostName, Address: vmName},
	)

	return nodeAddresses
}

// isLinkLocal checks if the address is in the link-local IP range.
func isLinkLocal(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false // Invalid IP address
	}

	linkLocalSubnet := &net.IPNet{
		IP:   net.IPv4(169, 254, 0, 0),
		Mask: net.CIDRMask(16, 32),
	}

	return linkLocalSubnet.Contains(parsedIP)
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
		klog.Errorf("%s: failed to get node %q. error: %v", s.machine.Name, nodeName, err)
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
