package machine

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/nutanix-cloud-native/prism-go-client/converged"
	v4Converged "github.com/nutanix-cloud-native/prism-go-client/converged/v4"
	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/v3"
	clusterModels "github.com/nutanix/ntnx-api-golang-clients/clustermgmt-go-client/v4/models/clustermgmt/v4/config"
	prismModels "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/prism/v4/config"
	vmmModels "github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/vmm/v4/ahv/config"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
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
					if _, err = mscp.nutanixClient.Images.Get(ctx, *disk.DataSource.UUID); err != nil {
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
	gpus := mscp.providerSpecValidated.GPUs
	if len(gpus) == 0 {
		return nil
	}
	peUUID := *mscp.providerSpecValidated.Cluster.UUID
	peGPUs, err := getGPUsForPEConverged(ctx, mscp.nutanixClient, peUUID)
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
				_, err := getGPUFromListConverged(gpu, peGPUs)
				if err != nil {
					fldErrs = append(fldErrs, field.Invalid(fldPath.Child("gpus", "deviceID"), *gpu.DeviceID, err.Error()))
				}
			}
		case machinev1.NutanixGPUIdentifierName:
			if gpu.Name == nil || *gpu.Name == "" {
				fldErrs = append(fldErrs, field.Required(fldPath.Child("gpus", "name"), "missing gpu name"))
			} else {
				_, err := getGPUFromListConverged(gpu, peGPUs)
				if err != nil {
					fldErrs = append(fldErrs, field.Invalid(fldPath.Child("gpus", "name"), gpu.Name, err.Error()))
				}
			}
		default:
			errMsg := fmt.Sprintf("invalid gpu identifier type, the valid values: %q, %q.", machinev1.NutanixGPUIdentifierDeviceID, machinev1.NutanixGPUIdentifierName)
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
			if _, err := mscp.nutanixClient.Clusters.Get(ctx, clusterUUID); err != nil {
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
			if _, err = mscp.nutanixClient.Images.Get(ctx, imageUUID); err != nil {
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
				_, err := mscp.nutanixClient.Subnets.Get(ctx, *subnet.UUID)
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
			projectRefUuidPtr, err := findProjectUuidByName(ctx, mscp.nutanixV3Client, projectName)
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
			if _, err = getProjectByUUID(ctx, mscp.nutanixV3Client, projectUUID); err != nil {
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
		_, err := getCategoryValue(ctx, mscp.nutanixClient, category.Key, category.Value)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to find the category with key %q and value %q. error: %v", category.Key, category.Value, err)
			fldErrs = append(fldErrs, field.Invalid(fldPath.Child("categories"), category, errMsg))
		}
	}

	return fldErrs
}

// CreateVM creates a VM using the converged client and returns the created VM.
func createVM(ctx context.Context, mscp *machineScope, userData []byte) (*vmmModels.Vm, error) {
	vmName := mscp.machine.Name
	startTime := time.Now()

	if mscp.providerStatus.VmUUID != nil {
		vm, err := findVMByUUID(ctx, mscp.nutanixClient, *mscp.providerStatus.VmUUID)
		if err == nil {
			klog.Infof("%s: VM with UUID %s already exists.", vmName, *vm.ExtId)
			return vm, nil
		}
	}

	userdataEncoded := base64.StdEncoding.EncodeToString(userData)
	v4vm, err := buildV4VMFromScope(ctx, mscp, userdataEncoded)
	if err != nil {
		return nil, fmt.Errorf("building v4 VM spec: %w", err)
	}

	klog.V(3).Infof("Creating VM %q with converged client.", vmName)
	created, err := mscp.nutanixClient.VMs.Create(ctx, v4vm)
	if err != nil {
		klog.Errorf("Failed to create VM %q: %v", vmName, err)
		return nil, err
	}

	vmUuid := *created.ExtId
	klog.Infof("%s: VM created, UUID: %s. Powering on.", vmName, vmUuid)

	powerOnOp, err := mscp.nutanixClient.VMs.PowerOnVM(vmUuid)
	if err != nil {
		return nil, fmt.Errorf("power on VM: %w", err)
	}
	_, err = powerOnOp.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("wait for power on: %w", err)
	}

	vm, err := findVMByUUID(ctx, mscp.nutanixClient, vmUuid)
	if err != nil {
		return nil, err
	}
	klog.Infof("VM %q ready. vmUUID: %s, used_time: %v", vmName, vmUuid, time.Since(startTime))
	return vm, nil
}

