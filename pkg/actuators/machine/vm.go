package machine

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"

	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/pkg/nutanix/v3"
	"github.com/nutanix-cloud-native/prism-go-client/pkg/utils"
	machinev1 "github.com/openshift/api/machine/v1"
	clientpkg "github.com/openshift/machine-api-provider-nutanix/pkg/client"
)

// validateVMConfig verifies if the machine's VM configuration is valid
func validateVMConfig(mscp *machineScope) field.ErrorList {
	errList := field.ErrorList{}
	fldPath := field.NewPath("spec", "providerSpec", "value")
	var errMsg string

	// verify the cluster configuration
	if fldErr := validateClusterConfig(mscp, fldPath); fldErr != nil {
		errList = append(errList, fldErr)
	}

	// verify the image configuration
	if fldErr := validateImageConfig(mscp, fldPath); fldErr != nil {
		errList = append(errList, fldErr)
	}

	// verify the subnets configuration
	// Currently we only allow and support one subnet per VM
	// We may extend to allow and support more than one subnets per VM, in the future release
	if len(mscp.providerSpec.Subnets) == 0 {
		errList = append(errList, field.Required(fldPath.Child("subnets"), "Missing subnets"))

	} else if len(mscp.providerSpec.Subnets) > 1 {
		errMsg = "Currently we only allow and support one subnet per VM, but more than one subnets are configured."
		errList = append(errList, field.Invalid(fldPath.Child("subnets"), len(mscp.providerSpec.Subnets), errMsg))

	} else {
		subnet := &mscp.providerSpec.Subnets[0]
		if fldErr := validateSubnetConfig(mscp, subnet, fldPath); fldErr != nil {
			errList = append(errList, fldErr)
		}
	}

	// verify the vcpusPerSocket configuration
	if mscp.providerSpec.VCPUsPerSocket < 1 {
		errMsg = "The minimum number of vCPUs per socket of the VM is 1."
		errList = append(errList, field.Invalid(fldPath.Child("vcpusPerSocket"), mscp.providerSpec.VCPUsPerSocket, errMsg))
	}

	// verify the vcpuSockets configuration
	if mscp.providerSpec.VCPUSockets < 1 {
		errMsg = "The minimum vCPU sockets of the VM is 1."
		errList = append(errList, field.Invalid(fldPath.Child("vcpuSockets"), mscp.providerSpec.VCPUSockets, errMsg))
	}

	// verify the memorySize configuration
	// The minimum memorySize is 2Gi bytes
	memSizeMib := GetMibValueOfQuantity(mscp.providerSpec.MemorySize)
	if memSizeMib < 2*1024 {
		errList = append(errList, field.Invalid(fldPath.Child("memorySize"), fmt.Sprintf("%vMib", memSizeMib), "The minimum memorySize is 2Gi bytes"))
	}

	// verify the systemDiskSize configuration
	// The minimum systemDiskSize is 20Gi bytes
	diskSizeMib := GetMibValueOfQuantity(mscp.providerSpec.SystemDiskSize)
	if diskSizeMib < 20*1024 {
		errList = append(errList, field.Invalid(fldPath.Child("systemDiskSize"), fmt.Sprintf("%vGib", diskSizeMib/1024), "The minimum systemDiskSize is 20Gi bytes"))
	}

	return errList
}

