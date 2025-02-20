package evacuation

import (
	stdruntime "runtime"
	"testing"

	gfh "github.com/AdaLogics/go-fuzz-headers"
	"github.com/golang/mock/gomock"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	virtconfig "kubevirt.io/kubevirt/pkg/virt-config"
	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	virtController "kubevirt.io/kubevirt/pkg/controller"
	kubevirtfake "kubevirt.io/client-go/kubevirt/fake"
	framework "k8s.io/client-go/tools/cache/testing"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"kubevirt.io/kubevirt/pkg/testutils"
)

var (
	maxResources = 4
	kvObjectNamespace = "kubevirt"
	kvObjectName      = "kubevirt"
)

func NewFakeClusterConfigUsingKV(kv *v1.KubeVirt) (*virtconfig.ClusterConfig, cache.SharedIndexInformer, cache.Store, *framework.FakeControllerSource, *framework.FakeControllerSource) {
	return NewFakeClusterConfigUsingKVWithCPUArch(kv, stdruntime.GOARCH)
}

func NewFakeClusterConfigUsingKVWithCPUArch(kv *v1.KubeVirt, CPUArch string) (*virtconfig.ClusterConfig, cache.SharedIndexInformer, cache.Store, *framework.FakeControllerSource, *framework.FakeControllerSource) {
	kv.ResourceVersion = rand.String(10)
	kv.Status.Phase = "Deployed"
	crdInformer, cs1 := testutils.NewFakeInformerFor(&extv1.CustomResourceDefinition{})
	kubeVirtInformer, cs2 := testutils.NewFakeInformerFor(&v1.KubeVirt{})

	kubeVirtInformer.GetStore().Add(kv)

	AddDataVolumeAPI(crdInformer)
	cfg, _ := virtconfig.NewClusterConfigWithCPUArch(crdInformer, kubeVirtInformer, kvObjectNamespace, CPUArch)
	return cfg, crdInformer, kubeVirtInformer.GetStore(), cs1, cs2
}

func AddDataVolumeAPI(crdInformer cache.SharedIndexInformer) {
	crdInformer.GetStore().Add(&extv1.CustomResourceDefinition{
		Spec: extv1.CustomResourceDefinitionSpec{
			Names: extv1.CustomResourceDefinitionNames{
				Kind: "DataVolume",
			},
		},
	})
}

func NewFakeClusterConfigUsingKVConfig(kv *v1.KubeVirt) (*virtconfig.ClusterConfig, cache.SharedIndexInformer, cache.Store, *framework.FakeControllerSource, *framework.FakeControllerSource) {
	return NewFakeClusterConfigUsingKV(kv)
}

// FuzzExecute add up to 4 resources
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

		if len(vmis) + len(nodes) + len(pods) + len(migrations) == 0 {
			return
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
		defer migrationSource.Shutdown()
		defer nodeSource.Shutdown()
		defer podSource.Shutdown()
		recorder := record.NewFakeRecorder(100)
		recorder.IncludeObject = true

		kv := &v1.KubeVirt{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kvObjectName,
				Namespace: kvObjectNamespace,
			},
			Spec: v1.KubeVirtSpec{
				Configuration: v1.KubeVirtConfiguration{},
			},
			Status: v1.KubeVirtStatus{
				DefaultArchitecture: stdruntime.GOARCH,
				Phase:               "Deployed",
			},
		}

		config, crdInformer, kubeVirtInformerStore, cs1, cs2 := NewFakeClusterConfigUsingKVConfig(kv)
		defer cs1.Shutdown()
		defer cs2.Shutdown()
		defer kubeVirtInformerStore.Delete(kv)
		defer func(){
				for _, obj := range crdInformer.GetStore().List() {
				err := crdInformer.GetStore().Delete(obj)
				if err != nil {
					panic(err)
				}
			}
		}()

		controller, _ := NewEvacuationController(vmiInformer, migrationInformer, nodeInformer, podInformer, recorder, virtClient, config)
		controller.Queue.ShutDown()
		mockQueue := testutils.NewMockWorkQueue(controller.Queue)
		controller.Queue = mockQueue

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
			key, err := virtController.KeyFunc(vmi)
			if err != nil {
				continue
			}
			controller.Queue.Add(key)
			vmiSource.Add(vmi)
		}
		for _, node := range nodes {
			key, err := virtController.KeyFunc(node)
			if err != nil {
				continue
			}
			controller.Queue.Add(key)
			nodeSource.Add(node)
		}
		for _, pod := range pods {
			key, err := virtController.KeyFunc(pod)
			if err != nil {
				continue
			}
			controller.Queue.Add(key)
			podSource.Add(pod)
		}
		for _, migration := range migrations {
			key, err := virtController.KeyFunc(migration)
			if err != nil {
				continue
			}
			controller.Queue.Add(key)
			migrationSource.Add(migration)
		}
		if controller.Queue.Len() == 0 {
			return
		}
		panic("Here")


		// Run the controller
		controller.Execute()
	})
}