// buildV4VMFromScope builds a v4 Vm from machine scope and base64-encoded userData.
func buildV4VMFromScope(ctx context.Context, mscp *machineScope, userdataEncoded string) (*vmmModels.Vm, error) {
	vmName := mscp.machine.Name
	spec := mscp.providerSpecValidated

	vm := vmmModels.NewVm()
	vm.Name = ptr.To(vmName)
	vm.MemorySizeBytes = ptr.To(spec.MemorySize.Value())
	vm.NumCoresPerSocket = ptr.To(int(spec.VCPUsPerSocket))
	vm.NumSockets = ptr.To(int(spec.VCPUSockets))
	vm.HardwareClockTimezone = ptr.To("UTC")

	clusterUUID := ""
	if spec.Cluster.UUID != nil {
		clusterUUID = *spec.Cluster.UUID
	} else {
		return nil, fmt.Errorf("cluster UUID is required")
	}
	vm.Cluster = vmmModels.NewClusterReference()
	vm.Cluster.ExtId = &clusterUUID

	// Nics
	nics := make([]vmmModels.Nic, 0, len(spec.Subnets))
	for _, sub := range spec.Subnets {
		if sub.UUID == nil {
			continue
		}
		nic := vmmModels.NewNic()
		nic.NetworkInfo = vmmModels.NewNicNetworkInfo()
		nic.NetworkInfo.Subnet = vmmModels.NewSubnetReference()
		nic.NetworkInfo.Subnet.ExtId = sub.UUID
		nics = append(nics, *nic)
	}
	vm.Nics = nics

	// System disk
	imageUUID := ""
	if spec.Image.UUID != nil {
		imageUUID = *spec.Image.UUID
	} else {
		return nil, fmt.Errorf("image UUID is required")
	}
	systemDiskSizeBytes := spec.SystemDiskSize.Value()
	sysDisk, err := newV4SystemDisk(&imageUUID, systemDiskSizeBytes)
	if err != nil {
		return nil, err
	}
	vm.Disks = []vmmModels.Disk{*sysDisk}

	// Data disks
	for _, dataDisk := range spec.DataDisks {
		diskSizeBytes := dataDisk.DiskSize.Value()

		vmDisk := vmmModels.NewVmDisk()
		if diskSizeBytes > 0 {
			vmDisk.DiskSizeBytes = &diskSizeBytes
		}

		if dataDisk.DataSource != nil && dataDisk.DataSource.UUID != nil {
			vmDisk.DataSource = vmmModels.NewDataSource()
			imgRef := vmmModels.NewImageReference()
			imgRef.ImageExtId = dataDisk.DataSource.UUID
			_ = vmDisk.DataSource.SetReference(*imgRef)
			vmDisk.DataSource.ReferenceItemDiscriminator_ = nil
		}

		if dataDisk.StorageConfig != nil {
			if dataDisk.StorageConfig.DiskMode == machinev1.NutanixDiskModeFlash {
				vmDisk.StorageConfig = vmmModels.NewVmDiskStorageConfig()
				vmDisk.StorageConfig.IsFlashModeEnabled = ptr.To(true)
			}
			if dataDisk.StorageConfig.StorageContainer != nil && dataDisk.StorageConfig.StorageContainer.UUID != nil {
				vmDisk.StorageContainer = vmmModels.NewVmDiskContainerReference()
				vmDisk.StorageContainer.ExtId = dataDisk.StorageConfig.StorageContainer.UUID
			}
		}

		if dataDisk.DeviceProperties != nil && dataDisk.DeviceProperties.DeviceType == machinev1.NutanixDiskDeviceTypeCDROM {
			cdrom := vmmModels.NewCdRom()
			cdrom.BackingInfo = vmDisk
			cdrom.DiskAddress = vmmModels.NewCdRomAddress()
			cdrom.DiskAddress.BusType = nutanixAdapterTypeToCdRomBusType(dataDisk.DeviceProperties.AdapterType).Ref()
			cdrom.DiskAddress.Index = ptr.To(int(dataDisk.DeviceProperties.DeviceIndex))
			vm.CdRoms = append(vm.CdRoms, *cdrom)
		} else {
			disk := vmmModels.NewDisk()
			if err := disk.SetBackingInfo(*vmDisk); err != nil {
				return nil, fmt.Errorf("setting backing info for data disk: %w", err)
			}
			if dataDisk.DeviceProperties != nil {
				disk.DiskAddress = vmmModels.NewDiskAddress()
				disk.DiskAddress.BusType = nutanixAdapterTypeToDiskBusType(dataDisk.DeviceProperties.AdapterType).Ref()
				disk.DiskAddress.Index = ptr.To(int(dataDisk.DeviceProperties.DeviceIndex))
			}
			vm.Disks = append(vm.Disks, *disk)
		}
	}

	// Boot type
	switch spec.BootType {
	case machinev1.NutanixUEFIBoot:
		uefiBoot := vmmModels.NewUefiBoot()
		uefiBoot.IsSecureBootEnabled = ptr.To(false)
		if err := vm.SetBootConfig(*uefiBoot); err != nil {
			return nil, fmt.Errorf("setting UEFI boot config: %w", err)
		}
	case machinev1.NutanixSecureBoot:
		uefiBoot := vmmModels.NewUefiBoot()
		uefiBoot.IsSecureBootEnabled = ptr.To(true)
		if err := vm.SetBootConfig(*uefiBoot); err != nil {
			return nil, fmt.Errorf("setting secure boot config: %w", err)
		}
	case machinev1.NutanixLegacyBoot:
		legacyBoot := vmmModels.NewLegacyBoot()
		legacyBoot.BootOrder = []vmmModels.BootDeviceType{
			vmmModels.BOOTDEVICETYPE_CDROM,
			vmmModels.BOOTDEVICETYPE_DISK,
			vmmModels.BOOTDEVICETYPE_NETWORK,
		}
		if err := vm.SetBootConfig(*legacyBoot); err != nil {
			return nil, fmt.Errorf("setting legacy boot config: %w", err)
		}
	default:
		// Use default boot type
	}

	// GPUs
	if len(spec.GPUs) > 0 {
		gpus, err := buildV4GPUList(ctx, mscp)
		if err != nil {
			return nil, fmt.Errorf("building GPU list: %w", err)
		}
		vm.Gpus = gpus
	}

	// Categories
	categoryRefs, err := buildV4CategoryRefs(ctx, mscp)
	if err != nil {
		klog.Warningf("Failed to build category references for VM %q: %v", vmName, err)
	} else if len(categoryRefs) > 0 {
		vm.Categories = categoryRefs
	}

	// Project
	if spec.Project.Type == machinev1.NutanixIdentifierUUID && spec.Project.UUID != nil && *spec.Project.UUID != "" {
		vm.Project = vmmModels.NewProjectReference()
		vm.Project.ExtId = spec.Project.UUID
		klog.V(3).Infof("VM %q: setting project reference to %s", vmName, *spec.Project.UUID)
	}

	// Guest customization (cloud-init)
	cloudInit := vmmModels.NewCloudInit()
	cloudInit.Metadata = ptr.To(base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(`{"hostname":"%s"}`, vmName))))
	cloudInit.DatasourceType = vmmModels.CLOUDINITDATASOURCETYPE_CONFIG_DRIVE_V2.Ref()
	userData := vmmModels.NewUserdata()
	userData.Value = &userdataEncoded
	_ = cloudInit.SetCloudInitScript(*userData)
	cloudInit.CloudInitScriptItemDiscriminator_ = nil
	vm.GuestCustomization = vmmModels.NewGuestCustomizationParams()
	_ = vm.GuestCustomization.SetConfig(*cloudInit)
	vm.GuestCustomization.ConfigItemDiscriminator_ = nil

	return vm, nil
}

