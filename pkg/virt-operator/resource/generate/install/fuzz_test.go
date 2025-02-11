package install

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/virt-operator/util"
)

var (
	namespace = "fake-namespace"

	getConfig = func(registry, version string) *util.KubeVirtDeploymentConfig {
		return util.GetTargetConfigFromKV(&v1.KubeVirt{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
			},
			Spec: v1.KubeVirtSpec{
				ImageRegistry: registry,
				ImageTag:      version,
			},
		})
	}

	config = getConfig("fake-registry", "v9.9.9")
)

func FuzzLoadInstallStrategyFromCache(f *testing.F) {
	f.Fuzz(func(t *testing.T, data string, encoded bool) {
		stores := util.Stores{}
		stores.InstallStrategyConfigMapCache = cache.NewStore(cache.MetaNamespaceKeyFunc)

		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "plaintext-install-strategy",
				Namespace:    config.GetNamespace(),
				Annotations: map[string]string{
					v1.InstallStrategyVersionAnnotation:    config.GetKubeVirtVersion(),
					v1.InstallStrategyRegistryAnnotation:   config.GetImageRegistry(),
					v1.InstallStrategyIdentifierAnnotation: config.GetDeploymentID(),
				},
			},
			Data: map[string]string{
				"manifests": data,
			},
		}
		if encoded {
			configMap.Annotations[v1.InstallStrategyConfigMapEncoding] = ManifestsEncodingGzipBase64
		}
		stores.InstallStrategyConfigMapCache.Add(configMap)
		_, _ = LoadInstallStrategyFromCache(stores, config)
	})
}