func validateClusterConfig(mscp *machineScope, fldPath *field.Path) *field.Error {
	var err error
	var errMsg string

	switch mscp.providerSpec.Cluster.Type {
	case machinev1.NutanixIdentifierName:
		if mscp.providerSpec.Cluster.Name == nil || *mscp.providerSpec.Cluster.Name == "" {
			return field.Required(fldPath.Child("cluster", "name"), "Missing cluster name")
		} else {
			clusterName := *mscp.providerSpec.Cluster.Name
			clusterRefUuidPtr, err := findClusterUuidByName(mscp.nutanixClient, clusterName)
			if err != nil {
				errMsg = fmt.Sprintf("Failed to find cluster with name %q. error: %v", clusterName, err)
				return field.Invalid(fldPath.Child("cluster", "name"), clusterName, errMsg)
			} else {
				mscp.providerSpec.Cluster.Type = machinev1.NutanixIdentifierUUID
				mscp.providerSpec.Cluster.UUID = clusterRefUuidPtr
			}
		}
	case machinev1.NutanixIdentifierUUID:
		if mscp.providerSpec.Cluster.UUID == nil || *mscp.providerSpec.Cluster.UUID == "" {
			return field.Required(fldPath.Child("cluster", "uuid"), "Missing cluster uuid")
		} else {
			clusterUUID := *mscp.providerSpec.Cluster.UUID
			if _, err = mscp.nutanixClient.V3.GetCluster(clusterUUID); err != nil {
				errMsg = fmt.Sprintf("Failed to find cluster with uuid %v. error: %v", clusterUUID, err)
				return field.Invalid(fldPath.Child("cluster", "uuid"), clusterUUID, errMsg)
			}
		}
	default:
		errMsg = fmt.Sprintf("Invalid cluster identifier type, valid types are: %q, %q.", machinev1.NutanixIdentifierName, machinev1.NutanixIdentifierUUID)
		return field.Invalid(fldPath.Child("cluster", "type"), mscp.providerSpec.Cluster.Type, errMsg)
	}

	return nil
}

func validateImageConfig(mscp *machineScope, fldPath *field.Path) *field.Error {
	var err error
	var errMsg string

	switch mscp.providerSpec.Image.Type {
	case machinev1.NutanixIdentifierName:
		if mscp.providerSpec.Image.Name == nil || *mscp.providerSpec.Image.Name == "" {
			return field.Required(fldPath.Child("image", "name"), "Missing image name")
		} else {
			imageName := *mscp.providerSpec.Image.Name
			imageRefUuidPtr, err := findImageUuidByName(mscp.nutanixClient, imageName)
			if err != nil {
				errMsg = fmt.Sprintf("Failed to find image with name %q. error: %v", imageName, err)
				return field.Invalid(fldPath.Child("image", "name"), imageName, errMsg)
			} else {
				mscp.providerSpec.Image.Type = machinev1.NutanixIdentifierUUID
				mscp.providerSpec.Image.UUID = imageRefUuidPtr
			}
		}
	case machinev1.NutanixIdentifierUUID:
		if mscp.providerSpec.Image.UUID == nil || *mscp.providerSpec.Image.UUID == "" {
			return field.Required(fldPath.Child("image", "uuid"), "Missing image uuid")
		} else {
			imageUUID := *mscp.providerSpec.Image.UUID
			if _, err = mscp.nutanixClient.V3.GetImage(imageUUID); err != nil {
				errMsg = fmt.Sprintf("Failed to find image with uuid %v. error: %v", imageUUID, err)
				return field.Invalid(fldPath.Child("image", "uuid"), imageUUID, errMsg)
			}
		}
	default:
		errMsg = fmt.Sprintf("Invalid image identifier type, valid types are: %q, %q.", machinev1.NutanixIdentifierName, machinev1.NutanixIdentifierUUID)
		return field.Invalid(fldPath.Child("image", "type"), mscp.providerSpec.Image.Type, errMsg)
	}

	return nil
}

func validateSubnetConfig(mscp *machineScope, subnet *machinev1.NutanixResourceIdentifier, fldPath *field.Path) *field.Error {
	var err error
	var errMsg string

	switch subnet.Type {
	case machinev1.NutanixIdentifierName:
		if subnet.Name == nil || *subnet.Name == "" {
			return field.Required(fldPath.Child("subnet", "name"), "Missing subnet name")
		} else {
			subnetRefUuidPtr, err := findSubnetUuidByName(mscp.nutanixClient, *subnet.Name)
			if err != nil {
				errMsg = fmt.Sprintf("Failed to find subnet with name %q. error: %v", *subnet.Name, err)
				return field.Invalid(fldPath.Child("subnet", "name"), *subnet.Name, errMsg)
			} else {
				subnet.Type = machinev1.NutanixIdentifierUUID
				subnet.UUID = subnetRefUuidPtr
			}
		}
	case machinev1.NutanixIdentifierUUID:
		if subnet.UUID == nil || *subnet.UUID == "" {
			return field.Required(fldPath.Child("subnet").Child("uuid"), "Missing subnet uuid")
		} else {
			if _, err = mscp.nutanixClient.V3.GetSubnet(*subnet.UUID); err != nil {
				errMsg = fmt.Sprintf("Failed to find subnet with uuid %v. error: %v", *subnet.UUID, err)
				return field.Invalid(fldPath.Child("subnet", "uuid"), *subnet.UUID, errMsg)
			}
		}
	default:
		errMsg = fmt.Sprintf("Invalid subnet identifier type, valid types are: %q, %q.", machinev1.NutanixIdentifierName, machinev1.NutanixIdentifierUUID)
		return field.Invalid(fldPath.Child("subnet", "type"), subnet.Type, errMsg)
	}

	return nil
}