// buildV4GPUList constructs v4 GPU specs from the provider spec GPUs.
func buildV4GPUList(ctx context.Context, mscp *machineScope) ([]vmmModels.Gpu, error) {
	spec := mscp.providerSpecValidated
	peUUID := *spec.Cluster.UUID

	peGPUs, err := getGPUsForPEConverged(ctx, mscp.nutanixClient, peUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve GPUs from PE cluster %s: %w", peUUID, err)
	}
	if len(peGPUs) == 0 {
		return nil, fmt.Errorf("no available GPUs found in PE cluster %s", peUUID)
	}

	gpus := make([]vmmModels.Gpu, 0, len(spec.GPUs))
	for _, gpu := range spec.GPUs {
		match, err := getGPUFromListConverged(gpu, peGPUs)
		if err != nil {
			return nil, fmt.Errorf("GPU not found matching required inputs: %w", err)
		}
		v4gpu := vmmModels.NewGpu()
		if match.DeviceID != nil {
			v4gpu.DeviceId = ptr.To(int(*match.DeviceID))
		}
		v4gpu.Name = ptr.To(match.Name)
		gpus = append(gpus, *v4gpu)
	}
	return gpus, nil
}

// buildV4CategoryRefs builds CategoryReference list for the VM.
func buildV4CategoryRefs(ctx context.Context, mscp *machineScope) ([]vmmModels.CategoryReference, error) {
	var refs []vmmModels.CategoryReference

	// Add user-configured categories
	for _, category := range mscp.providerSpecValidated.Categories {
		cat, err := getCategoryValue(ctx, mscp.nutanixClient, category.Key, category.Value)
		if err != nil {
			return nil, fmt.Errorf("category key=%q value=%q: %w", category.Key, category.Value, err)
		}
		if cat != nil && cat.ExtId != nil {
			ref := vmmModels.NewCategoryReference()
			ref.ExtId = cat.ExtId
			refs = append(refs, *ref)
		}
	}

	// Add the cluster ownership category for installer cleanup
	clusterID, ok := getClusterID(mscp.machine)
	if !ok || clusterID == "" {
		return refs, nil
	}
	categoryKey := fmt.Sprintf("%s%s", NutanixCategoryKeyPrefix, clusterID)
	cat, err := getCategoryValue(ctx, mscp.nutanixClient, categoryKey, NutanixCategoryValue)
	if err != nil {
		klog.Warningf("Failed to find cluster ownership category %q=%q: %v", categoryKey, NutanixCategoryValue, err)
		return refs, nil
	}
	if cat != nil && cat.ExtId != nil {
		ref := vmmModels.NewCategoryReference()
		ref.ExtId = cat.ExtId
		refs = append(refs, *ref)
	}

	return refs, nil
}

