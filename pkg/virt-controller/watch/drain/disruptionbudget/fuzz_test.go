package disruptionbudget

import (
	"testing"

	gfh "github.com/AdaLogics/go-fuzz-headers"
	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"
	policyv1 "k8s.io/api/policy/v1"
	k8sTesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/kubernetes/fake"
	corev1 "k8s.io/api/core/v1"
	k8sv1 "k8s.io/api/core/v1"
	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	virtconfig "kubevirt.io/kubevirt/pkg/virt-config"
	framework "k8s.io/client-go/tools/cache/testing"

	"kubevirt.io/kubevirt/pkg/testutils"
)

var (
	maxResources = 3
)

// FuzzExecute add up to 3 XXXXXXXXXXXXXXX
// to the context and then runs the controller.
func FuzzExecute(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte,
							  numberOfVMIs,
							  numberOfVMIMigrations,
							  numberOfPods,
							  numberOfPDBs uint8) {
		fdp := gfh.NewConsumer(data)

		vmis := make([]*v1.VirtualMachineInstance, 0)
		for _ = range int(numberOfVMIs) % maxResources {
			vmi := &v1.VirtualMachineInstance{}
			err := fdp.GenerateStruct(vmi)
			if err != nil {
				return
			}
			vmis = append(vmis, vmi)
		}

		pods := make([]*k8sv1.Pod, 0)
		for _ = range int(numberOfPods) % maxResources {
			pod := &k8sv1.Pod{}
			err := fdp.GenerateStruct(pod)
			if err != nil {
				return
			}
			pods = append(pods, pod)
		}

		vmiMigrations := make([]*v1.VirtualMachineInstanceMigration, 0)
		for _ = range int(numberOfVMIMigrations) % maxResources {
			vmiMigration := &v1.VirtualMachineInstanceMigration{}
			err := fdp.GenerateStruct(vmiMigration)
			if err != nil {
				return
			}
			vmiMigrations = append(vmiMigrations, vmiMigration)
		}

		pdbs := make([]*policyv1.PodDisruptionBudget, 0)
		for _ = range int(numberOfPDBs) % maxResources {
			pdb := &policyv1.PodDisruptionBudget{}
			err := fdp.GenerateStruct(pdb)
			if err != nil {
				return
			}
			pdbs = append(pdbs, pdb)
		}
		if len(vmis) + len(pods) + len(vmiMigrations) + len(pdbs) < 3 {
			return
		}

		var vmiInformer cache.SharedIndexInformer
		var pdbInformer cache.SharedIndexInformer
		var podInformer cache.SharedIndexInformer
		var pdbSource *framework.FakeControllerSource
		var vmimInformer cache.SharedIndexInformer
		var vmiSource *framework.FakeControllerSource
		var recorder *record.FakeRecorder
		var mockQueue *testutils.MockWorkQueue[string]
		var kubeClient *fake.Clientset
		var pdbFeeder *testutils.PodDisruptionBudgetFeeder[string]
		var vmiFeeder *testutils.VirtualMachineFeeder[string]
		var config *virtconfig.ClusterConfig
		var virtClient *kubecli.MockKubevirtClient
		syncCaches := func(stop chan struct{}) {
			go vmiInformer.Run(stop)
			go pdbInformer.Run(stop)
			go podInformer.Run(stop)
			go vmimInformer.Run(stop)

			cache.WaitForCacheSync(stop,
				vmiInformer.HasSynced,
				pdbInformer.HasSynced,
				podInformer.HasSynced,
				vmimInformer.HasSynced,
			)
		}


		stop := make(chan struct{})
		defer close(stop)
		ctrl := gomock.NewController(t)
		virtClient = kubecli.NewMockKubevirtClient(ctrl)
		vmiInformer, vmiSource = testutils.NewFakeInformerFor(&v1.VirtualMachineInstance{})
		pdbInformer, pdbSource = testutils.NewFakeInformerFor(&policyv1.PodDisruptionBudget{})
		vmimInformer, _ = testutils.NewFakeInformerFor(&v1.VirtualMachineInstanceMigration{})
		podInformer, _ = testutils.NewFakeInformerFor(&corev1.Pod{})
		recorder = record.NewFakeRecorder(100)
		recorder.IncludeObject = true
		config, _, _ = testutils.NewFakeClusterConfigUsingKVConfig(&v1.KubeVirtConfiguration{})

		controller, _ := NewDisruptionBudgetController(vmiInformer, pdbInformer, podInformer, vmimInformer, recorder, virtClient, config)
		mockQueue = testutils.NewMockWorkQueue(controller.Queue)
		controller.Queue = mockQueue
		pdbFeeder = testutils.NewPodDisruptionBudgetFeeder(mockQueue, pdbSource)
		vmiFeeder = testutils.NewVirtualMachineFeeder(mockQueue, vmiSource)

		// Set up mock client
		kubeClient = fake.NewSimpleClientset()
		virtClient.EXPECT().CoreV1().Return(kubeClient.CoreV1()).AnyTimes()
		virtClient.EXPECT().PolicyV1().Return(kubeClient.PolicyV1()).AnyTimes()

		// Make sure that all unexpected calls to kubeClient will fail
		kubeClient.Fake.PrependReactor("*", "*", func(action k8sTesting.Action) (handled bool, obj runtime.Object, err error) {
			return true, nil, nil
		})
		syncCaches(stop)

		// Add the resources to the context
		for _, vmi := range vmis {
			go vmiFeeder.Add(vmi)
		}
		for _, vmiMigration := range vmiMigrations {
			err := vmimInformer.GetIndexer().Add(vmiMigration)
			if err != nil {
				return
			}
		}
		for _, pod := range pods {
			err := podInformer.GetIndexer().Add(pod)
			if err != nil {
				return
			}
		}
		for _, pdb := range pdbs {
			go pdbFeeder.Add(pdb)
		}
		if controller.Queue.Len() == 0 {
			return
		}

		// Run the controller
		controller.Execute()
	})
}