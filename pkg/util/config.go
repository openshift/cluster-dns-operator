package util

import (
	"fmt"
	"net"

	"github.com/apparentlymart/go-cidr/cidr"
	"github.com/ghodss/yaml"

	kapicore "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// INSTALLER_CONFIG_NAMESPACE is the namespace containing the installer config.
	INSTALLER_CONFIG_NAMESPACE = "kube-system"

	// CLUSTER_CONFIG_RESOURCE is the resource containing the installer config.
	CLUSTER_CONFIG_RESOURCE = "cluster-config-v1"
)

// installerClusterDNSIP
func installerClusterDNSIP(cm *kapicore.ConfigMap) (string, error) {
	if cm == nil {
		return "", fmt.Errorf("invalid installer config")
	}

	ic, ok := cm.Data["install-config"]
	if !ok {
		return "", fmt.Errorf("missing installer config")
	}

	installConfigMap := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(ic), &installConfigMap); err != nil {
		return "", fmt.Errorf("unmarshall error: %v", err)
	}

	networking, ok := installConfigMap["networking"]
	if !ok {
		return "", fmt.Errorf("missing installer networking config")
	}

	networkingConfig, ok := networking.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid installer networking config")
	}

	serviceCIDR, ok := networkingConfig["serviceCIDR"]
	if !ok {
		return "", fmt.Errorf("missing networking service CIDR")
	}

	s, ok := serviceCIDR.(string)
	if !ok {
		return "", fmt.Errorf("invalid networking service CIDR")
	}

	_, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		return "", fmt.Errorf("invalid networking service CIDR info")
	}

	ip, err := cidr.Host(ipnet, 10)
	if err != nil {
		return "", err
	}

	return ip.String(), nil
}

// ClusterDNSIP returns the cluster dns ip from the installer config.
func ClusterDNSIP(client kubernetes.Interface) (string, error) {
	resourceClient := client.CoreV1().ConfigMaps(INSTALLER_CONFIG_NAMESPACE)

	obj, err := resourceClient.Get(CLUSTER_CONFIG_RESOURCE, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting %s resource: %v", CLUSTER_CONFIG_RESOURCE, err)
	}

	return installerClusterDNSIP(obj)
}