func nutanixAdapterTypeToDiskBusType(adapterType machinev1.NutanixDiskAdapterType) vmmModels.DiskBusType {
	switch adapterType {
	case machinev1.NutanixDiskAdapterTypeSCSI:
		return vmmModels.DISKBUSTYPE_SCSI
	case machinev1.NutanixDiskAdapterTypeIDE:
		return vmmModels.DISKBUSTYPE_IDE
	case machinev1.NutanixDiskAdapterTypePCI:
		return vmmModels.DISKBUSTYPE_PCI
	case machinev1.NutanixDiskAdapterTypeSATA:
		return vmmModels.DISKBUSTYPE_SATA
	case machinev1.NutanixDiskAdapterTypeSPAPR:
		return vmmModels.DISKBUSTYPE_SPAPR
	default:
		return vmmModels.DISKBUSTYPE_SCSI
	}
}

func nutanixAdapterTypeToCdRomBusType(adapterType machinev1.NutanixDiskAdapterType) vmmModels.CdRomBusType {
	switch adapterType {
	case machinev1.NutanixDiskAdapterTypeIDE:
		return vmmModels.CDROMBUSTYPE_IDE
	case machinev1.NutanixDiskAdapterTypeSATA:
		return vmmModels.CDROMBUSTYPE_SATA
	default:
		return vmmModels.CDROMBUSTYPE_IDE
	}
}

