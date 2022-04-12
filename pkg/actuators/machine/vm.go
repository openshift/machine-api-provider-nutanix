package machine

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/pkg/nutanix/v3"
	"github.com/nutanix-cloud-native/prism-go-client/pkg/utils"
	clientpkg "github.com/openshift/machine-api-provider-nutanix/pkg/client"
)

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
		klog.Infof("To create VM with name %q, and providerSpec: %+v", vmName, *mscp.providerSpec)
		vmInput := nutanixClientV3.VMIntentInput{}
		vmSpec := nutanixClientV3.VM{Name: utils.StringPtr(vmName)}

		// subnet
		var subnetUuidPtr *string
		if mscp.providerSpec.Subnet.UUID != nil {
			subnetUuidPtr = mscp.providerSpec.Subnet.UUID
		} else if mscp.providerSpec.Subnet.Name != nil {
			subnetUuidPtr, err = findSubenetUuidByName(mscp.nutanixClient, *mscp.providerSpec.Subnet.Name)
			if err != nil {
				return nil, err
			}
		}
		vmNic := &nutanixClientV3.VMNic{
			SubnetReference: &nutanixClientV3.Reference{
				Kind: utils.StringPtr("subnet"),
				UUID: subnetUuidPtr,
			}}
		nicList := []*nutanixClientV3.VMNic{vmNic}

		// rhcos image system disk
		var imageUuidPtr *string
		if mscp.providerSpec.Image.UUID != nil {
			imageUuidPtr = mscp.providerSpec.Image.UUID
		} else if mscp.providerSpec.Image.Name != nil {
			imageUuidPtr, err = findImageUuidByName(mscp.nutanixClient, *mscp.providerSpec.Image.Name)
			if err != nil {
				return nil, err
			}

		}
		diskList := []*nutanixClientV3.VMDisk{}
		diskList = append(diskList, &nutanixClientV3.VMDisk{
			DataSourceReference: &nutanixClientV3.Reference{
				Kind: utils.StringPtr("image"),
				UUID: imageUuidPtr,
			},
			DiskSizeMib: utils.Int64Ptr(GetMibValueOfQuantity(mscp.providerSpec.SystemDiskSize)),
		})

		vmMetadata := nutanixClientV3.Metadata{
			Kind:        utils.StringPtr("vm"),
			SpecVersion: utils.Int64Ptr(1),
		}

		// Add the category for installer clueanup the VM at cluster torn-down time
		// if the category exists in PC
		err = addCategory(mscp, &vmMetadata)
		if err != nil {
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
		var clusterRefUuidPtr *string
		if mscp.providerSpec.Cluster.UUID != nil {
			clusterRefUuidPtr = mscp.providerSpec.Cluster.UUID
		} else if mscp.providerSpec.Cluster.Name != nil {
			clusterRefUuidPtr, err = findClusterUuidByName(mscp.nutanixClient, *mscp.providerSpec.Cluster.Name)
			if err != nil {
				return nil, err
			}
		}
		vmSpec.ClusterReference = &nutanixClientV3.Reference{
			Kind: utils.StringPtr("cluster"),
			UUID: clusterRefUuidPtr,
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
		klog.Errorf("Error deleting vm with uuid %s. %w", vmUUID, err)
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

// findSubenetUuidByName retrieves the subnet uuid by the given subnet name
func findSubenetUuidByName(ntnxclient *nutanixClientV3.Client, subnetName string) (*string, error) {
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
