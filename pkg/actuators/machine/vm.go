package machine

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"

	"github.com/nutanix-cloud-native/prism-go-client/utils"
	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/v3"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	clientpkg "github.com/openshift/machine-api-provider-nutanix/pkg/client"
)

// validateVMConfig verifies if the machine's VM configuration is valid
func validateVMConfig(ctx context.Context, mscp *machineScope) field.ErrorList {
	errList := field.ErrorList{}
	fldPath := field.NewPath("spec", "providerSpec", "value")
	var errMsg string

	// Will use the parameters in providerSpecValidated to create the VM
	mscp.providerSpecValidated = mscp.providerSpec.DeepCopy()

	// If the providerSpec configures the failureDomain reference, use the referenced failureDomain's
	// configuration for correponding configuration in the providerSpec
	if mscp.failureDomain != nil {
		mscp.providerSpecValidated.Cluster = getMachineResourceIdentifierFromFailureDomainConfig(mscp.failureDomain.Cluster)
		mscp.providerSpecValidated.Subnets = make([]machinev1.NutanixResourceIdentifier, len(mscp.failureDomain.Subnets))
		for i, subnet := range mscp.failureDomain.Subnets {
			mscp.providerSpecValidated.Subnets[i] = getMachineResourceIdentifierFromFailureDomainConfig(subnet)
		}
	}

	// verify the cluster configuration
	if fldErr := validateClusterConfig(ctx, mscp, fldPath); fldErr != nil {
		errList = append(errList, fldErr)
	}

	// verify the image configuration
	if fldErr := validateImageConfig(ctx, mscp, fldPath); fldErr != nil {
		errList = append(errList, fldErr)
	}

	// verify the subnets configuration
	numOfSubnets := len(mscp.providerSpecValidated.Subnets)
	switch {
	case numOfSubnets == 0:
		errList = append(errList, field.Required(fldPath.Child("subnets"), "Missing subnets"))
	case numOfSubnets > 32:
		errList = append(errList, field.TooMany(fldPath.Child("subnets"), numOfSubnets, 32))
	default:
		errList = append(errList, validateSubnetsConfig(ctx, mscp, fldPath)...)
	}

	// verify the vcpusPerSocket configuration
	if mscp.providerSpecValidated.VCPUsPerSocket < 1 {
		errMsg = "The minimum number of vCPUs per socket of the VM is 1."
		errList = append(errList, field.Invalid(fldPath.Child("vcpusPerSocket"), mscp.providerSpecValidated.VCPUsPerSocket, errMsg))
	}

	// verify the vcpuSockets configuration
	if mscp.providerSpecValidated.VCPUSockets < 1 {
		errMsg = "The minimum vCPU sockets of the VM is 1."
		errList = append(errList, field.Invalid(fldPath.Child("vcpuSockets"), mscp.providerSpecValidated.VCPUSockets, errMsg))
	}

	// verify the memorySize configuration
	// The minimum memorySize is 2Gi bytes
	memSizeMib := GetMibValueOfQuantity(mscp.providerSpecValidated.MemorySize)
	if memSizeMib < 2*1024 {
		errList = append(errList, field.Invalid(fldPath.Child("memorySize"), fmt.Sprintf("%vMib", memSizeMib), "The minimum memorySize is 2Gi bytes"))
	}

	// verify the systemDiskSize configuration
	// The minimum systemDiskSize is 20Gi bytes
	diskSizeMib := GetMibValueOfQuantity(mscp.providerSpecValidated.SystemDiskSize)
	if diskSizeMib < 20*1024 {
		errList = append(errList, field.Invalid(fldPath.Child("systemDiskSize"), fmt.Sprintf("%vGib", diskSizeMib/1024), "The minimum systemDiskSize is 20Gi bytes"))
	}

	// verify the bootType configurations
	// Type bootType field is optional, and valid values include: "", Legacy, UEFI, SecureBoot
	switch mscp.providerSpecValidated.BootType {
	case "", machinev1.NutanixLegacyBoot, machinev1.NutanixUEFIBoot, machinev1.NutanixSecureBoot:
		// valid bootType
	default:
		errMsg = fmt.Sprintf("Invalid bootType, the valid bootType values are: \"\", %q, %q, %q.",
			machinev1.NutanixLegacyBoot, machinev1.NutanixUEFIBoot, machinev1.NutanixSecureBoot)
		errList = append(errList, field.Invalid(fldPath.Child("bootType"), mscp.providerSpecValidated.BootType, errMsg))
	}

	// verify the project configuration
	if fldErr := validateProjectConfig(ctx, mscp, fldPath); fldErr != nil {
		errList = append(errList, fldErr)
	}

	// verify the categories configuration
	if len(mscp.providerSpecValidated.Categories) > 0 {
		fldErrs := validateCategoriesConfig(ctx, mscp, fldPath)
		for _, fldErr := range fldErrs {
			errList = append(errList, fldErr)
		}
	}

	// verify the gpus configuration
	if len(mscp.providerSpecValidated.GPUs) > 0 {
		fldErrs := validateGPUsConfig(ctx, mscp, fldPath)
		for _, fldErr := range fldErrs {
			errList = append(errList, fldErr)
		}
	}

	// verify the dataDisks configuration
	if len(mscp.providerSpecValidated.DataDisks) > 0 {
		fldErrs := validateDataDisksConfig(ctx, mscp, fldPath)
		for _, fldErr := range fldErrs {
			errList = append(errList, fldErr)
		}
	}

	return errList
}

