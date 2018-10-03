// +build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	stub "github.com/openshift/cluster-dns-operator/pkg/stub"

	kubeclient "github.com/operator-framework/operator-sdk/pkg/k8sclient"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	DNSNamespace   = "openshift-cluster-dns"
	AppNamespace   = "cluster-dns-test"
	AppServiceName = "app"

	ClusterDNSCustomResourceName = "test-default"
)

var testConfig *TestConfig

func TestIntegration(t *testing.T) {
	testConfig = NewTestConfig(t)

	createManifests()
	defer deleteManifests()

	startOperator()

	// Execute subtests
	t.Run("TestDNSRecords", testDNSRecords)
}

func testDNSRecords(t *testing.T) {
	createAppManifests()
	defer deleteAppManifests()

	clusterDNS, err := getClusterDNSCustomResource()
	if err != nil {
		t.Fatal(err)
	}

	f := manifests.NewFactory()
	dnsService, err := f.DNSService(clusterDNS)
	if err != nil {
		t.Fatal(err)
	}

	clusterDomain := getClusterDomain(clusterDNS)
	serviceIP, servicePort, err := getClusterIP(dnsService)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("configured cluster domain: %s, clusterIP: %s:%s", clusterDomain, serviceIP, servicePort)

	testPod, err := getDNSPod(DNSNamespace)
	if err != nil {
		t.Fatal(err)
	}

	type dnsTest struct {
		domain  string
		dnsType string
	}
	tests := []dnsTest{
		{
			domain:  fmt.Sprintf("dns-version.%s.", clusterDomain),
			dnsType: "txt",
		},
		{
			domain:  fmt.Sprintf("kubernetes.default.svc.%s.", clusterDomain),
			dnsType: "a",
		},
		{
			domain:  fmt.Sprintf("%s.%s.svc.%s.", AppServiceName, AppNamespace, clusterDomain),
			dnsType: "a",
		},
		{
			domain:  fmt.Sprintf("_https._tcp.kubernetes.default.svc.%s.", clusterDomain),
			dnsType: "srv",
		},
		{
			domain:  fmt.Sprintf("_http._tcp.%s.%s.svc.%s.", AppServiceName, AppNamespace, clusterDomain),
			dnsType: "srv",
		},
	}

	for _, test := range tests {
		digCmd := fmt.Sprintf("dig +noall +answer %s %s @%s -p %s", test.dnsType, test.domain, serviceIP, servicePort)
		cmd := fmt.Sprintf("oc exec %s -n %s -- %s", testPod, DNSNamespace, digCmd)
		msg := fmt.Sprintf("test domain: %s, type: %s", test.domain, test.dnsType)

		outStr, errStr := runShellCmd(cmd, msg)
		if len(errStr) != 0 {
			t.Fatalf("failed to %s, err: %s", msg, errStr)
		} else if len(outStr) == 0 {
			t.Fatalf("failed to %s, invalid answer", msg)
		}
	}
}

