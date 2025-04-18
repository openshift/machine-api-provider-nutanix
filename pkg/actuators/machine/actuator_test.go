package machine

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
)

func init() {
	// Add types to scheme
	machinev1beta1.AddToScheme(scheme.Scheme)
	machinev1.Install(scheme.Scheme)
	configv1.AddToScheme(scheme.Scheme)
}

func TestMachineEvents(t *testing.T) {

	g := NewWithT(t)
	ctx := context.Background()

	// Create the Infrastructure CR with the failureDomains configuration
	infra := getInfrastructureObject(true)
	g.Expect(k8sClient.Create(ctx, infra)).To(Succeed())
	defer func() {
		g.Expect(k8sClient.Delete(ctx, infra)).To(Succeed())
	}()
	g.Expect(infra.Spec.PlatformSpec.Nutanix.PrismCentral.Address).Should(Equal("10.40.42.15"))
	g.Expect(infra.Spec.PlatformSpec.Nutanix.PrismCentral.Port).Should(Equal(int32(9440)))
	g.Expect(infra.Spec.PlatformSpec.Nutanix.PrismElements).Should(HaveLen(1))
	g.Expect(infra.Spec.PlatformSpec.Nutanix.PrismElements[0].Name).Should(Equal("test-pe"))
	g.Expect(infra.Spec.PlatformSpec.Nutanix.PrismElements[0].Endpoint.Address).Should(Equal("10.40.131.30"))
	g.Expect(infra.Spec.PlatformSpec.Nutanix.PrismElements[0].Endpoint.Port).Should(Equal(int32(9440)))

	// Update the infrastructure status
	infra.Status = infrastructureStatus
	g.Expect(k8sClient.Status().Update(ctx, infra)).To(Succeed())

	testNsName := "test"
	testNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNsName,
		},
	}
	g.Expect(k8sClient.Create(ctx, testNs)).To(Succeed())
	defer func() {
		g.Expect(k8sClient.Delete(ctx, testNs)).To(Succeed())
	}()

	credsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultCredentialsSecretName,
			Namespace: testNsName,
		},
		Data: map[string][]byte{
			"credentials": []byte(credentialsData),
		},
	}
	g.Expect(k8sClient.Create(ctx, &credsSecret)).To(Succeed())
	defer func() {
		g.Expect(k8sClient.Delete(ctx, &credsSecret)).To(Succeed())
	}()

	userDataSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userDataSecretName,
			Namespace: testNsName,
		},
		Data: map[string][]byte{
			userDataSecretKey: []byte("{}"),
		},
	}
	g.Expect(k8sClient.Create(ctx, userDataSecret)).To(Succeed())
	defer func() {
		g.Expect(k8sClient.Delete(ctx, userDataSecret)).To(Succeed())
	}()

	cases := []struct {
		name         string
		machineName  string
		providerSpec *machinev1.NutanixMachineProviderConfig
		error        string
		operation    func(actuator *Actuator, machine *machinev1beta1.Machine)
		events       []string
	}{
		{
			name:         "Create machine failed on invalid machine scope",
			machineName:  "test-machine",
			providerSpec: getValidProviderSpec(""),
			operation: func(actuator *Actuator, machine *machinev1beta1.Machine) {
				actuator.Create(nil, machine)
			},
			events: []string{"context and machine should not be nil"},
		},
		{
			name:         "Create machine failed on missing required label",
			machineName:  "test-machine",
			providerSpec: getValidProviderSpec(""),
			operation: func(actuator *Actuator, machine *machinev1beta1.Machine) {
				machine.Labels[machinev1beta1.MachineClusterIDLabel] = ""
				actuator.Create(ctx, machine)
			},
			events: []string{"missing \"machine.openshift.io/cluster-api-cluster\" label"},
		},
		{
			name:        "Create machine failed on configuration errors",
			machineName: "test-machine",
			providerSpec: func() *machinev1.NutanixMachineProviderConfig {
				pspec := getValidProviderSpec("")
				pspec.Cluster.Type = "invalid-type"
				pspec.Image.Name = nil
				pspec.Subnets[0].Name = nil
				pspec.VCPUSockets = 0
				pspec.MemorySize = resource.MustParse("1.5Gi")
				pspec.SystemDiskSize = resource.MustParse("18Gi")
				pspec.BootType = "invalid-boottype"
				pspec.Project.Type = "uuid"
				dataDisk := machinev1.NutanixVMDisk{}
				dataDisk.DiskSize = resource.MustParse("500Mi")
				pspec.DataDisks = append(pspec.DataDisks, dataDisk)
				return pspec
			}(),
			operation: func(actuator *Actuator, machine *machinev1beta1.Machine) {
				actuator.Create(ctx, machine)
			},
			events: []string{
				"Invalid cluster identifier type",
				"Missing image name",
				"Missing subnet name",
				"The minimum vCPU sockets of the VM is 1",
				"The minimum memorySize is 2Gi bytes",
				"The minimum systemDiskSize is 20Gi bytes",
				"Invalid bootType, the valid bootType values are: \"\", \"Legacy\", \"UEFI\", \"SecureBoot\"",
				"Missing project uuid",
				"The minimum diskSize is 1Gi bytes.",
			},
		},
		{
			name:         "Create machine failed with invalid failure domain reference",
			machineName:  "test-machine",
			providerSpec: getValidProviderSpec("fd.invalid"),
			operation: func(actuator *Actuator, machine *machinev1beta1.Machine) {
				actuator.Create(ctx, machine)
			},
			events: []string{"Could not find the failureDomain with name \"fd.invalid\" configured in the Infrastructure resource."},
		},
		{
			name:         "Update machine failed on invalid machine scope",
			machineName:  "test-machine",
			providerSpec: getValidProviderSpec(""),
			operation: func(actuator *Actuator, machine *machinev1beta1.Machine) {
				actuator.Update(nil, machine)
			},
			events: []string{"context and machine should not be nil"},
		},
		{
			name:         "Update failed on missing required label",
			machineName:  "test-machine",
			providerSpec: getValidProviderSpec(""),
			operation: func(actuator *Actuator, machine *machinev1beta1.Machine) {
				machine.Labels[machinev1beta1.MachineClusterIDLabel] = ""
				actuator.Update(ctx, machine)
			},
			events: []string{"missing \"machine.openshift.io/cluster-api-cluster\" label"},
		},
		{
			name:         "Delete machine event failed on invalid machine scope",
			machineName:  "test-machine",
			providerSpec: getValidProviderSpec(""),
			operation: func(actuator *Actuator, machine *machinev1beta1.Machine) {
				actuator.Delete(nil, machine)
			},
			events: []string{"context and machine should not be nil"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			timeout := 30 * time.Second
			gs := NewWithT(t)

			providerSpec, err := RawExtensionFromNutanixMachineProviderSpec(tc.providerSpec)
			gs.Expect(err).ToNot(HaveOccurred())

			machine := &machinev1beta1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.machineName,
					Namespace: testNsName,
					Labels: map[string]string{
						machinev1beta1.MachineClusterIDLabel: "CLUSTERID",
					},
				},
				Spec: machinev1beta1.MachineSpec{
					ProviderSpec: machinev1beta1.ProviderSpec{
						Value: providerSpec,
					},
				},
				Status: machinev1beta1.MachineStatus{
					NodeRef: &v1.ObjectReference{
						Name: tc.machineName,
					},
				},
			}

			// Create the machine
			gs.Expect(k8sClient.Create(ctx, machine)).To(Succeed())
			defer func() {
				gs.Expect(k8sClient.Delete(ctx, machine)).To(Succeed())
			}()

			// Ensure the machine has synced to the cache
			getMachine := func() error {
				machineKey := types.NamespacedName{Namespace: machine.Namespace, Name: machine.Name}
				return k8sClient.Get(ctx, machineKey, &machinev1beta1.Machine{})
			}
			gs.Eventually(getMachine, timeout).Should(Succeed())

			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: tc.machineName,
					Labels: map[string]string{
						machinev1beta1.MachineClusterIDLabel: "CLUSTERID",
					},
				},
				Spec: v1.NodeSpec{},
				Status: v1.NodeStatus{
					VolumesAttached: []v1.AttachedVolume{},
				},
			}

			// Create the node
			gs.Expect(k8sClient.Create(ctx, node)).To(Succeed())
			defer func() {
				gs.Expect(k8sClient.Delete(ctx, node)).To(Succeed())
			}()

			// Ensure the node has synced to the cache
			getNode := func() error {
				nodeKey := types.NamespacedName{Name: node.Name}
				return k8sClient.Get(ctx, nodeKey, &v1.Node{})
			}
			gs.Eventually(getNode, timeout).Should(Succeed())

			params := ActuatorParams{
				Client:        k8sClient,
				EventRecorder: eventRecorder,
			}

			actuator := NewActuator(params)
			tc.operation(actuator, machine)

			eventList := &v1.EventList{}
			waitForEvent := func() error {
				err := k8sClient.List(ctx, eventList, client.InNamespace(machine.Namespace))
				if err != nil {
					return err
				}

				if len(eventList.Items) != 1 {
					return fmt.Errorf("expected len 1, got %d", len(eventList.Items))
				}

				if eventList.Items[0].Count != 1 {
					return fmt.Errorf("expected event %v to happen only once", eventList.Items[0].Name)
				}
				return nil
			}

			gs.Eventually(waitForEvent, timeout).Should(Succeed())

			for _, msg := range tc.events {
				gs.Expect(eventList.Items[0].Message).To(ContainSubstring(msg))
			}

			for i := range eventList.Items {
				gs.Expect(k8sClient.Delete(ctx, &eventList.Items[i])).To(Succeed())
			}
		})
	}
}