func validateDataDisksConfig(ctx context.Context, mscp *machineScope, fldPath *field.Path) (fldErrs []*field.Error) {
	var err error
	var errMsg string
	disks := mscp.providerSpecValidated.DataDisks

	for _, disk := range disks {
		diskSizeMib := GetMibValueOfQuantity(disk.DiskSize)
		if diskSizeMib < 1024 {
			fldErrs = append(fldErrs, field.Invalid(fldPath.Child("dataDisks", "diskSize"), fmt.Sprintf("%vMi bytes", diskSizeMib), "The minimum diskSize is 1Gi bytes."))
		}

		if disk.DeviceProperties != nil {
			switch disk.DeviceProperties.DeviceType {
			case machinev1.NutanixDiskDeviceTypeDisk:
				switch disk.DeviceProperties.AdapterType {
				case machinev1.NutanixDiskAdapterTypeSCSI, machinev1.NutanixDiskAdapterTypeIDE, machinev1.NutanixDiskAdapterTypePCI, machinev1.NutanixDiskAdapterTypeSATA, machinev1.NutanixDiskAdapterTypeSPAPR:
					// valid configuration
				default:
					// invalid configuration
					fldErrs = append(fldErrs, field.Invalid(fldPath.Child("deviceProperties", "adapterType"), disk.DeviceProperties.AdapterType,
						fmt.Sprintf("invalid adapter type for the %q device type, the valid values: %q, %q, %q, %q, %q.",
							machinev1.NutanixDiskDeviceTypeDisk, machinev1.NutanixDiskAdapterTypeSCSI, machinev1.NutanixDiskAdapterTypeIDE,
							machinev1.NutanixDiskAdapterTypePCI, machinev1.NutanixDiskAdapterTypeSATA, machinev1.NutanixDiskAdapterTypeSPAPR)))
				}
			case machinev1.NutanixDiskDeviceTypeCDROM:
				switch disk.DeviceProperties.AdapterType {
				case machinev1.NutanixDiskAdapterTypeIDE, machinev1.NutanixDiskAdapterTypeSATA:
					// valid configuration
				default:
					// invalid configuration
					fldErrs = append(fldErrs, field.Invalid(fldPath.Child("deviceProperties", "adapterType"), disk.DeviceProperties.AdapterType,
						fmt.Sprintf("invalid adapter type for the %q device type, the valid values: %q, %q.",
							machinev1.NutanixDiskDeviceTypeCDROM, machinev1.NutanixDiskAdapterTypeIDE, machinev1.NutanixDiskAdapterTypeSATA)))
				}
			default:
				fldErrs = append(fldErrs, field.Invalid(fldPath.Child("deviceProperties", "deviceType"), disk.DeviceProperties.DeviceType,
					fmt.Sprintf("invalid device type, the valid types are: %q, %q.", machinev1.NutanixDiskDeviceTypeDisk, machinev1.NutanixDiskDeviceTypeCDROM)))
			}

			if disk.DeviceProperties.DeviceIndex < 0 {
				fldErrs = append(fldErrs, field.Invalid(fldPath.Child("deviceProperties", "deviceIndex"),
					disk.DeviceProperties.DeviceIndex, "invalid device index, the valid values are non-negative integers."))
			}
		}

		if disk.StorageConfig != nil {
			if disk.StorageConfig.DiskMode != machinev1.NutanixDiskModeStandard && disk.StorageConfig.DiskMode != machinev1.NutanixDiskModeFlash {
				fldErrs = append(fldErrs, field.Invalid(fldPath.Child("storageConfig", "diskMode"), disk.StorageConfig.DiskMode,
					fmt.Sprintf("invalid disk mode, the valid values: %q, %q.", machinev1.NutanixDiskModeStandard, machinev1.NutanixDiskModeFlash)))
			}

			storageContainerRef := disk.StorageConfig.StorageContainer
			if storageContainerRef != nil {
				switch storageContainerRef.Type {
				case machinev1.NutanixIdentifierUUID:
					if storageContainerRef.UUID == nil || *storageContainerRef.UUID == "" {
						fldErrs = append(fldErrs, field.Required(fldPath.Child("storageConfig", "storageContainer", "uuid"), "missing storageContainer uuid."))
					}
				default:
					errMsg = fmt.Sprintf("invalid storageContainer reference type, the valid values: %q.", machinev1.NutanixIdentifierUUID)
					fldErrs = append(fldErrs, field.Invalid(fldPath.Child("storageConfig", "storageContainer", "type"), storageContainerRef.Type, errMsg))
				}
			}
		}

		if disk.DataSource != nil {
			switch disk.DataSource.Type {
			case machinev1.NutanixIdentifierUUID:
				if disk.DataSource.UUID == nil || *disk.DataSource.UUID == "" {
					fldErrs = append(fldErrs, field.Required(fldPath.Child("dataDisks", "dataSource", "uuid"), "missing disk dataSource uuid."))
				} else {
					if _, err = mscp.nutanixClient.V3.GetImage(ctx, *disk.DataSource.UUID); err != nil {
						errMsg = fmt.Sprintf("failed to find the dataSource image with uuid %s. error: %v", *disk.DataSource.UUID, err)
						fldErrs = append(fldErrs, field.Invalid(fldPath.Child("dataDisks", "dataSource", "uuid"), *disk.DataSource.UUID, errMsg))
					}
				}
			case machinev1.NutanixIdentifierName:
				if disk.DataSource.Name == nil || *disk.DataSource.Name == "" {
					fldErrs = append(fldErrs, field.Required(fldPath.Child("dataDisks", "dataSource", "name"), "missing disk dataSource name."))
				} else {
					if dsUUID, err := findImageUuidByName(ctx, mscp.nutanixClient, *disk.DataSource.Name); err != nil {
						errMsg = fmt.Sprintf("failed to find the dataSource image with name %q. error: %v", *disk.DataSource.Name, err)
						fldErrs = append(fldErrs, field.Invalid(fldPath.Child("dataDisks", "dataSource", "name"), *disk.DataSource.Name, errMsg))
					} else {
						disk.DataSource.Type = machinev1.NutanixIdentifierUUID
						disk.DataSource.UUID = dsUUID
					}
				}
			default:
				errMsg := fmt.Sprintf("invalid disk dataSource reference type, the valid values: %q, %q.", machinev1.NutanixIdentifierUUID, machinev1.NutanixIdentifierName)
				fldErrs = append(fldErrs, field.Invalid(fldPath.Child("dataDisks", "dataSource", "type"), disk.DataSource.Type, errMsg))
			}
		}
	}

	return fldErrs
}

