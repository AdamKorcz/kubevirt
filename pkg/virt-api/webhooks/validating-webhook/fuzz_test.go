package validating_webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/mock/gomock"
	admissionv1 "k8s.io/api/admission/v1"
	"kubevirt.io/client-go/kubecli"
	"kubevirt.io/client-go/kubevirt/fake"

	virtconfig "kubevirt.io/kubevirt/pkg/virt-config"

	gfh "github.com/AdaLogics/go-fuzz-headers"
)

func FuzzWebhookAdmitters(f *testing.F) {
	f.Fuzz(func(t *testing.T, arBytes, clusterConfigData []byte, admitterType uint8) {
		ar := admissionv1.AdmissionReview{}
		err := ar.Unmarshal(arBytes)
		if err != nil {
			return
		}		
		if ar.Request == nil {
			return
		}
		marshaledBytes, err := json.Marshal(ar)
		if err != nil {
			return
		}
		req, err := http.NewRequest("GET", "http://example.com", bytes.NewReader(marshaledBytes))
		if err != nil {
			panic(err)
		}
		req.Header.Add("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		ctrl := gomock.NewController(t)
		virtClient := kubecli.NewMockKubevirtClient(ctrl)
		virtClient.EXPECT().GeneratedKubeVirtClient().Return(fake.NewSimpleClientset()).AnyTimes()
		switch int(admitterType) % 15 {
		case 0:
			fdp := gfh.NewConsumer(clusterConfigData)
			clusterConfig := &virtconfig.ClusterConfig{}
			err = fdp.GenerateStruct(clusterConfig)
			if err != nil {
				return
			}
			kubeVirtServiceAccounts := make(map[string]struct{})
			ServeVMIUpdate(resp, req, clusterConfig, kubeVirtServiceAccounts)
		case 1:
			fdp := gfh.NewConsumer(clusterConfigData)
			clusterConfig := &virtconfig.ClusterConfig{}
			err = fdp.GenerateStruct(clusterConfig)
			if err != nil {
				return
			}
			ServeVMIRS(resp, req, clusterConfig)
		case 2:
			fdp := gfh.NewConsumer(clusterConfigData)
			clusterConfig := &virtconfig.ClusterConfig{}
			err = fdp.GenerateStruct(clusterConfig)
			if err != nil {
				return
			}
			kubeVirtServiceAccounts := make(map[string]struct{})
			ServeVMPool(resp, req, clusterConfig, kubeVirtServiceAccounts)
		case 3:
			ServeVMIPreset(resp, req)
		case 4:
			var clusterConfig *virtconfig.ClusterConfig
			ServeMigrationCreate(resp, req, clusterConfig, virtClient)
		case 5:
			ServeMigrationUpdate(resp, req)
		case 6:
			fdp := gfh.NewConsumer(clusterConfigData)
			clusterConfig := &virtconfig.ClusterConfig{}
			err = fdp.GenerateStruct(clusterConfig)
			if err != nil {
				return
			}
			ServeVMSnapshots(resp, req, clusterConfig, virtClient)
		case 7:
			fdp := gfh.NewConsumer(clusterConfigData)
			clusterConfig := &virtconfig.ClusterConfig{}
			err = fdp.GenerateStruct(clusterConfig)
			if err != nil {
				return
			}
			ServeVMExports(resp, req, clusterConfig)
		case 8:
			ServeVmInstancetypes(resp, req)
		case 9:
			ServeVmClusterInstancetypes(resp, req)
		case 10:
			ServeVmPreferences(resp, req)
		case 11:
			ServeVmClusterPreferences(resp, req)
		case 12:
			ServeMigrationPolicies(resp, req)
		case 13:
			fdp := gfh.NewConsumer(clusterConfigData)
			clusterConfig := &virtconfig.ClusterConfig{}
			err = fdp.GenerateStruct(clusterConfig)
			if err != nil {
				return
			}
			ServeVirtualMachineClones(resp, req, clusterConfig, virtClient)
		}
	})
}
