package migration

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
	kubevirtfake "kubevirt.io/client-go/kubevirt/fake"
	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	poolv1 "kubevirt.io/api/pool/v1alpha1"

	virtcontroller "kubevirt.io/kubevirt/pkg/controller"
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
							  numberOfNodes,
							  numberOfPDBs,
							  numberOfMPs uint8) {
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

		vmiMigrations := make([]*virtv1.VirtualMachineInstanceMigration, 0)
		for _ = range int(numberOfVMIMigrations) % maxResources {
			vmiMigration := &virtv1.VirtualMachineInstanceMigration{}
			err := fdp.GenerateStruct(vmiMigration)
			if err != nil {
				return
			}
			vmiMigrations = append(vmiMigrations, vmiMigration)
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

		pdbs := make([]*policyv1.PodDisruptionBudget, 0)
		for _ = range int(numberOfPDBs) % maxResources {
			pdb := &policyv1.PodDisruptionBudget{}
			err := fdp.GenerateStruct(pdb)
			if err != nil {
				return
			}
			pdbs = append(pdbs, pdb)
		}

		mps := make([]*migrationsv1.MigrationPolicy, 0)
		for _ = range int(numberOfMPs) % maxResources {
			pdb := &migrationsv1.MigrationPolicy{}
			err := fdp.GenerateStruct(pdb)
			if err != nil {
				return
			}
			mps = append(mps, pdb)
		}

		virtClient := kubecli.NewMockKubevirtClient(gomock.NewController(GinkgoT()))
		virtClientset = kubevirtfake.NewSimpleClientset()

		vmiInformer, _ := testutils.NewFakeInformerFor(&virtv1.VirtualMachineInstance{})
		migrationInformer, _ := testutils.NewFakeInformerFor(&virtv1.VirtualMachineInstanceMigration{})
		podInformer, _ := testutils.NewFakeInformerFor(&k8sv1.Pod{})
		pdbInformer, _ := testutils.NewFakeInformerFor(&policyv1.PodDisruptionBudget{})
		resourceQuotaInformer, _ := testutils.NewFakeInformerFor(&k8sv1.ResourceQuota{})
		namespaceInformer, _ := testutils.NewFakeInformerFor(&k8sv1.Namespace{})
		migrationPolicyInformer, _ := testutils.NewFakeInformerFor(&migrationsv1.MigrationPolicy{})
		recorder = record.NewFakeRecorder(100)
		recorder.IncludeObject = true
		nodeInformer, _ := testutils.NewFakeInformerFor(&k8sv1.Node{})

		pvcInformer, _ := testutils.NewFakeInformerFor(&k8sv1.PersistentVolumeClaim{})
		storageClassInformer, _ := testutils.NewFakeInformerFor(&storagev1.StorageClass{})
		storageProfileInformer, _ := testutils.NewFakeInformerFor(&cdiv1.StorageProfile{})

		config, _, _ := testutils.NewFakeClusterConfigUsingKVConfig(&virtv1.KubeVirtConfiguration{})
		controller, _ = NewController(
			services.NewTemplateService("a", 240, "b", "c", "d", "e", "f", pvcInformer.GetStore(), virtClient, config, qemuGid, "g", resourceQuotaInformer.GetStore(), namespaceInformer.GetStore()),
			vmiInformer,
			podInformer,
			migrationInformer,
			nodeInformer,
			pvcInformer,
			storageClassInformer,
			storageProfileInformer,
			pdbInformer,
			migrationPolicyInformer,
			resourceQuotaInformer,
			recorder,
			virtClient,
			config,
		)
		// Wrap our workqueue to have a way to detect when we are done processing updates
		mockQueue = testutils.NewMockWorkQueue(controller.Queue)
		controller.Queue = mockQueue

		// Set up mock client
		kubeClient = fake.NewSimpleClientset()
		virtClient.EXPECT().VirtualMachineInstanceMigration(k8sv1.NamespaceDefault).Return(virtClientset.KubevirtV1().VirtualMachineInstanceMigrations(k8sv1.NamespaceDefault)).AnyTimes()
		virtClient.EXPECT().VirtualMachineInstance(k8sv1.NamespaceDefault).Return(virtClientset.KubevirtV1().VirtualMachineInstances(k8sv1.NamespaceDefault)).AnyTimes()
		virtClient.EXPECT().CoreV1().Return(kubeClient.CoreV1()).AnyTimes()
		virtClient.EXPECT().PolicyV1().Return(kubeClient.PolicyV1()).AnyTimes()
		networkClient = fakenetworkclient.NewSimpleClientset()
		virtClient.EXPECT().NetworkClient().Return(networkClient).AnyTimes()
		virtClient.EXPECT().MigrationPolicy().Return(virtClientset.MigrationsV1alpha1().MigrationPolicies()).AnyTimes()


		// Add the resources to the context
		for _, vmi := range vmis {
			if len(vmi.Annotations) == 0 {
				vmi.Annotations = nil
			}
			if len(vmi.Labels) == 0 {
				vmi.Labels = nil
			}
			controller.vmiStore.Add(vmi)
			key, err := virtcontroller.KeyFunc(vmi)
			if err != nil {
				return
			}
			mockQueue.Add(key)
			_, err = virtClientset.KubevirtV1().VirtualMachineInstances(vmi.Namespace).Create(context.Background(), vmi, metav1.CreateOptions{})
			if err != nil {
				return
			}
		}
		for _, vmiMigration := range vmiMigrations {
			controller.migrationIndexer.Add(vmiMigration)
			key, err := virtcontroller.KeyFunc(vmiMigration)
			if err != nil {
				return
			}
			mockQueue.Add(key)
			_, err = virtClientset.KubevirtV1().VirtualMachineInstanceMigrations(vmiMigration.Namespace).Create(context.Background(), vmiMigration, metav1.CreateOptions{})
			if err != nil {
				return
			}
		}
		for _, node := range nodes {
			err := controller.nodeStore.Add(node)
			if err != nil {
				return
			}
			_, err = kubeClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
			if err != nil {
				return
			}
		}
		for _, pdb := range pdbs {
			err := controller.pdbIndexer.Add(pdb)
			if err != nil {
				return
			}
			_, err = kubeClient.PolicyV1().PodDisruptionBudgets(pdb.Namespace).Create(context.Background(), pdb, metav1.CreateOptions{})
			if err != nil {
				return
			}
		}
		for _, mp := range mps {
			err := controller.migrationPolicyStore.Add(mp)
			if err != nil {
				return
			}
			_, err = virtClientset.MigrationsV1alpha1().MigrationPolicies().Create(context.Background(), mp, metav1.CreateOptions{})
			if err != nil {
				return
			}
		}
		if mockQueue.Len() == 0 {
			return
		}

		// Run the controller
		controller.Execute()
	})
}