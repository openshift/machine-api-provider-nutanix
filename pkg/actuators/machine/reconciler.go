package machine

import (
	"fmt"
	"strings"
	"time"

	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/pkg/nutanix/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"
	"github.com/openshift/machine-api-operator/pkg/metrics"
)

const (
	requeueAfterSeconds      = 20
	requeueAfterFatalSeconds = 180
	masterLabel              = "node-role.kubernetes.io/master"

	providerIDFormat = "nutanix://%s"

	// MachineInstancePowerStateAnnotationName as annotation name for a machine instance power state
	MachineInstancePowerStateAnnotationName = "machine.openshift.io/instance-power-state"
)

// Reconciler runs the logic to reconciles a machine resource towards its desired state
type Reconciler struct {
	*machineScope
}

func newReconciler(scope *machineScope) *Reconciler {
	return &Reconciler{
		machineScope: scope,
	}
}

// create creates machine if it does not exists.
func (r *Reconciler) create() error {
	klog.Infof("%s: creating machine", r.machine.Name)

	if err := validateMachine(*r.machine); err != nil {
		e1 := fmt.Errorf("%v: failed validating machine provider spec: %w", r.machine.GetName(), err)
		klog.Error(e1.Error())
		return e1
	}

	userData, err := r.machineScope.getUserData()
	if err != nil {
		return fmt.Errorf("failed to get user data: %w", err)
	}

	if errList := validateVMConfig(r.machineScope); len(errList) > 0 {
		machineErr := machinecontroller.InvalidMachineConfiguration(
			"%v: failed in validating machine providerSpec: %v",
			r.machine.GetName(), errList.ToAggregate().Error())
		return machineErr
	}

	vm, err := createVM(r.machineScope, userData)
	if err != nil {
		klog.Errorf("%s: error creating machine vm. error: %v", r.machine.Name, err)
		r.machineScope.setProviderStatus(nil, conditionFailed(machineCreation, err.Error()))
		return fmt.Errorf("failed to create VM: %w", err)
	}

	klog.Infof("Created VM %q, with vm uuid: %s", r.machine.Name, *vm.Metadata.UUID)
	if err = r.updateMachineWithVMState(vm); err != nil {
		return fmt.Errorf("failed to update machine with vm state: %w", err)
	}

	r.machineScope.setProviderStatus(vm, conditionSuccess(machineCreation))

	return nil
}

// update finds a vm and reconciles the machine resource status against it.
func (r *Reconciler) update() error {
	klog.Infof("%s: updating machine", r.machine.Name)

	err := validateMachine(*r.machine)
	if err != nil {
		return fmt.Errorf("%v: failed validating machine provider spec: %w", r.machine.GetName(), err)
	}

	var vm *nutanixClientV3.VMIntentResponse
	if r.providerStatus.VmUUID == nil {
		// Try to find the vm by name
		vm, err = findVMByName(r.nutanixClient, r.machine.Name)
		if err != nil {
			metrics.RegisterFailedInstanceUpdate(&metrics.MachineLabels{
				Name:      r.machine.Name,
				Namespace: r.machine.Namespace,
				Reason:    err.Error(),
			})
			klog.Errorf("%s: error finding the vm with name %q. error: %v", r.machine.Name, r.machine.Name, err)

			r.machineScope.setProviderStatus(nil, conditionFailed(machineUpdate, err.Error()))
			return err
		}
		r.providerStatus.VmUUID = vm.Metadata.UUID

	} else {
		// find the existing VM with the vmUuid
		vmUuid := *r.providerStatus.VmUUID
		vm, err = findVMByUUID(r.nutanixClient, vmUuid)
		if err != nil {
			metrics.RegisterFailedInstanceUpdate(&metrics.MachineLabels{
				Name:      r.machine.Name,
				Namespace: r.machine.Namespace,
				Reason:    err.Error(),
			})
			klog.Errorf("%s: error finding the vm with uuid %s. error: %v", r.machine.Name, vmUuid, err)

			r.machineScope.setProviderStatus(nil, conditionFailed(machineUpdate, err.Error()))
			return err
		}
	}

	if err = r.updateMachineWithVMState(vm); err != nil {
		metrics.RegisterFailedInstanceUpdate(&metrics.MachineLabels{
			Name:      r.machine.Name,
			Namespace: r.machine.Namespace,
			Reason:    err.Error(),
		})
		klog.Errorf("%s: error update machine with VM state. error: %v", r.machine.Name, err)

		r.machineScope.setProviderStatus(vm, conditionFailed(machineUpdate, err.Error()))
		return err
	}

	r.machineScope.setProviderStatus(vm, conditionSuccess(machineUpdate))

	klog.Infof("%s: updated machine vm state", r.machine.Name)
	return nil
}

