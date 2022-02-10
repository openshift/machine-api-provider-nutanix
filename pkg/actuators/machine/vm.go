package machine

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	clientpkg "github.com/nutanix-cloud-native/machine-api-provider-nutanix/pkg/client"
	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/pkg/nutanix/v3"
	"github.com/nutanix-cloud-native/prism-go-client/pkg/utils"
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
		klog.Infof("To create VM with name %s, and providerSpec: %+v", vmName, *mscp.providerSpec)
		vmInput := nutanixClientV3.VMIntentInput{}
		vmSpec := nutanixClientV3.VM{Name: utils.StringPtr(vmName)}
		vmNic := &nutanixClientV3.VMNic{
			SubnetReference: &nutanixClientV3.Reference{
				UUID: &mscp.providerSpec.SubnetUUID,
				Kind: utils.StringPtr("subnet"),
			}}
		nicList := []*nutanixClientV3.VMNic{vmNic}

		var imageUuidPtr *string
		if len(mscp.providerSpec.ImageUUID) > 0 {
			imageUuidPtr = utils.StringPtr(mscp.providerSpec.ImageUUID)
		} else if len(mscp.providerSpec.ImageName) > 0 {
			imageUuidPtr, err = findImageUuidByName(mscp.nutanixClient, mscp.providerSpec.ImageName)
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
			DiskSizeMib: utils.Int64Ptr(mscp.providerSpec.DiskSizeMib),
		})
		vmMetadata := nutanixClientV3.Metadata{
			Kind:        utils.StringPtr("vm"),
			SpecVersion: utils.Int64Ptr(1),
		}
		vmSpec.Resources = &nutanixClientV3.VMResources{
			PowerState:            utils.StringPtr(mscp.providerSpec.PowerState),
			HardwareClockTimezone: utils.StringPtr("UTC"),
			NumVcpusPerSocket:     utils.Int64Ptr(mscp.providerSpec.NumVcpusPerSocket),
			NumSockets:            utils.Int64Ptr(mscp.providerSpec.NumSockets),
			MemorySizeMib:         utils.Int64Ptr(mscp.providerSpec.MemorySizeMib),
			NicList:               nicList,
			DiskList:              diskList,
			GuestCustomization: &nutanixClientV3.GuestCustomization{
				IsOverridable: utils.BoolPtr(true),
				CloudInit:     &nutanixClientV3.GuestCustomizationCloudInit{UserData: utils.StringPtr(userdataEncoded)}},
		}
		vmSpec.ClusterReference = &nutanixClientV3.Reference{
			Kind: utils.StringPtr("cluster"),
			UUID: utils.StringPtr(mscp.providerSpec.ClusterReferenceUUID),
		}
		vmInput.Spec = &vmSpec
		vmInput.Metadata = &vmMetadata

		vm, err = mscp.nutanixClient.V3.CreateVM(&vmInput)
		if err != nil {
			klog.Errorf("Failed to create VM %s. error: %v", vmName, err)
			return nil, err
		}
		vmUuid = *vm.Metadata.UUID
		klog.Infof("Sent the post request to create VM %s. Got the vm UUID: %s, status.state: %s",
			vmName, vmUuid, *vm.Status.State)
		// Wait for some time for the VM getting ready
		time.Sleep(10 * time.Second)
	}

	//Let's wait to vm's state to become "COMPLETE"
	err = clientpkg.WaitForGetVMComplete(mscp.nutanixClient, vmUuid)
	if err != nil {
		klog.Errorf("Failed to get the vm with UUID %s. error: %v", vmUuid, err)
		return nil, fmt.Errorf("Error retriving the created vm %s", vmName)
	}

	vm, err = findVMByUUID(mscp.nutanixClient, vmUuid)
	for err != nil {
		klog.Errorf("Failed to find the vm with UUID %s. %v", vmUuid, err)
		return nil, err
	}
	klog.Infof("The vm is ready. vmUUID: %s, vmState: %s", vmUuid, *vm.Status.State)

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
	klog.Infof("Checking if VM with name %s exists.", vmName)

	res, err := ntnxclient.V3.ListVM(&nutanixClientV3.DSMetadata{
		Filter: utils.StringPtr(fmt.Sprintf("vm_name==%s", vmName))})

	if err != nil {
		err = fmt.Errorf("Error when finding VM by name %s. error: %v", vmName, err)
		klog.Errorf(err.Error())
		return nil, err
	}
	if len(res.Entities) == 0 {
		err = fmt.Errorf("Not Found VM by name %s. error: %v", vmName, err)
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
		klog.Infof("Error deleting vm with uuid %s", vmUUID)
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

// findImageByName retrieves the Image with the given vm name
func findImageUuidByName(ntnxclient *nutanixClientV3.Client, imageName string) (*string, error) {
	klog.Infof("Checking if Image with name %s exists.", imageName)

	res, err := ntnxclient.V3.ListImage(&nutanixClientV3.DSMetadata{
		//Kind: utils.StringPtr("image"),
		Filter: utils.StringPtr(fmt.Sprintf("name==%s", imageName)),
	})
	if err != nil || len(res.Entities) == 0 {
		klog.Errorf("Failed to find image by name %s. error: %v", imageName, err)
		return nil, fmt.Errorf("Failed to find image by name %s. error: %v", imageName, err)
	}

	if len(res.Entities) > 1 {
		klog.Warningf("Found more than one (%v) images with name %s.", len(res.Entities), imageName)
	}

	return res.Entities[0].Metadata.UUID, nil
}