func validateGPUsConfig(ctx context.Context, mscp *machineScope, fldPath *field.Path) (fldErrs []*field.Error) {
	var errMsg string
	gpus := mscp.providerSpecValidated.GPUs
	peUUID := *mscp.providerSpecValidated.Cluster.UUID

	peGPUs, err := getGPUsForPE(ctx, mscp.nutanixClient, peUUID)
	if err != nil || len(peGPUs) == 0 {
		err = fmt.Errorf("no available GPUs found in Prism Element cluster (uuid: %s): %w", peUUID, err)
		fldErrs = append(fldErrs, field.InternalError(fldPath.Child("gpus"), err))
		return fldErrs
	}

	for _, gpu := range gpus {
		switch gpu.Type {
		case machinev1.NutanixGPUIdentifierDeviceID:
			if gpu.DeviceID == nil {
				fldErrs = append(fldErrs, field.Required(fldPath.Child("gpus", "deviceID"), "missing gpu deviceID"))
			} else {
				_, err := getGPUFromList(gpu, peGPUs)
				if err != nil {
					fldErrs = append(fldErrs, field.Invalid(fldPath.Child("gpus", "deviceID"), *gpu.DeviceID, err.Error()))
				}
			}
		case machinev1.NutanixGPUIdentifierName:
			if gpu.Name == nil || *gpu.Name == "" {
				fldErrs = append(fldErrs, field.Required(fldPath.Child("gpus", "name"), "missing gpu name"))
			} else {
				_, err := getGPUFromList(gpu, peGPUs)
				if err != nil {
					fldErrs = append(fldErrs, field.Invalid(fldPath.Child("gpus", "name"), gpu.Name, err.Error()))
				}
			}
		default:
			errMsg = fmt.Sprintf("invalid gpu identifier type, the valid values: %q, %q.", machinev1.NutanixGPUIdentifierDeviceID, machinev1.NutanixGPUIdentifierName)
			fldErrs = append(fldErrs, field.Invalid(fldPath.Child("gpus", "type"), gpu.Type, errMsg))
		}
	}

	return fldErrs
}

