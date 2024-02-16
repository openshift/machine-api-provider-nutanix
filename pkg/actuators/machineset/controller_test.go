/*
Copyright 2021 Nutanix Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package machineset

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gtypes "github.com/onsi/gomega/types"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var _ = Describe("MachineSet Reconciler", func() {
	var c client.Client
	var stopMgr context.CancelFunc
	var fakeRecorder *record.FakeRecorder
	var namespace *corev1.Namespace

	BeforeEach(func() {
		mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
		Expect(err).ToNot(HaveOccurred())

		r := Reconciler{
			Client: mgr.GetClient(),
			Log:    log.Log,
		}
		Expect(r.SetupWithManager(mgr, controller.Options{})).To(Succeed())

		fakeRecorder = record.NewFakeRecorder(1)
		r.recorder = fakeRecorder

		c = mgr.GetClient()
		stopMgr = StartTestManager(mgr)

		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "mhc-test-"}}
		Expect(c.Create(ctx, namespace)).To(Succeed())
	})

	AfterEach(func() {
		Expect(deleteMachineSets(c, namespace.Name)).To(Succeed())
		stopMgr()
	})

	type reconcileTestCase = struct {
		numVcpusPerSocket   int32
		numSockets          int32
		memorySize          resource.Quantity
		existingAnnotations map[string]string
		expectedAnnotations map[string]string
		expectedEvents      []string
	}

	DescribeTable("when reconciling MachineSets", func(rtc reconcileTestCase) {
		machineSet, err := newTestMachineSet(namespace.Name, rtc.numVcpusPerSocket,
			rtc.numSockets, rtc.memorySize, rtc.existingAnnotations)
		Expect(err).ToNot(HaveOccurred())

		Expect(c.Create(ctx, machineSet)).To(Succeed())

		Eventually(func() map[string]string {
			m := &machinev1beta1.MachineSet{}
			key := client.ObjectKey{Namespace: machineSet.Namespace, Name: machineSet.Name}
			err := c.Get(ctx, key, m)
			if err != nil {
				return nil
			}
			annotations := m.GetAnnotations()
			if annotations != nil {
				return annotations
			}
			// Return an empty map to distinguish between empty annotations and errors
			return make(map[string]string)
		}, timeout).Should(Equal(rtc.expectedAnnotations))

		// Check which event types were sent
		Eventually(fakeRecorder.Events, timeout).Should(HaveLen(len(rtc.expectedEvents)))
		receivedEvents := []string{}
		eventMatchers := []gtypes.GomegaMatcher{}
		for _, ev := range rtc.expectedEvents {
			receivedEvents = append(receivedEvents, <-fakeRecorder.Events)
			eventMatchers = append(eventMatchers, ContainSubstring(fmt.Sprintf(" %s ", ev)))
		}
		Expect(receivedEvents).To(ConsistOf(eventMatchers))
	},
		Entry("with 2cpu 8memory", reconcileTestCase{
			numVcpusPerSocket:   2,
			numSockets:          1,
			memorySize:          parseQuantity("8192Mi"),
			existingAnnotations: make(map[string]string),
			expectedAnnotations: map[string]string{
				cpuKey:    "2",
				memoryKey: "8192",
			},
			expectedEvents: []string{},
		}),
		Entry("with 4cpu 16memory", reconcileTestCase{
			numVcpusPerSocket:   4,
			numSockets:          1,
			memorySize:          parseQuantity("16Gi"),
			existingAnnotations: make(map[string]string),
			expectedAnnotations: map[string]string{
				cpuKey:    "4",
				memoryKey: "16384",
			},
			expectedEvents: []string{},
		}),
	)
})

func deleteMachineSets(c client.Client, namespaceName string) error {
	machineSets := &machinev1beta1.MachineSetList{}
	err := c.List(ctx, machineSets, client.InNamespace(namespaceName))
	if err != nil {
		return err
	}

	for _, ms := range machineSets.Items {
		err := c.Delete(ctx, &ms)
		if err != nil {
			return err
		}
	}

	Eventually(func() error {
		machineSets := &machinev1beta1.MachineSetList{}
		err := c.List(ctx, machineSets)
		if err != nil {
			return err
		}
		if len(machineSets.Items) > 0 {
			return fmt.Errorf("machineSets not deleted")
		}
		return nil
	}, timeout).Should(Succeed())

	return nil
}

func TestReconcile(t *testing.T) {
	testCases := []struct {
		name                string
		numVcpusPerSocket   int32
		numSockets          int32
		memorySize          resource.Quantity
		existingAnnotations map[string]string
		expectedAnnotations map[string]string
		expectErr           bool
	}{
		{
			name:                "with 2cpu 8memory",
			numVcpusPerSocket:   2,
			numSockets:          1,
			memorySize:          parseQuantity("8192Mi"),
			existingAnnotations: make(map[string]string),
			expectedAnnotations: map[string]string{
				cpuKey:    "2",
				memoryKey: "8192",
			},
			expectErr: false,
		},
		{
			name:                "with 4cpu 16memory",
			numVcpusPerSocket:   4,
			numSockets:          1,
			memorySize:          parseQuantity("16Gi"),
			existingAnnotations: make(map[string]string),
			expectedAnnotations: map[string]string{
				cpuKey:    "4",
				memoryKey: "16384",
			},
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			g := NewWithT(tt)

			machineSet, err := newTestMachineSet("default", tc.numVcpusPerSocket,
				tc.numSockets, tc.memorySize, tc.existingAnnotations)
			g.Expect(err).ToNot(HaveOccurred())

			_, err = reconcile(machineSet)
			g.Expect(err != nil).To(Equal(tc.expectErr))
			g.Expect(machineSet.Annotations).To(Equal(tc.expectedAnnotations))
		})
	}
}

func parseQuantity(str string) resource.Quantity {
	q, err := resource.ParseQuantity(str)
	if err != nil {
		klog.Fatal(err)
	}

	return q
}

func newTestMachineSet(namespace string, numVcpusPerSocket, numSockets int32, memorySize resource.Quantity,
	existingAnnotations map[string]string) (*machinev1beta1.MachineSet, error) {

	// Copy anntotations map so we don't modify the input
	annotations := make(map[string]string)
	for k, v := range existingAnnotations {
		annotations[k] = v
	}

	machineProviderSpec := &machinev1.NutanixMachineProviderConfig{
		VCPUsPerSocket: numVcpusPerSocket,
		VCPUSockets:    numSockets,
		MemorySize:     memorySize,
	}
	providerSpec, err := providerSpecFromMachine(machineProviderSpec)
	if err != nil {
		return nil, err
	}

	replicas := int32(1)
	return &machinev1beta1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations:  annotations,
			GenerateName: "test-machineset-",
			Namespace:    namespace,
		},
		Spec: machinev1beta1.MachineSetSpec{
			Replicas: &replicas,
			Template: machinev1beta1.MachineTemplateSpec{
				Spec: machinev1beta1.MachineSpec{
					ProviderSpec: providerSpec,
				},
			},
		},
	}, nil
}

func providerSpecFromMachine(in *machinev1.NutanixMachineProviderConfig) (machinev1beta1.ProviderSpec, error) {
	bytes, err := json.Marshal(in)
	if err != nil {
		return machinev1beta1.ProviderSpec{}, err
	}
	return machinev1beta1.ProviderSpec{
		Value: &runtime.RawExtension{Raw: bytes},
	}, nil
}