// CreateVM creates a VM and is invoked by the NutanixMachineReconciler
func createVM(mscp *machineScope, userData []byte) (*nutanixClientV3.VMIntentResponse, error) {

	var err error
	var vm *nutanixClientV3.VMIntentResponse
	var vmUuid string
	vmName := mscp.machine.Name

	// Check if the VM already exists
	if mscp.providerStatus.VmUUID != nil {
		// Try to find the vm by uuid
		vm, err = findVMByUUID(mscp.nutanixClient, *mscp.providerStatus.VmUUID)
		if err == nil {
			vmUuid = *vm.Metadata.UUID
			klog.Infof("%s: The VM with UUID %s already exists. No need to create one.", vmName, vmUuid)
		}
	}

	if len(vmUuid) == 0 {
		// Encode the userData by base64
		userdataEncoded := base64.StdEncoding.EncodeToString(userData)

		// Create the VM
		klog.V(5).Infof("To create VM with name %q, and providerSpec: %+v", vmName, *mscp.providerSpec)
		vmInput := nutanixClientV3.VMIntentInput{}
		vmSpec := nutanixClientV3.VM{Name: utils.StringPtr(vmName)}

		nicList := []*nutanixClientV3.VMNic{}
		for _, subnet := range mscp.providerSpec.Subnets {
			vmNic := &nutanixClientV3.VMNic{
				SubnetReference: &nutanixClientV3.Reference{
					Kind: utils.StringPtr("subnet"),
					UUID: subnet.UUID,
				}}
			nicList = append(nicList, vmNic)
		}

		// rhcos image system disk
		diskList := []*nutanixClientV3.VMDisk{}
		diskList = append(diskList, &nutanixClientV3.VMDisk{
			DataSourceReference: &nutanixClientV3.Reference{
				Kind: utils.StringPtr("image"),
				UUID: mscp.providerSpec.Image.UUID,
			},
			DiskSizeMib: utils.Int64Ptr(GetMibValueOfQuantity(mscp.providerSpec.SystemDiskSize)),
		})

		vmMetadata := nutanixClientV3.Metadata{
			Kind:        utils.StringPtr("vm"),
			SpecVersion: utils.Int64Ptr(1),
		}

		// Add the category for installer clueanup the VM at cluster torn-down time
		// if the category exists in PC
		if err := addCategory(mscp, &vmMetadata); err != nil {
			klog.Warningf("Failed to add category to the vm %q. %v", vmName, err)
		} else {
			klog.Infof("%s: added the category to the vm %q: %v", mscp.machine.Name, vmName, vmMetadata.Categories)
		}

		vmSpec.Resources = &nutanixClientV3.VMResources{
			PowerState:            utils.StringPtr("ON"),
			HardwareClockTimezone: utils.StringPtr("UTC"),
			NumVcpusPerSocket:     utils.Int64Ptr(int64(mscp.providerSpec.VCPUsPerSocket)),
			NumSockets:            utils.Int64Ptr(int64(mscp.providerSpec.VCPUSockets)),
			MemorySizeMib:         utils.Int64Ptr(GetMibValueOfQuantity(mscp.providerSpec.MemorySize)),
			NicList:               nicList,
			DiskList:              diskList,
			GuestCustomization: &nutanixClientV3.GuestCustomization{
				IsOverridable: utils.BoolPtr(true),
				CloudInit:     &nutanixClientV3.GuestCustomizationCloudInit{UserData: utils.StringPtr(userdataEncoded)}},
		}

		// Set cluster/PE reference
		vmSpec.ClusterReference = &nutanixClientV3.Reference{
			Kind: utils.StringPtr("cluster"),
			UUID: mscp.providerSpec.Cluster.UUID,
		}

		vmInput.Spec = &vmSpec
		vmInput.Metadata = &vmMetadata
		vm, err = mscp.nutanixClient.V3.CreateVM(&vmInput)
		if err != nil {
			klog.Errorf("Failed to create VM %q. error: %v", vmName, err)
			return nil, err
		}
		vmUuid = *vm.Metadata.UUID
		klog.Infof("Sent the post request to create VM %q. Got the vm UUID: %s, status.state: %s",
			vmName, vmUuid, *vm.Status.State)
		// Wait for some time for the VM getting ready
		time.Sleep(10 * time.Second)
	}

	//Let's wait to vm's state to become "COMPLETE"
	err = clientpkg.WaitForGetVMComplete(mscp.nutanixClient, vmUuid)
	if err != nil {
		klog.Errorf("Failed to get the vm with UUID %s. error: %v", vmUuid, err)
		return nil, fmt.Errorf("Error retriving the created vm %q", vmName)
	}

	vm, err = findVMByUUID(mscp.nutanixClient, vmUuid)
	for err != nil {
		klog.Errorf("Failed to find the vm with UUID %s. %v", vmUuid, err)
		return nil, err
	}
	klog.Infof("The vm %q is ready. vmUUID: %s, vmState: %s", vmName, vmUuid, *vm.Status.State)

	return vm, nil
}

