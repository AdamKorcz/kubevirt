package disruptionbudget

import (
	stdruntime "runtime"
	"testing"

	gfh "github.com/AdaLogics/go-fuzz-headers"
	"github.com/golang/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	k8sv1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	framework "k8s.io/client-go/tools/cache/testing"
	"k8s.io/client-go/tools/record"
	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"

	virtController "kubevirt.io/kubevirt/pkg/controller"
	virtconfig "kubevirt.io/kubevirt/pkg/virt-config"

	"kubevirt.io/kubevirt/pkg/testutils"
)

var (
	maxResources      = 3
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

// FuzzExecute add up to 3 resources
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
		if len(vmis)+len(pods)+len(vmiMigrations)+len(pdbs) < 3 {
			return
		}

		ctrl := gomock.NewController(t)
		virtClient := kubecli.NewMockKubevirtClient(ctrl)
		vmiInformer, vmiSource := testutils.NewFakeInformerFor(&v1.VirtualMachineInstance{})
		pdbInformer, pdbSource := testutils.NewFakeInformerFor(&policyv1.PodDisruptionBudget{})
		vmimInformer, vmimSource := testutils.NewFakeInformerFor(&v1.VirtualMachineInstanceMigration{})
		podInformer, podSource := testutils.NewFakeInformerFor(&corev1.Pod{})
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
		defer func() {
			for _, obj := range crdInformer.GetStore().List() {
				err := crdInformer.GetStore().Delete(obj)
				if err != nil {
					panic(err)
				}
			}
		}()
		defer vmiSource.Shutdown()
		defer pdbSource.Shutdown()
		defer vmimSource.Shutdown()
		defer podSource.Shutdown()

		controller, _ := NewDisruptionBudgetController(vmiInformer, pdbInformer, podInformer, vmimInformer, recorder, virtClient, config)
		// Shut down default controller queue to avoid memory leak
		controller.Queue.ShutDown()
		mockQueue := testutils.NewMockWorkQueue(controller.Queue)
		controller.Queue = mockQueue

		// Set up mock client
		kubeClient := fake.NewSimpleClientset()
		virtClient.EXPECT().CoreV1().Return(kubeClient.CoreV1()).AnyTimes()
		virtClient.EXPECT().PolicyV1().Return(kubeClient.PolicyV1()).AnyTimes()

		// Make sure that all unexpected calls to kubeClient will fail
		kubeClient.Fake.PrependReactor("*", "*", func(action k8sTesting.Action) (handled bool, obj runtime.Object, err error) {
			return true, nil, nil
		})

		// Add the resources to the context
		for _, vmi := range vmis {
			key, err := virtController.KeyFunc(vmi)
			if err != nil {
				continue
			}
			controller.Queue.Add(key)
			vmiSource.Add(vmi)
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
			key, err := virtController.KeyFunc(pdb)
			if err != nil {
				continue
			}
			controller.Queue.Add(key)
			pdbSource.Add(pdb)
		}
		if controller.Queue.Len() == 0 {
			return
		}

		// Run the controller
		controller.Execute()
	})
}