func validateClusterConfig(ctx context.Context, mscp *machineScope, fldPath *field.Path) *field.Error {
	var errMsg string
	var clusterName string

	switch mscp.providerSpecValidated.Cluster.Type {
	case machinev1.NutanixIdentifierName:
		if mscp.providerSpecValidated.Cluster.Name == nil || *mscp.providerSpecValidated.Cluster.Name == "" {
			return field.Required(fldPath.Child("cluster", "name"), "Missing cluster name")
		} else {
			clusterName = *mscp.providerSpecValidated.Cluster.Name
			clusterRefUuidPtr, err := findClusterUuidByName(ctx, mscp.nutanixClient, clusterName)
			if err != nil {
				errMsg = fmt.Sprintf("Failed to find cluster with name %q. error: %v", clusterName, err)
				return field.Invalid(fldPath.Child("cluster", "name"), clusterName, errMsg)
			} else {
				mscp.providerSpecValidated.Cluster.Type = machinev1.NutanixIdentifierUUID
				mscp.providerSpecValidated.Cluster.UUID = clusterRefUuidPtr
			}
		}
	case machinev1.NutanixIdentifierUUID:
		if mscp.providerSpecValidated.Cluster.UUID == nil || *mscp.providerSpecValidated.Cluster.UUID == "" {
			return field.Required(fldPath.Child("cluster", "uuid"), "Missing cluster uuid")
		} else {
			clusterUUID := *mscp.providerSpecValidated.Cluster.UUID
			if _, err := mscp.nutanixClient.V3.GetCluster(ctx, clusterUUID); err != nil {
				errMsg = fmt.Sprintf("Failed to find cluster with uuid %v. error: %v", clusterUUID, err)
				return field.Invalid(fldPath.Child("cluster", "uuid"), clusterUUID, errMsg)
			}
		}
	default:
		errMsg = fmt.Sprintf("Invalid cluster identifier type, valid types are: %q, %q.", configv1.NutanixIdentifierName, configv1.NutanixIdentifierUUID)
		return field.Invalid(fldPath.Child("cluster", "type"), mscp.providerSpecValidated.Cluster.Type, errMsg)
	}

	return nil
}

func validateImageConfig(ctx context.Context, mscp *machineScope, fldPath *field.Path) *field.Error {
	var err error
	var errMsg string

	switch mscp.providerSpecValidated.Image.Type {
	case machinev1.NutanixIdentifierName:
		if mscp.providerSpecValidated.Image.Name == nil || *mscp.providerSpecValidated.Image.Name == "" {
			return field.Required(fldPath.Child("image", "name"), "Missing image name")
		} else {
			imageName := *mscp.providerSpecValidated.Image.Name
			imageRefUuidPtr, err := findImageUuidByName(ctx, mscp.nutanixClient, imageName)
			if err != nil {
				errMsg = fmt.Sprintf("Failed to find image with name %q. error: %v", imageName, err)
				return field.Invalid(fldPath.Child("image", "name"), imageName, errMsg)
			} else {
				mscp.providerSpecValidated.Image.Type = machinev1.NutanixIdentifierUUID
				mscp.providerSpecValidated.Image.UUID = imageRefUuidPtr
			}
		}
	case machinev1.NutanixIdentifierUUID:
		if mscp.providerSpecValidated.Image.UUID == nil || *mscp.providerSpecValidated.Image.UUID == "" {
			return field.Required(fldPath.Child("image", "uuid"), "Missing image uuid")
		} else {
			imageUUID := *mscp.providerSpecValidated.Image.UUID
			if _, err = mscp.nutanixClient.V3.GetImage(ctx, imageUUID); err != nil {
				errMsg = fmt.Sprintf("Failed to find image with uuid %v. error: %v", imageUUID, err)
				return field.Invalid(fldPath.Child("image", "uuid"), imageUUID, errMsg)
			}
		}
	default:
		errMsg = fmt.Sprintf("Invalid image identifier type, valid types are: %q, %q.", configv1.NutanixIdentifierName, configv1.NutanixIdentifierUUID)
		return field.Invalid(fldPath.Child("image", "type"), mscp.providerSpecValidated.Image.Type, errMsg)
	}

	return nil
}