// findVMByUUID retrieves the VM with the given vm UUID
func findVMByUUID(ntnxclient *nutanixClientV3.Client, uuid string) (*nutanixClientV3.VMIntentResponse, error) {
	klog.Infof("Checking if VM with UUID %s exists.", uuid)

	response, err := ntnxclient.V3.GetVM(uuid)
	if err != nil {
		klog.Errorf("Failed to find VM by vmUUID %s. error: %v", uuid, err)
		return nil, err
	}

	return response, nil
}

// findVMByName retrieves the VM with the given vm name
func findVMByName(ntnxclient *nutanixClientV3.Client, vmName string) (*nutanixClientV3.VMIntentResponse, error) {
	klog.Infof("Checking if VM with name %q exists.", vmName)

	res, err := ntnxclient.V3.ListVM(&nutanixClientV3.DSMetadata{
		Filter: utils.StringPtr(fmt.Sprintf("vm_name==%s", vmName))})

	if err != nil {
		err = fmt.Errorf("Error when finding VM by name %s. error: %w", vmName, err)
		klog.Errorf(err.Error())
		return nil, err
	}
	if len(res.Entities) == 0 {
		err = fmt.Errorf("Not Found VM by name %q. error: VM_NOT_FOUND", vmName)
		klog.Errorf(err.Error())
		return nil, err
	}
	if len(res.Entities) > 1 {
		err = fmt.Errorf("Found more than one (%v) vms with name %s.", len(res.Entities), vmName)
		klog.Errorf(err.Error())
		return nil, err
	}

	vm := res.Entities[0]
	vmResp := &nutanixClientV3.VMIntentResponse{
		APIVersion: vm.APIVersion,
		Metadata:   vm.Metadata,
		Spec:       vm.Spec,
		Status:     vm.Status,
	}
	return vmResp, nil
}

// deleteVM deletes a VM with the specified UUID
func deleteVM(ntnxclient *nutanixClientV3.Client, vmUUID string) error {
	klog.Infof("Deleting VM with UUID %s.", vmUUID)

	_, err := ntnxclient.V3.DeleteVM(vmUUID)
	if err != nil {
		klog.Errorf("Error deleting vm with uuid %s. error: %v", vmUUID, err)
		return err
	}

	err = clientpkg.WaitForGetVMDelete(ntnxclient, vmUUID)
	if err != nil {
		if strings.Contains(err.Error(), "Not Found") {
			klog.Infof("Successfully deleted vm with uuid %s", vmUUID)
			return nil
		}

		return err
	}

	return nil
}