// delete deletes VM
func (r *Reconciler) delete() error {
	klog.Infof("%s: deleting machine's vm", r.machine.Name)

	var err error
	var vm *nutanixClientV3.VMIntentResponse

	// Check if the machine vm exists
	if r.providerStatus.VmUUID != nil {
		// Try to find the vm by uuid
		vm, err = findVMByUUID(r.nutanixClient, *r.providerStatus.VmUUID)
	} else {
		// Try to find the vm by name
		vm, err = findVMByName(r.nutanixClient, r.machine.Name)
	}
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			// Not found the machine vm, could be deleted or not created yet.
			klog.Warningf("%s: the machine vm does not exist", r.machine.Name)
			return nil
		}

		klog.Errorf("%s: error finding the machine vm. %v", r.machine.Name, err)
		return err
	}

	vmUuid := *vm.Metadata.UUID

	// Ensure volumes are detached before deleting the Node.
	if r.isNodeLinked() {
		attached, err := r.nodeHasVolumesAttached()
		if err != nil {
			return fmt.Errorf("failed to determine if node %s has attached volumes: %w", r.machine.Status.NodeRef.Name, err)
		}
		if attached {
			return &machinecontroller.RequeueAfterError{RequeueAfter: 20 * time.Second}
		}
	}

	// Delete vm with the vmUuid
	err = deleteVM(r.nutanixClient, vmUuid)
	if err != nil {
		metrics.RegisterFailedInstanceDelete(&metrics.MachineLabels{
			Name:      r.machine.Name,
			Namespace: r.machine.Namespace,
			Reason:    err.Error(),
		})
		klog.Errorf("%s: error deleting vm with uuid %s. error: %v", r.machine.Name, vmUuid, err)
		return err
	}

	// update machine spec and status
	r.machine.Spec.ProviderID = nil
	r.machine.Status.Addresses = r.machine.Status.Addresses[:0]

	klog.Infof("%s: deleted machine's vm with uuid %s", r.machine.Name, vmUuid)
	return nil
}

// exists returns true if machine corresponding VM exists.
func (r *Reconciler) exists() (bool, error) {
	err := validateMachine(*r.machine)
	if err != nil {
		return false, fmt.Errorf("%s: failed validating machine provider spec. %w", r.machine.GetName(), err)
	}

	if r.providerStatus.VmUUID != nil {
		// Try to find the vm by uuid
		vmUuid := *r.providerStatus.VmUUID
		_, err = findVMByUUID(r.nutanixClient, vmUuid)
	} else {
		// Try to find the vm by name
		_, err = findVMByName(r.nutanixClient, r.machine.Name)
	}

	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") {
			return false, nil
		}

		metrics.RegisterFailedInstanceUpdate(&metrics.MachineLabels{
			Name:      r.machine.Name,
			Namespace: r.machine.Namespace,
			Reason:    err.Error(),
		})
		klog.Errorf("%s: error finding the vm: %w", r.machine.Name, err)
		return false, err
	}

	klog.Infof("%s: vm exists", r.machine.Name)
	return true, nil
}