func validateSubnetsConfig(ctx context.Context, mscp *machineScope, fldPath *field.Path) field.ErrorList {
	fldErrs := field.ErrorList{}
	var errMsg string

	for i, subnet := range mscp.providerSpecValidated.Subnets {
		switch subnet.Type {
		case machinev1.NutanixIdentifierName:
			if subnet.Name == nil || *subnet.Name == "" {
				fldErrs = append(fldErrs, field.Required(fldPath.Child("subnet", "name"), "Missing subnet name"))
			} else {
				subnetName := *subnet.Name
				subnetRefUuidPtr, err := findSubnetUuidByName(ctx, mscp.nutanixClient, subnetName)
				if err != nil {
					errMsg = fmt.Sprintf("Failed to find subnet with name %q. error: %v", subnetName, err)
					fldErrs = append(fldErrs, field.Invalid(fldPath.Child("subnet", "name"), subnetName, errMsg))
				} else {
					mscp.providerSpecValidated.Subnets[i].Type = machinev1.NutanixIdentifierUUID
					mscp.providerSpecValidated.Subnets[i].UUID = subnetRefUuidPtr
				}
			}
		case machinev1.NutanixIdentifierUUID:
			if subnet.UUID == nil || *subnet.UUID == "" {
				fldErrs = append(fldErrs, field.Required(fldPath.Child("subnet").Child("uuid"), "Missing subnet uuid"))
			} else {
				_, err := mscp.nutanixClient.V3.GetSubnet(ctx, *subnet.UUID)
				if err != nil {
					errMsg = fmt.Sprintf("Failed to find subnet with uuid %v. error: %v", *subnet.UUID, err)
					fldErrs = append(fldErrs, field.Invalid(fldPath.Child("subnet", "uuid"), *subnet.UUID, errMsg))
				}
			}
		default:
			errMsg = fmt.Sprintf("Invalid subnet identifier type, valid types are: %q, %q.", configv1.NutanixIdentifierName, configv1.NutanixIdentifierUUID)
			fldErrs = append(fldErrs, field.Invalid(fldPath.Child("subnet", "type"), subnet.Type, errMsg))
		}
	}

	return fldErrs
}

func validateProjectConfig(ctx context.Context, mscp *machineScope, fldPath *field.Path) *field.Error {
	var err error
	var errMsg string

	switch mscp.providerSpecValidated.Project.Type {
	case "":
		// ignore if not configured
		return nil
	case machinev1.NutanixIdentifierName:
		if mscp.providerSpecValidated.Project.Name == nil || *mscp.providerSpecValidated.Project.Name == "" {
			return field.Required(fldPath.Child("project", "name"), "Missing projct name")
		} else {
			projectName := *mscp.providerSpecValidated.Project.Name
			projectRefUuidPtr, err := findProjectUuidByName(ctx, mscp.nutanixClient, projectName)
			if err != nil {
				errMsg = fmt.Sprintf("Failed to find project with name %q. error: %v", projectName, err)
				return field.Invalid(fldPath.Child("project", "name"), projectName, errMsg)
			} else {
				mscp.providerSpecValidated.Project.Type = machinev1.NutanixIdentifierUUID
				mscp.providerSpecValidated.Project.UUID = projectRefUuidPtr
			}
		}
	case machinev1.NutanixIdentifierUUID:
		if mscp.providerSpecValidated.Project.UUID == nil || *mscp.providerSpecValidated.Project.UUID == "" {
			return field.Required(fldPath.Child("project", "uuid"), "Missing project uuid")
		} else {
			projectUUID := *mscp.providerSpecValidated.Project.UUID
			if _, err = mscp.nutanixClient.V3.GetProject(ctx, projectUUID); err != nil {
				errMsg = fmt.Sprintf("Failed to find project with uuid %v. error: %v", projectUUID, err)
				return field.Invalid(fldPath.Child("project", "uuid"), projectUUID, errMsg)
			}
		}
	default:
		errMsg = fmt.Sprintf("Invalid project identifier type, valid types are: %q, %q.", configv1.NutanixIdentifierName, configv1.NutanixIdentifierUUID)
		return field.Invalid(fldPath.Child("project", "type"), mscp.providerSpecValidated.Project.Type, errMsg)
	}

	return nil
}

func validateCategoriesConfig(ctx context.Context, mscp *machineScope, fldPath *field.Path) (fldErrs []*field.Error) {
	for _, category := range mscp.providerSpecValidated.Categories {
		_, err := mscp.nutanixClient.V3.GetCategoryValue(ctx, category.Key, category.Value)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to find the category with key %q and value %q. error: %v", category.Key, category.Value, err)
			fldErrs = append(fldErrs, field.Invalid(fldPath.Child("categories"), category, errMsg))
		}
	}

	return fldErrs
}

