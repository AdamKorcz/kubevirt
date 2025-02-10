package evacuation

import (
	"testing"

	gfh "github.com/AdaLogics/go-fuzz-headers"
	"github.com/golang/mock/gomock"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	kubevirtfake "kubevirt.io/client-go/kubevirt/fake"

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
		numberOfNodes,
		numberOfPods,
		numberOfMigrations uint8) {
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

		nodes := make([]*k8sv1.Node, 0)
		for _ = range int(numberOfNodes) % maxResources {
			node := &k8sv1.Node{}
			err := fdp.GenerateStruct(node)
			if err != nil {
				return
			}
			nodes = append(nodes, node)
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

		migrations := make([]*v1.VirtualMachineInstanceMigration, 0)
		for _ = range int(numberOfMigrations) % maxResources {
			migration := &v1.VirtualMachineInstanceMigration{}
			err := fdp.GenerateStruct(migration)
			if err != nil {
				return
			}
			migrations = append(migrations, migration)
		}

		stop := make(chan struct{})
		defer close(stop)
		ctrl := gomock.NewController(t)
		virtClient := kubecli.NewMockKubevirtClient(ctrl)
		fakeVirtClient := kubevirtfake.NewSimpleClientset()

		vmiInformer, vmiSource := testutils.NewFakeInformerWithIndexersFor(&v1.VirtualMachineInstance{}, cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
			"node": func(obj interface{}) (strings []string, e error) {
				return []string{obj.(*v1.VirtualMachineInstance).Status.NodeName}, nil
			},
		})
		migrationInformer, migrationSource := testutils.NewFakeInformerFor(&v1.VirtualMachineInstanceMigration{})
		nodeInformer, nodeSource := testutils.NewFakeInformerFor(&k8sv1.Node{})
		podInformer, podSource := testutils.NewFakeInformerFor(&k8sv1.Pod{})
		recorder := record.NewFakeRecorder(100)
		recorder.IncludeObject = true
		config, _, _ := testutils.NewFakeClusterConfigUsingKVConfig(&v1.KubeVirtConfiguration{})

		controller, _ := NewEvacuationController(vmiInformer, migrationInformer, nodeInformer, podInformer, recorder, virtClient, config)
		mockQueue := testutils.NewMockWorkQueue(controller.Queue)
		controller.Queue = mockQueue
		migrationFeeder := testutils.NewMigrationFeeder(mockQueue, migrationSource)
		vmiFeeder := testutils.NewVirtualMachineFeeder(mockQueue, vmiSource)

		// Set up mock client
		virtClient.EXPECT().VirtualMachineInstanceMigration(k8sv1.NamespaceDefault).Return(fakeVirtClient.KubevirtV1().VirtualMachineInstanceMigrations(k8sv1.NamespaceDefault)).AnyTimes()
		kubeClient := fake.NewSimpleClientset()
		virtClient.EXPECT().CoreV1().Return(kubeClient.CoreV1()).AnyTimes()
		virtClient.EXPECT().PolicyV1().Return(kubeClient.PolicyV1()).AnyTimes()

		// Make sure that all unexpected calls to kubeClient will fail
		kubeClient.Fake.PrependReactor("*", "*", func(action k8sTesting.Action) (handled bool, obj runtime.Object, err error) {
			return true, nil, nil
		})

		syncCaches := func(stop chan struct{}) {
			go vmiInformer.Run(stop)
			go migrationInformer.Run(stop)
			go nodeInformer.Run(stop)
			go podInformer.Run(stop)

			cache.WaitForCacheSync(stop,
				vmiInformer.HasSynced,
				migrationInformer.HasSynced,
				nodeInformer.HasSynced,
				podInformer.HasSynced,
			)
		}

		syncCaches(stop)

		// Add the resources to the context
		for _, vmi := range vmis {
			go vmiFeeder.Add(vmi)
		}
		for _, node := range nodes {
			go func() {
				mockQueue.ExpectAdds(1)
				nodeSource.Add(node)
				mockQueue.Wait()
			}()
		}
		for _, pod := range pods {
			go func() {
				mockQueue.ExpectAdds(1)
				podSource.Add(pod)
				mockQueue.Wait()
			}()
		}
		for _, migration := range migrations {
			go migrationFeeder.Add(migration)
		}

		// Run the controller
		controller.Execute()
	})
}