// isMaster returns true if the machine is part of a cluster's control plane
func (r *Reconciler) isMaster() (bool, error) {
	if r.machine.Status.NodeRef == nil {
		klog.Errorf("NodeRef not found in machine %q", r.machine.Name)
		return false, nil
	}
	node := &corev1.Node{}
	nodeKey := types.NamespacedName{
		Name: r.machine.Status.NodeRef.Name,
	}

	err := r.client.Get(r.Context, nodeKey, node)
	if err != nil {
		return false, fmt.Errorf("failed to get node from machine %s", r.machine.Name)
	}

	if _, exists := node.Labels[masterLabel]; exists {
		return true, nil
	}
	return false, nil
}

// setProviderID adds providerID in the machine spec
func (r *Reconciler) setProviderID(vmUUID *string) error {
	if vmUUID == nil {
		return fmt.Errorf("%s: Failed to update machine providerID: null vmUUID", r.machine.Name)
	}

	// update the machine.Spec.ProviderID
	existingProviderID := r.machine.Spec.ProviderID
	providerID := fmt.Sprintf(providerIDFormat, *vmUUID)
	if existingProviderID != nil && *existingProviderID == providerID {
		klog.Infof("%s: ProviderID already set in the machine Spec with value: %s", r.machine.Name, *existingProviderID)
	} else {
		r.machine.Spec.ProviderID = &providerID
		klog.Infof("%s: ProviderID set at machine.spec: %s", r.machine.Name, providerID)
	}

	// update the corresponding node.Spec.ProviderID
	var nodeName string
	if r.machine.Status.NodeRef != nil {
		nodeName = r.machine.Status.NodeRef.Name
	}
	if len(nodeName) == 0 {
		nodeName = r.machine.Name
	}
	nodeKey := types.NamespacedName{Name: nodeName}
	node := &corev1.Node{}
	err := r.client.Get(r.Context, nodeKey, node)
	if err != nil {
		return fmt.Errorf("%s: failed to get node %s: %w", r.machine.Name, nodeName, err)
	}

	existingNodeProviderID := node.Spec.ProviderID
	if existingNodeProviderID == providerID {
		klog.Infof("%s: The node %q spec.providerID is already set with value: %s", r.machine.Name, nodeName, existingNodeProviderID)
	} else {
		node.Spec.ProviderID = providerID
		err := r.client.Update(r.Context, node)
		if err != nil {
			klog.Errorf("%s: failed to update the node %q spec.providerID. error: %v", r.machine.Name, nodeName, err)
			return err
		}
		klog.Infof("%s: The node %q spec.providerID is set to: %s", r.machine.Name, nodeName, providerID)
	}

	return nil
}

func (r *Reconciler) updateMachineWithVMState(vm *nutanixClientV3.VMIntentResponse) error {
	if vm == nil {
		return nil
	}

	klog.Infof("%s: updating machine providerID", r.machine.Name)
	if err := r.setProviderID(vm.Metadata.UUID); err != nil {
		return err
	}

	vmType := stringPointerDeref(vm.Status.Resources.HypervisorType)
	vmState := stringPointerDeref(vm.Status.State)
	powerState := stringPointerDeref(vm.Status.Resources.PowerState)
	if r.machine.Annotations == nil {
		r.machine.Annotations = map[string]string{}
	}
	r.machine.Annotations[machinecontroller.MachineInstanceTypeLabelName] = vmType
	r.machine.Annotations[machinecontroller.MachineInstanceStateAnnotationName] = vmState
	r.machine.Annotations[MachineInstancePowerStateAnnotationName] = powerState
	klog.Infof("%s: updated machine instance state annotations (%s: %s), (%s: %s), (%s: %s)", r.machine.Name,
		machinecontroller.MachineInstanceTypeLabelName, vmType,
		machinecontroller.MachineInstanceStateAnnotationName, vmState,
		MachineInstancePowerStateAnnotationName, powerState)

	return nil
}