// CreateVM creates a VM and is invoked by the NutanixMachineReconciler
func createVM(ctx context.Context, mscp *machineScope, userData []byte) (*nutanixClientV3.VMIntentResponse, error) {

	var err error
	var vm *nutanixClientV3.VMIntentResponse
	var vmUuid string
	vmName := mscp.machine.Name
	startTime := time.Now()

	// Check if the VM already exists
	if mscp.providerStatus.VmUUID != nil {
		// Try to find the vm by uuid
		vm, err = findVMByUUID(ctx, mscp.nutanixClient, *mscp.providerStatus.VmUUID)
		if err == nil {
			vmUuid = *vm.Metadata.UUID
			klog.Infof("%s: The VM with UUID %s already exists. No need to create one.", vmName, vmUuid)
		}
	}

	if len(vmUuid) == 0 {
		// Encode the userData by base64
		userdataEncoded := base64.StdEncoding.EncodeToString(userData)

		// Create the VM
		klog.V(3).Infof("To create VM with name %q, and providerSpec: %+v", vmName, *mscp.providerSpecValidated)
		vmInput := nutanixClientV3.VMIntentInput{}
		vmSpec := nutanixClientV3.VM{Name: utils.StringPtr(vmName)}

		nicList := []*nutanixClientV3.VMNic{}
		for _, subnet := range mscp.providerSpecValidated.Subnets {
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
				UUID: mscp.providerSpecValidated.Image.UUID,
			},
			DiskSizeMib: utils.Int64Ptr(GetMibValueOfQuantity(mscp.providerSpecValidated.SystemDiskSize)),
		})

		// Add the configured dataDisks
		for _, dataDisk := range mscp.providerSpecValidated.DataDisks {
			vmDisk := &nutanixClientV3.VMDisk{}
			vmDisk.DiskSizeMib = utils.Int64Ptr(GetMibValueOfQuantity(dataDisk.DiskSize))

			if dataDisk.DeviceProperties != nil {
				deviceType := ""
				if dataDisk.DeviceProperties.DeviceType == machinev1.NutanixDiskDeviceTypeDisk {
					deviceType = "DISK"
				} else if dataDisk.DeviceProperties.DeviceType == machinev1.NutanixDiskDeviceTypeCDROM {
					deviceType = "CDROM"
				}
				vmDisk.DeviceProperties = &nutanixClientV3.VMDiskDeviceProperties{
					DeviceType: utils.StringPtr(deviceType),
					DiskAddress: &nutanixClientV3.DiskAddress{
						AdapterType: utils.StringPtr(string(dataDisk.DeviceProperties.AdapterType)),
						DeviceIndex: utils.Int64Ptr(int64(dataDisk.DeviceProperties.DeviceIndex)),
					},
				}
			}

			if dataDisk.StorageConfig != nil {
				vmDisk.StorageConfig = &nutanixClientV3.VMStorageConfig{}
				if dataDisk.StorageConfig.DiskMode == machinev1.NutanixDiskModeFlash {
					vmDisk.StorageConfig.FlashMode = "ON"
				}

				if dataDisk.StorageConfig.StorageContainer != nil && dataDisk.StorageConfig.StorageContainer.UUID != nil {
					vmDisk.StorageConfig.StorageContainerReference = &nutanixClientV3.StorageContainerReference{
						Kind: "storage_container",
						UUID: *dataDisk.StorageConfig.StorageContainer.UUID,
					}
				}
			}

			if dataDisk.DataSource != nil && dataDisk.DataSource.UUID != nil {
				vmDisk.DataSourceReference = &nutanixClientV3.Reference{
					Kind: utils.StringPtr("image"),
					UUID: dataDisk.DataSource.UUID,
				}
			}

			diskList = append(diskList, vmDisk)
		}

		// Get GPU list
		gpuList, err := getGPUList(ctx, mscp.nutanixClient, mscp.providerSpecValidated.GPUs, *mscp.providerSpecValidated.Cluster.UUID)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to get the GPU list to create the VM: %w", vmName, err)
		}

		vmMetadata := nutanixClientV3.Metadata{
			Kind:        utils.StringPtr("vm"),
			SpecVersion: utils.Int64Ptr(1),
		}

		// Add the projectReference if project is configured
		if mscp.providerSpecValidated.Project.Type == machinev1.NutanixIdentifierUUID {
			vmMetadata.ProjectReference = &nutanixClientV3.Reference{
				Kind: utils.StringPtr("project"),
				UUID: mscp.providerSpecValidated.Project.UUID,
			}
		}

		// Add the categories
		if err := addCategories(mscp, &vmMetadata); err != nil {
			klog.Warningf("Failed to add categories to the vm %q. %v", vmName, err)
		} else {
			klog.Infof("%s: added the categories to the vm %q: %v", mscp.machine.Name, vmName, vmMetadata.Categories)
		}

		vmSpec.Resources = &nutanixClientV3.VMResources{
			PowerState:            utils.StringPtr("ON"),
			HardwareClockTimezone: utils.StringPtr("UTC"),
			NumVcpusPerSocket:     utils.Int64Ptr(int64(mscp.providerSpecValidated.VCPUsPerSocket)),
			NumSockets:            utils.Int64Ptr(int64(mscp.providerSpecValidated.VCPUSockets)),
			MemorySizeMib:         utils.Int64Ptr(GetMibValueOfQuantity(mscp.providerSpecValidated.MemorySize)),
			NicList:               nicList,
			DiskList:              diskList,
			GuestCustomization: &nutanixClientV3.GuestCustomization{
				IsOverridable: utils.BoolPtr(true),
				CloudInit:     &nutanixClientV3.GuestCustomizationCloudInit{UserData: utils.StringPtr(userdataEncoded)}},
		}

		// Set cluster/PE reference
		vmSpec.ClusterReference = &nutanixClientV3.Reference{
			Kind: utils.StringPtr("cluster"),
			UUID: mscp.providerSpecValidated.Cluster.UUID,
		}

		if len(gpuList) > 0 {
			vmSpec.Resources.GpuList = gpuList
		}

		// Set boot_type if configured in the machine's providerSpec
		switch mscp.providerSpecValidated.BootType {
		case machinev1.NutanixUEFIBoot:
			vmSpec.Resources.BootConfig = &nutanixClientV3.VMBootConfig{
				BootType: utils.StringPtr("UEFI"),
			}
		case machinev1.NutanixSecureBoot:
			vmSpec.Resources.BootConfig = &nutanixClientV3.VMBootConfig{
				BootType: utils.StringPtr("SECURE_BOOT"),
			}
		case machinev1.NutanixLegacyBoot:
			bootDeviceOrderList := make([]*string, 0)
			bootDeviceOrderList = append(bootDeviceOrderList, utils.StringPtr("CDROM"))
			bootDeviceOrderList = append(bootDeviceOrderList, utils.StringPtr("DISK"))
			bootDeviceOrderList = append(bootDeviceOrderList, utils.StringPtr("NETWORK"))
			vmSpec.Resources.BootConfig = &nutanixClientV3.VMBootConfig{
				BootType:            utils.StringPtr("LEGACY"),
				BootDeviceOrderList: bootDeviceOrderList,
			}
		default:
			// The vm uses the default boot type
		}

		vmInput.Spec = &vmSpec
		vmInput.Metadata = &vmMetadata
		vm, err = mscp.nutanixClient.V3.CreateVM(ctx, &vmInput)
		if err != nil {
			klog.Errorf("Failed to create VM %q. error: %v", vmName, err)
			return nil, err
		}

		if taskUUID, ok := vm.Status.ExecutionContext.TaskUUID.(string); ok {
			klog.Infof("%s: waiting for the vm creation task to complete, taskUUID: %s.", vmName, taskUUID)
			if err = waitForTask(mscp.nutanixClient.V3, taskUUID); err != nil {
				e1 := fmt.Errorf("%s failed to create the vm: %w", vmName, err)
				klog.Error(e1.Error())
				return nil, e1
			}
		} else {
			err = fmt.Errorf("failed to convert the vm creation task UUID %v to string", vm.Status.ExecutionContext.TaskUUID)
			klog.Errorf(err.Error())
			return nil, err
		}

		vmUuid = *vm.Metadata.UUID
		klog.Infof("%s: successfully created the vm. The vm UUID: %s", vmName, vmUuid)
	}

	vm, err = findVMByUUID(ctx, mscp.nutanixClient, vmUuid)
	if err != nil {
		klog.Errorf("Failed to find the vm with UUID %s. %v", vmUuid, err)
		return nil, err
	}
	klog.Infof("The vm %q is ready. vmUUID: %s, vmState: %s, used_time: %v", vmName, vmUuid, *vm.Status.State, time.Since(startTime).String())

	return vm, nil
}

