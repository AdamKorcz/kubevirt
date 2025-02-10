package pool

import (
	"testing"

	gfh "github.com/AdaLogics/go-fuzz-headers"
	"github.com/golang/mock/gomock"
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
)

var (
	maxResources = 3
)

// FuzzExecute add up to 3 XXXXXXXXXXXXXXX
// to the context and then runs the controller.
func FuzzExecute(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte, numberOfCRs, numberOfVMs uint8) {
		fdp := gfh.NewConsumer(data)

		crs := make([]*appsv1.ControllerRevision, 0)
		for _ = range int(numberOfCRs) % maxResources {
			cr := &appsv1.ControllerRevision{}
			err := fdp.GenerateStruct(cr)
			if err != nil {
				return
			}
			crs = append(crs, cr)
		}

		vms := make([]*v1.VirtualMachine, 0)
		for _ = range int(numberOfVMs) % maxResources {
			vm := &v1.VirtualMachine{}
			err := fdp.GenerateStruct(vm)
			if err != nil {
				return
			}
			vms = append(vms, vm)
		}

		vmis := make([]*v1.VirtualMachineInstance, 0)
		for _ = range int(numberOfVMs) % maxResources {
			vmi := &v1.VirtualMachineInstance{}
			err := fdp.GenerateStruct(vmi)
			if err != nil {
				return
			}
			vmis = append(vmis, vmi)
		}

		vmPools := make([]*poolv1.VirtualMachinePool, 0)
		for _ = range int(numberOfVMs) % maxResources {
			vmPool := &poolv1.VirtualMachinePool{}
			err := fdp.GenerateStruct(vmPool)
			if err != nil {
				return
			}
			vmPools = append(vmPools, vmPool)
		}
		// There is no point in continuing
		// if we have not created any resources.
		/*if len(vms) == 0 || len(vms) == 0 || len(vmis) == 0 || len(vmPools) == 0 {
			return
		}*/

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

		controller, _ := NewController(virtClient,
			vmiInformer,
			vmInformer,
			poolInformer,
			crInformer,
			recorder,
			uint(10))
		// Wrap our workqueue to have a way to detect when we are done processing updates
		mockQueue := testutils.NewMockWorkQueue(controller.queue)
		controller.queue = mockQueue

		fakeVirtClient := kubevirtfake.NewSimpleClientset()

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

		// Add the resources to the context
		for _, cr := range crs {
			controller.revisionIndexer.Add(cr)
			key, err := virtcontroller.KeyFunc(cr)
			if err != nil {
				return
			}
			mockQueue.Add(key)
		}
		for _, vm := range vms {
			controller.vmIndexer.Add(vm)
			key, err := virtcontroller.KeyFunc(vm)
			if err != nil {
				return
			}
			mockQueue.Add(key)
		}
		for _, vmi := range vmis {
			controller.vmiStore.Add(vmi)
			key, err := virtcontroller.KeyFunc(vmi)
			if err != nil {
				return
			}
			mockQueue.Add(key)
		}
		for _, vmPool := range vmPools {
			controller.poolIndexer.Add(vmPool)
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

		// Run the controller
		controller.Execute()
	})
}