func newV4SystemDisk(imageUUID *string, diskSizeBytes int64) (*vmmModels.Disk, error) {
	vmDisk := vmmModels.NewVmDisk()
	if diskSizeBytes > 0 {
		vmDisk.DiskSizeBytes = &diskSizeBytes
	}
	if imageUUID != nil {
		vmDisk.DataSource = vmmModels.NewDataSource()
		imgRef := vmmModels.NewImageReference()
		imgRef.ImageExtId = imageUUID
		_ = vmDisk.DataSource.SetReference(*imgRef)
		vmDisk.DataSource.ReferenceItemDiscriminator_ = nil
	}
	disk := vmmModels.NewDisk()
	if err := disk.SetBackingInfo(*vmDisk); err != nil {
		return nil, err
	}
	return disk, nil
}

// findVMByUUID retrieves the VM with the given vm UUID using converged client.
func findVMByUUID(ctx context.Context, client *v4Converged.Client, uuid string) (*vmmModels.Vm, error) {
	klog.Infof("Checking if VM with UUID %s exists.", uuid)
	vm, err := client.VMs.Get(ctx, uuid)
	if err != nil {
		klog.Errorf("Failed to find VM by vmUUID %s: %v", uuid, err)
		return nil, err
	}
	return vm, nil
}

// findVMByName retrieves the VM with the given vm name using converged client.
func findVMByName(ctx context.Context, client *v4Converged.Client, vmName string) (*vmmModels.Vm, error) {
	klog.Infof("Checking if VM with name %q exists.", vmName)
	vms, err := client.VMs.List(ctx, converged.WithFilter(fmt.Sprintf("name eq '%s'", vmName)))
	if err != nil {
		return nil, fmt.Errorf("finding VM by name %s: %w", vmName, err)
	}
	if len(vms) == 0 {
		return nil, fmt.Errorf("VM with name %q not found", vmName)
	}
	if len(vms) > 1 {
		return nil, fmt.Errorf("found more than one (%d) VMs with name %s", len(vms), vmName)
	}
	return findVMByUUID(ctx, client, *vms[0].ExtId)
}

// deleteVM deletes a VM with the specified UUID using converged client.
func deleteVM(ctx context.Context, client *v4Converged.Client, vmUUID string) error {
	klog.Infof("Deleting VM with UUID %s.", vmUUID)
	op, err := client.VMs.DeleteAsync(ctx, vmUUID)
	if err != nil {
		klog.Errorf("Error deleting vm with uuid %s: %v", vmUUID, err)
		return err
	}
	_, err = op.Wait(ctx)
	if err != nil {
		if strings.Contains(strings.ToUpper(fmt.Sprint(err)), "NOT_FOUND") {
			klog.Infof("Successfully deleted vm with uuid %s", vmUUID)
			return nil
		}
		return err
	}
	return nil
}

// findClusterUuidByName retrieves the cluster uuid by the given cluster name using converged client.
func findClusterUuidByName(ctx context.Context, client *v4Converged.Client, clusterName string) (*string, error) {
	klog.Infof("Checking if cluster with name %q exists.", clusterName)
	clusters, err := client.Clusters.List(ctx, converged.WithFilter(fmt.Sprintf("name eq '%s'", clusterName)))
	if err != nil {
		return nil, fmt.Errorf("listing clusters: %w", err)
	}
	var match *string
	for i := range clusters {
		if clusters[i].Name != nil && *clusters[i].Name == clusterName {
			if match != nil {
				return nil, fmt.Errorf("found more than one cluster with name %q", clusterName)
			}
			match = clusters[i].ExtId
		}
	}
	if match == nil {
		return nil, fmt.Errorf("unable to find cluster by name %q", clusterName)
	}
	return match, nil
}

