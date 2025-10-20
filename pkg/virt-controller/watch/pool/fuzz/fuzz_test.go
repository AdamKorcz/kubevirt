/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright The KubeVirt Authors.
 *
 */

package fuzz

import (
	"testing"

	fuzz "github.com/google/gofuzz"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	v1 "kubevirt.io/api/core/v1"
	poolv1 "kubevirt.io/api/pool/v1alpha1"
	"kubevirt.io/client-go/kubecli"
	kubevirtfake "kubevirt.io/client-go/kubevirt/fake"

	virtcontroller "kubevirt.io/kubevirt/pkg/controller"
	"kubevirt.io/kubevirt/pkg/testutils"
	"kubevirt.io/kubevirt/pkg/virt-controller/watch/pool"
)

var (
	maxResources = 3
)

// FuzzExecute adds random resources to the context
// and then runs the controller.
func FuzzExecute(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte, numberOfCRs, numberOfVMs uint8) {
		fdp := fuzz.NewFromGoFuzz(data)

		crs := make([]*appsv1.ControllerRevision, 0)
		for _ = range int(numberOfCRs) % maxResources {
			var cr *appsv1.ControllerRevision
			fdp.Fuzz(cr)
			crs = append(crs, cr)
		}

		vms := make([]*v1.VirtualMachine, 0)
		for _ = range int(numberOfVMs) % maxResources {
			var vm *v1.VirtualMachine
			fdp.Fuzz(vm)
			vms = append(vms, vm)
		}

		vmis := make([]*v1.VirtualMachineInstance, 0)
		for _ = range int(numberOfVMs) % maxResources {
			var vmi *v1.VirtualMachineInstance
			fdp.Fuzz(vmi)
			vmis = append(vmis, vmi)
		}

		vmPools := make([]*poolv1.VirtualMachinePool, 0)
		for _ = range int(numberOfVMs) % maxResources {
			var vmPool *poolv1.VirtualMachinePool
			fdp.Fuzz(vmPool)
			vmPools = append(vmPools, vmPool)
		}
		// There is no point in continuing
		// if we have not created any resources.
		if len(vms)+len(vms)+len(vmis)+len(vmPools) < 3 {
			return
		}

		virtClient := kubecli.NewMockKubevirtClient(gomock.NewController(t))

		vmiInformer, _ := testutils.NewFakeInformerFor(&v1.VirtualMachineInstance{})
		vmInformer, _ := testutils.NewFakeInformerFor(&v1.VirtualMachine{})
		poolInformer, _ := testutils.NewFakeInformerFor(&poolv1.VirtualMachinePool{})
		recorder := record.NewFakeRecorder(100)
		recorder.IncludeObject = true

		crInformer, _ := testutils.NewFakeInformerWithIndexersFor(&appsv1.ControllerRevision{}, cache.Indexers{
			"vmpool": func(obj interface{}) ([]string, error) {
				cr := obj.(*appsv1.ControllerRevision)
				for _, ref := range cr.OwnerReferences {
					if ref.Kind == "VirtualMachinePool" {
						return []string{string(ref.UID)}, nil
					}
				}
				return nil, nil
			},
		})

		controller, _ := pool.NewController(virtClient,
			vmiInformer,
			vmInformer,
			poolInformer,
			crInformer,
			recorder,
			uint(10))
		fakeVirtClient := kubevirtfake.NewSimpleClientset()
		mockQueue := testutils.NewMockWorkQueue(pool.GetQueue(controller))
		// Add the resources to the context
		for _, cr := range crs {
			if cr == nil {
				continue
			}
			crInformer.GetIndexer().Add(cr)
			key, err := virtcontroller.KeyFunc(cr)
			if err != nil {
				return
			}
			mockQueue.Add(key)
		}
		for _, vm := range vms {
			if vm == nil {
				continue
			}
			vmInformer.GetIndexer().Add(vm)
			key, err := virtcontroller.KeyFunc(vm)
			if err != nil {
				return
			}
			mockQueue.Add(key)
		}
		for _, vmi := range vmis {
			if vmi == nil {
				continue
			}
			vmiInformer.GetStore().Add(vmi)
			key, err := virtcontroller.KeyFunc(vmi)
			if err != nil {
				return
			}
			mockQueue.Add(key)
		}
		for _, vmPool := range vmPools {
			if vmPool == nil {
				continue
			}
			poolInformer.GetIndexer().Add(vmPool)
			key, err := virtcontroller.KeyFunc(vmPool)
			if err != nil {
				return
			}
			mockQueue.Add(key)
			virtClient.EXPECT().VirtualMachinePool(vmPool.Namespace).Return(fakeVirtClient.PoolV1alpha1().VirtualMachinePools(vmPool.Namespace)).AnyTimes()

		}
		if mockQueue.Len() == 0 {
			return
		}
		pool.ShutdownCtrlQueue(controller)
		pool.SetQueue(controller, mockQueue)

		// Set up mock client
		virtClient.EXPECT().VirtualMachineInstance(metav1.NamespaceDefault).Return(fakeVirtClient.KubevirtV1().VirtualMachineInstances(metav1.NamespaceDefault)).AnyTimes()
		virtClient.EXPECT().VirtualMachine(metav1.NamespaceDefault).Return(fakeVirtClient.KubevirtV1().VirtualMachines(metav1.NamespaceDefault)).AnyTimes()

		fakeVirtClient.Fake.PrependReactor("*", "*", func(action k8sTesting.Action) (handled bool, obj runtime.Object, err error) {
			return true, nil, nil
		})

		k8sClient := k8sfake.NewSimpleClientset()
		k8sClient.Fake.PrependReactor("*", "*", func(action k8sTesting.Action) (handled bool, obj runtime.Object, err error) {
			return true, nil, nil
		})
		virtClient.EXPECT().AppsV1().Return(k8sClient.AppsV1()).AnyTimes()

		// Run the controller
		controller.Execute()
	})
}