// findClusterUuidByName retrieves the cluster uuid by the given cluster name
func findClusterUuidByName(ntnxclient *nutanixClientV3.Client, clusterName string) (*string, error) {
	klog.Infof("Checking if cluster with name %q exists.", clusterName)

	res, err := ntnxclient.V3.ListCluster(&nutanixClientV3.DSMetadata{
		//Kind: utils.StringPtr("cluster"),
		Filter: utils.StringPtr(fmt.Sprintf("name==%s", clusterName)),
	})
	if err != nil || len(res.Entities) == 0 {
		e1 := fmt.Errorf("Failed to find cluster by name %q. error: %w", clusterName, err)
		klog.Errorf(e1.Error())
		return nil, e1
	}

	if len(res.Entities) > 1 {
		klog.Warningf("Found more than one (%v) clusters with name %q.", len(res.Entities), clusterName)
	}

	return res.Entities[0].Metadata.UUID, nil
}

// findImageByName retrieves the image uuid by the given image name
func findImageUuidByName(ntnxclient *nutanixClientV3.Client, imageName string) (*string, error) {
	klog.Infof("Checking if image with name %q exists.", imageName)

	res, err := ntnxclient.V3.ListImage(&nutanixClientV3.DSMetadata{
		//Kind: utils.StringPtr("image"),
		Filter: utils.StringPtr(fmt.Sprintf("name==%s", imageName)),
	})
	if err != nil || len(res.Entities) == 0 {
		e1 := fmt.Errorf("Failed to find image by name %q. error: %w", imageName, err)
		klog.Errorf(e1.Error())
		return nil, e1
	}

	if len(res.Entities) > 1 {
		klog.Warningf("Found more than one (%v) images with name %q.", len(res.Entities), imageName)
	}

	return res.Entities[0].Metadata.UUID, nil
}

// findSubnetUuidByName retrieves the subnet uuid by the given subnet name
func findSubnetUuidByName(ntnxclient *nutanixClientV3.Client, subnetName string) (*string, error) {
	klog.Infof("Checking if subnet with name %q exists.", subnetName)

	res, err := ntnxclient.V3.ListSubnet(&nutanixClientV3.DSMetadata{
		//Kind: utils.StringPtr("subnet"),
		Filter: utils.StringPtr(fmt.Sprintf("name==%s", subnetName)),
	})
	if err != nil || len(res.Entities) == 0 {
		e1 := fmt.Errorf("Failed to find subnet by name %q. error: %w", subnetName, err)
		klog.Errorf(e1.Error())
		return nil, e1
	}

	if len(res.Entities) > 1 {
		klog.Warningf("Found more than one (%v) subnets with name %q.", len(res.Entities), subnetName)
	}

	return res.Entities[0].Metadata.UUID, nil
}

func getPrismCentralCluster(ntnxclient *nutanixClientV3.Client) (*nutanixClientV3.ClusterIntentResponse, error) {
	const filter = ""
	clusterList, err := ntnxclient.V3.ListAllCluster(filter)
	if err != nil {
		return nil, err
	}

	foundPCs := make([]*nutanixClientV3.ClusterIntentResponse, 0)
	for _, cl := range clusterList.Entities {
		if cl.Status != nil && cl.Status.Resources != nil && cl.Status.Resources.Config != nil {
			serviceList := cl.Status.Resources.Config.ServiceList
			for _, svc := range serviceList {
				if svc != nil && strings.ToUpper(*svc) == "PRISM_CENTRAL" {
					foundPCs = append(foundPCs, cl)
				}
			}
		}
	}
	numFoundPCs := len(foundPCs)
	if numFoundPCs == 1 {
		return foundPCs[0], nil
	}
	if len(foundPCs) == 0 {
		return nil, fmt.Errorf("failed to retrieve Prism Central cluster")
	}
	return nil, fmt.Errorf("found more than one Prism Central cluster: %v", numFoundPCs)
}