func getClusterDNSCustomResource() (*dnsv1alpha1.ClusterDNS, error) {
	cr := &dnsv1alpha1.ClusterDNS{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterDNS",
			APIVersion: "dns.openshift.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClusterDNSCustomResourceName,
			Namespace: testConfig.operatorNamespace,
		},
	}
	if err := sdk.Get(cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func getClusterDomain(dns *dnsv1alpha1.ClusterDNS) string {
	clusterDomain := "cluster.local"
	if dns.Spec.ClusterDomain != nil {
		clusterDomain = *dns.Spec.ClusterDomain
	}
	return clusterDomain
}

func getClusterIP(dnsService *corev1.Service) (string, string, error) {
	var serviceIP string
	var servicePort string
	var err error

	err = wait.Poll(500*time.Millisecond, time.Minute, func() (bool, error) {
		err = sdk.Get(dnsService)
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if len(dnsService.Spec.ClusterIP) > 0 {
			serviceIP = dnsService.Spec.ClusterIP
			servicePort = strconv.FormatUint(uint64(dnsService.Spec.Ports[0].Port), 10)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("timed out waiting for DNS service: %v", err)
	}
	return serviceIP, servicePort, nil
}

func getDNSPod(ns string) (string, error) {
	var podName string

	var err error
	kubeClient := kubeclient.GetKubeClient()

	err = wait.Poll(500*time.Millisecond, time.Minute, func() (bool, error) {
		// NOTE: kubeClient is used instead of sdk.List(ns, podList) as the
		// latter is not reliably populating pod.Status.Phase field at this time.
		podList, err := kubeClient.Core().Pods(ns).List(metav1.ListOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		for _, pod := range podList.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			podName = pod.Name
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("timed out waiting for DNS pod: %v", err)
	}
	return podName, nil
}

type TestConfig struct {
	operatorNamespace string
	kubeConfig        string

	t *testing.T
}

func NewTestConfig(t *testing.T) *TestConfig {
	config := &TestConfig{t: t}

	// Check prerequisites
	kubeConfig := os.Getenv("KUBECONFIG")
	if len(kubeConfig) == 0 {
		t.Fatalf("KUBECONFIG is required")
	}
	// The operator-sdk uses KUBERNETES_CONFIG...
	os.Setenv("KUBERNETES_CONFIG", kubeConfig)
	config.kubeConfig = kubeConfig

	config.operatorNamespace = "default"
	os.Setenv("WATCH_NAMESPACE", config.operatorNamespace)

	// NOTE: Do not call sdk.GetKubeClient() or sdk.GetKubeConfig() in this method.
	// Calling these methods before starting operator will intialize the sdk rest mapper
	// and the mapper is refreshed every 1 minute for new changes.
	// Due to this side effect, sdk.Watch will fail for 'ClusterDNS' kind as this will
	// not be found in the initial rest mapper cache.
	return config
}

func startOperator() {
	resource := "dns.openshift.io/v1alpha1"
	kind := "ClusterDNS"
	resyncPeriod := 10 * time.Minute
	testConfig.t.Logf("Watching %s, %s, %s, %d", resource, kind, testConfig.operatorNamespace, resyncPeriod)
	sdk.Watch(resource, kind, testConfig.operatorNamespace, resyncPeriod)
	sdk.Handle(stub.NewHandler())
	go sdk.Run(context.TODO())
}

func createAppManifests() {
	runShellCmd("oc apply -f manifests/", "create app manifests")
}

func deleteAppManifests() {
	runShellCmd(fmt.Sprintf("oc delete ns %s", AppNamespace), "delete app namespace")
	runShellCmd(fmt.Sprintf("oc delete clusterdns %s -n %s", ClusterDNSCustomResourceName, testConfig.operatorNamespace), "delete cluster dns custom resource")
}

func createManifests() {
	runShellCmd(fmt.Sprintf("oc apply -f ../../manifests/0000_08_cluster-dns-operator_00-custom-resource-definition.yaml -n %s", testConfig.operatorNamespace), "create cluster dns CRD")
	runShellCmd("oc apply -f ../../manifests/0000_08_cluster-dns-operator_00-dns-namespace.yaml", "create dns namespace")
	runShellCmd("oc apply -f ../../manifests/0000_08_cluster-dns-operator_00-dns-cluster-role.yaml", "create dns cluster role")
	runShellCmd("oc apply -f ../../manifests/0000_08_cluster-dns-operator_01-dns-cluster-role-binding.yaml", "create dns cluster role binding")
	runShellCmd("oc apply -f ../../manifests/0000_08_cluster-dns-operator_01-dns-service-account.yaml", "create dns service account")
}

func deleteManifests() {
	runShellCmd(fmt.Sprintf("oc delete crd clusterdnses.dns.openshift.io -n %s", testConfig.operatorNamespace), "delete cluster dns CRD")
	runShellCmd(fmt.Sprintf("oc delete ns %s", DNSNamespace), "delete dns namespace")
	runShellCmd(fmt.Sprintf("oc delete clusterrolebinding cluster-dns:dns -n %s", DNSNamespace), "delete dns cluster role binding")
	runShellCmd(fmt.Sprintf("oc delete clusterrole cluster-dns:dns -n %s", DNSNamespace), "delete dns cluster role")
	runShellCmd(fmt.Sprintf("oc delete sa dns -n %s", DNSNamespace), "delete dns service account")
}

func runShellCmd(command, msg string) (string, string) {
	outBuf := bytes.Buffer{}
	errBuf := bytes.Buffer{}

	command = fmt.Sprintf("KUBECONFIG=%s %s", testConfig.kubeConfig, command)
	cmd := []string{"sh", "-c", command}
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	if err := c.Run(); err != nil {
		testConfig.t.Fatalf("failed to %s: %v\ncommand: %s\nstdout: %s\nstderr: %s", msg, err, strings.Join(append([]string{cmd[0]}, cmd[1:]...), " "), outBuf.String(), errBuf.String())
	}
	return outBuf.String(), errBuf.String()
}
