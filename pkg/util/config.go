package util

import (
	"fmt"
	"strings"

	"github.com/ghodss/yaml"

	"github.com/operator-framework/operator-sdk/pkg/k8sclient"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// installerConfigNamespace is the namespace containing the installer config.
	installerConfigNamespace = "kube-system"
	// clusterConfigResource is the resource containing the installer config.
	clusterConfigResource = "cluster-config-v1"
)

func GetInstallerConfigMap() (*corev1.ConfigMap, error) {
	client := k8sclient.GetKubeClient()
	resourceClient := client.CoreV1().ConfigMaps(installerConfigNamespace)

	cm, err := resourceClient.Get(clusterConfigResource, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting %s resource: %v", clusterConfigResource, err)
	}
	return cm, nil
}

func GetDefaultClusterDNSIP(cm *corev1.ConfigMap) (string, error) {
	if cm == nil {
		return "", fmt.Errorf("invalid installer config")
	}

	kc, ok := cm.Data["kco-config"]
	if !ok {
		return "", fmt.Errorf("missing kco-config in configmap")
	}

	kcoConfigMap := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(kc), &kcoConfigMap); err != nil {
		return "", fmt.Errorf("kco-config unmarshall error: %v", err)
	}

	dns, ok := kcoConfigMap["dnsConfig"]
	if !ok {
		return "", fmt.Errorf("missing dnsConfig in kco-config")
	}
	dnsConfig, ok := dns.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid dnsConfig in kco-config")
	}

	clusterIP, ok := dnsConfig["clusterIP"]
	if !ok {
		return "", fmt.Errorf("missing cluster IP in dnsConfig")
	}
	dnsClusterIP, ok := clusterIP.(string)
	if !ok {
		return "", fmt.Errorf("invalid cluster IP in dnsConfig")
	}
	if len(strings.TrimSpace(dnsClusterIP)) == 0 {
		return "", fmt.Errorf("empty cluster IP in dnsConfig")
	}

	return dnsClusterIP, nil
}