// findImageUuidByName retrieves the image uuid by the given image name using converged client.
func findImageUuidByName(ctx context.Context, client *v4Converged.Client, imageName string) (*string, error) {
	klog.Infof("Checking if image with name %q exists.", imageName)
	images, err := client.Images.List(ctx, converged.WithFilter(fmt.Sprintf("name eq '%s'", imageName)))
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("failed to find image by name %q", imageName)
	}
	if len(images) > 1 {
		return nil, fmt.Errorf("found more than one (%d) images with name %q", len(images), imageName)
	}
	return images[0].ExtId, nil
}

// findSubnetUuidByName retrieves the subnet uuid by the given subnet name using converged client.
func findSubnetUuidByName(ctx context.Context, client *v4Converged.Client, subnetName string) (*string, error) {
	klog.Infof("Checking if subnet with name %q exists.", subnetName)
	subnets, err := client.Subnets.List(ctx, converged.WithFilter(fmt.Sprintf("name eq '%s'", subnetName)))
	if err != nil {
		return nil, fmt.Errorf("listing subnets: %w", err)
	}
	if len(subnets) == 0 {
		return nil, fmt.Errorf("failed to find subnet by name %q", subnetName)
	}
	if len(subnets) > 1 {
		return nil, fmt.Errorf("found more than one (%d) subnets with name %q", len(subnets), subnetName)
	}
	return subnets[0].ExtId, nil
}

// findProjectUuidByName resolves a project name to its UUID using the v3 API.
func findProjectUuidByName(ctx context.Context, v3client *nutanixClientV3.Client, projectName string) (*string, error) {
	klog.Infof("Checking if project with name %q exists.", projectName)

	res, err := v3client.V3.ListProject(ctx, &nutanixClientV3.DSMetadata{
		Filter: ptr.To(fmt.Sprintf("name==%s", projectName)),
	})
	if err != nil || len(res.Entities) == 0 {
		return nil, fmt.Errorf("failed to find project by name %q: %w", projectName, err)
	}

	if len(res.Entities) > 1 {
		return nil, fmt.Errorf("found more than one (%v) projects with name %q", len(res.Entities), projectName)
	}

	return res.Entities[0].Metadata.UUID, nil
}

// getProjectByUUID validates that a project exists by UUID using the v3 API.
func getProjectByUUID(ctx context.Context, v3client *nutanixClientV3.Client, projectUUID string) (*nutanixClientV3.Project, error) {
	return v3client.V3.GetProject(ctx, projectUUID)
}

// getCategoryValue checks that a category key/value exists using converged client.
func getCategoryValue(ctx context.Context, client *v4Converged.Client, key, value string) (*prismModels.Category, error) {
	cats, err := client.Categories.List(ctx, converged.WithFilter(fmt.Sprintf("key eq '%s' and value eq '%s'", key, value)))
	if err != nil {
		return nil, err
	}
	switch len(cats) {
	case 0:
		return nil, fmt.Errorf("category %q with value %q not found", key, value)
	case 1:
		return &cats[0], nil
	default:
		return nil, fmt.Errorf("found more than one (%d) categories with key %q and value %q", len(cats), key, value)
	}
}

// getPrismCentralCluster returns the Prism Central cluster using converged client.
func getPrismCentralCluster(ctx context.Context, client *v4Converged.Client) (*clusterModels.Cluster, error) {
	clusters, err := client.Clusters.List(ctx)
	if err != nil {
		return nil, err
	}
	// Prefer cluster with PRISM_CENTRAL in services; otherwise return first if only one.
	found := clusterModels.Cluster{}
	for i := range clusters {
		if clusters[i].ExtId == nil || clusters[i].Config.ClusterFunction == nil || clusters[i].Config == nil ||
			clusters[i].Config.ClusterFunction[0] != clusterModels.CLUSTERFUNCTIONREF_PRISM_CENTRAL {
			continue
		}
		if reflect.DeepEqual(found, clusterModels.Cluster{}) {
			found = clusters[i]
		}
		break
	}
	if reflect.DeepEqual(found, clusterModels.Cluster{}) {
		return nil, fmt.Errorf("failed to retrieve Prism Central cluster")
	}
	return &found, nil
}