// findVMByUUID retrieves the VM with the given vm UUID
func findVMByUUID(ctx context.Context, ntnxclient *nutanixClientV3.Client, uuid string) (*nutanixClientV3.VMIntentResponse, error) {
	klog.Infof("Checking if VM with UUID %s exists.", uuid)

	response, err := ntnxclient.V3.GetVM(ctx, uuid)
	if err != nil {
		klog.Errorf("Failed to find VM by vmUUID %s. error: %v", uuid, err)
		return nil, err
	}

	return response, nil
}

// findVMByName retrieves the VM with the given vm name
func findVMByName(ctx context.Context, ntnxclient *nutanixClientV3.Client, vmName string) (*nutanixClientV3.VMIntentResponse, error) {
	klog.Infof("Checking if VM with name %q exists.", vmName)

	res, err := ntnxclient.V3.ListVM(ctx, &nutanixClientV3.DSMetadata{
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
func deleteVM(ctx context.Context, ntnxclient *nutanixClientV3.Client, vmUUID string) error {
	klog.Infof("Deleting VM with UUID %s.", vmUUID)

	_, err := ntnxclient.V3.DeleteVM(ctx, vmUUID)
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
func findClusterUuidByName(ctx context.Context, ntnxclient *nutanixClientV3.Client, clusterName string) (*string, error) {
	klog.Infof("Checking if cluster with name %q exists.", clusterName)

	r, e := ntnxclient.V3.ListAllCluster(ctx, "")
	if e != nil {
		return nil, fmt.Errorf("Failed to list all clusters. error: %w", e)
	}

	var clusterUUID *string
	numClusters := 0

	for _, item := range r.Entities {
		if *item.Spec.Name == clusterName {
			numClusters++
			clusterUUID = item.Metadata.UUID
		}
	}

	if numClusters == 0 {
		return nil, fmt.Errorf("Unable to find cluster by name %q.", clusterName)
	}

	if numClusters > 1 {
		return nil, fmt.Errorf("Found more than one (%v) clusters with name %q.", numClusters, clusterName)
	}

	return clusterUUID, nil
}

// findImageByName retrieves the image uuid by the given image name
func findImageUuidByName(ctx context.Context, ntnxclient *nutanixClientV3.Client, imageName string) (*string, error) {
	klog.Infof("Checking if image with name %q exists.", imageName)

	res, err := ntnxclient.V3.ListImage(ctx, &nutanixClientV3.DSMetadata{
		//Kind: utils.StringPtr("image"),
		Filter: utils.StringPtr(fmt.Sprintf("name==%s", imageName)),
	})
	if err != nil || len(res.Entities) == 0 {
		e1 := fmt.Errorf("Failed to find image by name %q. error: %w", imageName, err)
		klog.Errorf(e1.Error())
		return nil, e1
	}

	if len(res.Entities) > 1 {
		err = fmt.Errorf("Found more than one (%v) images with name %q.", len(res.Entities), imageName)
		klog.Errorf(err.Error())
		return nil, err
	}

	return res.Entities[0].Metadata.UUID, nil
}

// findSubnetUuidByName retrieves the subnet uuid by the given subnet name
func findSubnetUuidByName(ctx context.Context, ntnxclient *nutanixClientV3.Client, subnetName string) (*string, error) {
	klog.Infof("Checking if subnet with name %q exists.", subnetName)

	res, err := ntnxclient.V3.ListSubnet(ctx, &nutanixClientV3.DSMetadata{
		//Kind: utils.StringPtr("subnet"),
		Filter: utils.StringPtr(fmt.Sprintf("name==%s", subnetName)),
	})
	if err != nil || len(res.Entities) == 0 {
		e1 := fmt.Errorf("Failed to find subnet by name %q. error: %w", subnetName, err)
		klog.Errorf(e1.Error())
		return nil, e1
	}

	if len(res.Entities) > 1 {
		err = fmt.Errorf("Found more than one (%v) subnets with name %q.", len(res.Entities), subnetName)
		klog.Errorf(err.Error())
		return nil, err
	}

	return res.Entities[0].Metadata.UUID, nil
}

// findProjectUuidByName retrieves the project uuid by the given project name
func findProjectUuidByName(ctx context.Context, ntnxclient *nutanixClientV3.Client, projectName string) (*string, error) {
	klog.Infof("Checking if project with name %q exists.", projectName)

	res, err := ntnxclient.V3.ListProject(ctx, &nutanixClientV3.DSMetadata{
		Filter: utils.StringPtr(fmt.Sprintf("name==%s", projectName)),
	})
	if err != nil || len(res.Entities) == 0 {
		e1 := fmt.Errorf("Failed to find project by name %q. error: %w", projectName, err)
		klog.Errorf(e1.Error())
		return nil, e1
	}

	if len(res.Entities) > 1 {
		err = fmt.Errorf("Found more than one (%v) projects with name %q.", len(res.Entities), projectName)
		klog.Errorf(err.Error())
		return nil, err
	}

	return res.Entities[0].Metadata.UUID, nil
}

func getPrismCentralCluster(ctx context.Context, ntnxclient *nutanixClientV3.Client) (*nutanixClientV3.ClusterIntentResponse, error) {
	clusterList, err := ntnxclient.V3.ListAllCluster(ctx, "")
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
