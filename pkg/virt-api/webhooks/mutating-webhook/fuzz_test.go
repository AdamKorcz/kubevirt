package mutating_webhook

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

func FuzzWebhookMutators(f *testing.F) {
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
		switch int(admitterType) % 3 {
		case 0:
			fdp := gfh.NewConsumer(clusterConfigData)
			clusterConfig := &virtconfig.ClusterConfig{}
			err = fdp.GenerateStruct(clusterConfig)
			if err != nil {
				return
			}
			ServeVMs(resp, req, clusterConfig, virtClient)
		case 1:
			ServeMigrationCreate(resp, req)
		case 2:
			ServeClones(resp, req)
		}
	})
}
