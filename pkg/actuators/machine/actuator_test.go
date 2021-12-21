package machine

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nutanixv1 "github.com/nutanix-cloud-native/machine-api-provider-nutanix/pkg/apis/nutanixprovider/v1beta1"
	clientpkg "github.com/nutanix-cloud-native/machine-api-provider-nutanix/pkg/client"
)

func init() {
	// Add types to scheme
	machinev1.AddToScheme(scheme.Scheme)
	configv1.AddToScheme(scheme.Scheme)
}

func TestMachineEvents(t *testing.T) {

	g := NewWithT(t)
	ctx := context.Background()

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

	endpoint, err := base64.StdEncoding.DecodeString("MTAuNDAuMTQyLjE1")
	g.Expect(err).ToNot(HaveOccurred())
	port, err := base64.StdEncoding.DecodeString("OTQ0MA==")
	g.Expect(err).ToNot(HaveOccurred())
	user, err := base64.StdEncoding.DecodeString("YWRtaW4=")
	g.Expect(err).ToNot(HaveOccurred())
	password, err := base64.StdEncoding.DecodeString("TnV0YW5peC4xMjM=")
	g.Expect(err).ToNot(HaveOccurred())

	credsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultNutanixCredsSecretName,
			Namespace: testNsName,
		},
		Data: map[string][]byte{
			clientpkg.NutanixEndpointKey: endpoint,
			clientpkg.NutanixPortKey:     port,
			clientpkg.NutanixUserKey:     user,
			clientpkg.NutanixPasswordKey: password,
		},
	}
	g.Expect(k8sClient.Create(ctx, &credsSecret)).To(Succeed())
	defer func() {
		g.Expect(k8sClient.Delete(ctx, &credsSecret)).To(Succeed())
	}()

	infra := &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: globalInfrastuctureName,
		},
		Spec: configv1.InfrastructureSpec{
			PlatformSpec: configv1.PlatformSpec{
				Type:      configv1.BareMetalPlatformType,
				BareMetal: &configv1.BareMetalPlatformSpec{},
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, infra)).To(Succeed())
	defer func() {
		g.Expect(k8sClient.Delete(ctx, infra)).To(Succeed())
	}()

	userDataSecretName := "nutanix-userdata"
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

	var vmUuid *string
	cases := []struct {
		name        string
		machineName string
		error       string
		operation   func(actuator *Actuator, machine *machinev1.Machine)
		event       string
	}{
		{
			name:        "Create machine failed on invalid machine scope",
			machineName: "test-machine",
			operation: func(actuator *Actuator, machine *machinev1.Machine) {
				actuator.Create(nil, machine)
			},
			event: "context and machine should not be nil",
		},
		{
			name:        "Create machine failed on missing required label",
			machineName: "test-machine",
			operation: func(actuator *Actuator, machine *machinev1.Machine) {
				machine.Labels[machinev1.MachineClusterIDLabel] = ""
				actuator.Create(ctx, machine)
			},
			event: "missing \"machine.openshift.io/cluster-api-cluster\" label",
		},
		{
			name:        "Create machine succeed",
			machineName: "test-machine",
			operation: func(actuator *Actuator, machine *machinev1.Machine) {
				actuator.Create(ctx, machine)
				providerStatus, err1 := nutanixv1.ProviderStatusFromRawExtension(machine.Status.ProviderStatus)
				g.Expect(err1).ToNot(HaveOccurred())
				vmUuid = providerStatus.VmUUID
				g.Expect(vmUuid).NotTo(BeNil())
				g.Expect(machine.Spec.ProviderID).NotTo(BeNil())
				g.Expect(machine.Status.Addresses).To(HaveLen(2))
			},
			event: "Created Machine",
		},
		{
			name:        "Update machine failed on invalid machine scope",
			machineName: "test-machine",
			operation: func(actuator *Actuator, machine *machinev1.Machine) {
				actuator.Update(nil, machine)
			},
			event: "context and machine should not be nil",
		},
		{
			name:        "Update failed on missing required label",
			machineName: "test-machine",
			operation: func(actuator *Actuator, machine *machinev1.Machine) {
				machine.Labels[machinev1.MachineClusterIDLabel] = ""
				actuator.Update(ctx, machine)
			},
			event: "missing \"machine.openshift.io/cluster-api-cluster\" label",
		},
		{
			name:        "Update machine succeed",
			machineName: "test-machine",
			operation: func(actuator *Actuator, machine *machinev1.Machine) {
				g.Expect(vmUuid).NotTo(BeNil())
				providerStatus, err1 := nutanixv1.ProviderStatusFromRawExtension(machine.Status.ProviderStatus)
				g.Expect(err1).ToNot(HaveOccurred())
				providerStatus.VmUUID = vmUuid
				rawProviderStatus, err1 := nutanixv1.RawExtensionFromProviderStatus(providerStatus)
				g.Expect(err1).ToNot(HaveOccurred())
				machine.Status.ProviderStatus = rawProviderStatus
				actuator.Update(ctx, machine)
			},
			event: "Updated Machine",
		},
		{
			name:        "Delete machine event failed on invalid machine scope",
			machineName: "test-machine",
			operation: func(actuator *Actuator, machine *machinev1.Machine) {
				actuator.Delete(nil, machine)
			},
			event: "context and machine should not be nil",
		},
		{
			name:        "Delete machine succeed",
			machineName: "test-machine",
			operation: func(actuator *Actuator, machine *machinev1.Machine) {
				g.Expect(vmUuid).NotTo(BeNil())
				providerStatus, err1 := nutanixv1.ProviderStatusFromRawExtension(machine.Status.ProviderStatus)
				g.Expect(err1).ToNot(HaveOccurred())
				providerStatus.VmUUID = vmUuid
				rawProviderStatus, err1 := nutanixv1.RawExtensionFromProviderStatus(providerStatus)
				g.Expect(err1).ToNot(HaveOccurred())
				machine.Status.ProviderStatus = rawProviderStatus
				actuator.Delete(ctx, machine)
			},
			event: "Deleted machine",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			timeout := 30 * time.Second
			gs := NewWithT(t)

			providerSpec, err := nutanixv1.RawExtensionFromProviderSpec(&nutanixv1.NutanixMachineProviderConfig{
				ClusterReferenceUuid: "0005b0f1-8f43-a0f2-02b7-3cecef193712",
				ImageName:            "rhcos-4.10-openstack",
				SubnetUUID:           "c7938dc6-7659-453e-a688-e26020c68e43",
				NumVcpusPerSocket:    2,
				NumSockets:           1,
				MemorySizeMib:        4096,
				DiskSizeMib:          122880,
				PowerState:           "ON",
				UserDataSecret: &corev1.LocalObjectReference{
					Name: userDataSecretName,
				},
			})
			gs.Expect(err).ToNot(HaveOccurred())

			machine := &machinev1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.machineName,
					Namespace: testNsName,
					Labels: map[string]string{
						machinev1.MachineClusterIDLabel: "CLUSTERID",
					},
				},
				Spec: machinev1.MachineSpec{
					ProviderSpec: machinev1.ProviderSpec{
						Value: providerSpec,
					},
				},
				Status: machinev1.MachineStatus{
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
				return k8sClient.Get(ctx, machineKey, &machinev1.Machine{})
			}
			gs.Eventually(getMachine, timeout).Should(Succeed())

			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: tc.machineName,
					Labels: map[string]string{
						machinev1.MachineClusterIDLabel: "CLUSTERID",
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

			gs.Expect(eventList.Items[0].Message).To(ContainSubstring(tc.event))

			for i := range eventList.Items {
				gs.Expect(k8sClient.Delete(ctx, &eventList.Items[i])).To(Succeed())
			}
		})
	}
}
