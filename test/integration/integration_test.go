// +build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	stub "github.com/openshift/cluster-dns-operator/pkg/stub"
	"github.com/openshift/cluster-dns-operator/test/manifests"

	kubeclient "github.com/operator-framework/operator-sdk/pkg/k8sclient"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	k8sutil "github.com/operator-framework/operator-sdk/pkg/util/k8sutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var testConfig *TestConfig

func TestIntegration(t *testing.T) {
	testConfig = NewTestConfig(t)

	createCRD()
	defer deleteCRD()

	startOperator()

	// Execute subtests
	t.Run("TestDNSRecords", testDNSRecords)
}

func testDNSRecords(t *testing.T) {
	f := manifests.NewFactory()

	appNamespace, err := f.AppDNSNamespace()
	if err != nil {
		t.Fatal(err)
	}
	appDeployment, err := f.AppDNSDeployment()
	if err != nil {
		t.Fatal(err)
	}
	appService, err := f.AppDNSService()
	if err != nil {
		t.Fatal(err)
	}

	dnsNamespace, err := f.DNSNamespace()
	if err != nil {
		t.Fatal(err)
	}
	clusterDNS, err := f.ClusterDNSCustomResource()
	if err != nil {
		t.Fatal(err)
	}
	clusterDNS.Namespace = testConfig.operatorNamespace

	dnsService, err := f.DNSService(clusterDNS)
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		leftovers := []sdk.Object{
			clusterDNS,
			dnsNamespace,
			appNamespace,
		}
		anyFailed := false
		for _, o := range leftovers {
			err := sdk.Delete(o)
			if err != nil && !errors.IsNotFound(err) {
				t.Logf("failed to clean up object %#v: %s", o, err)
				anyFailed = true
			}
		}
		if anyFailed {
			t.Fatalf("failed to clean up resources")
		}
	}
	defer cleanup()

	err = sdk.Create(appNamespace)
	if err != nil {
		t.Fatal(err)
	}
	err = sdk.Create(appDeployment)
	if err != nil {
		t.Fatal(err)
	}
	err = sdk.Create(appService)
	if err != nil {
		t.Fatal(err)
	}
	err = sdk.Create(clusterDNS)
	if err != nil {
		t.Fatal(err)
	}

	clusterDomain := getClusterDomain(clusterDNS)
	serviceIP, servicePort, err := getClusterIP(dnsService)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("configured cluster domain: %s, clusterIP: %s:%s", clusterDomain, serviceIP, servicePort)

	testPod, err := getDNSPod(dnsNamespace.Name)
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
			domain:  fmt.Sprintf("%s.%s.svc.%s.", appService.Name, appService.Namespace, clusterDomain),
			dnsType: "a",
		},
		{
			domain:  fmt.Sprintf("_https._tcp.kubernetes.default.svc.%s.", clusterDomain),
			dnsType: "srv",
		},
		{
			domain:  fmt.Sprintf("_http._tcp.%s.%s.svc.%s.", appService.Name, appService.Namespace, clusterDomain),
			dnsType: "srv",
		},
	}

	for _, test := range tests {
		digCmd := fmt.Sprintf("dig +noall +answer %s %s @%s -p %s", test.dnsType, test.domain, serviceIP, servicePort)
		cmd := fmt.Sprintf("oc exec %s -n %s -- %s", testPod, dnsNamespace.Name, digCmd)
		msg := fmt.Sprintf("test domain: %s, type: %s", test.domain, test.dnsType)

		outStr, errStr := runShellCmd(cmd, msg)
		if len(errStr) != 0 {
			t.Fatalf("failed to %s, err: %s", msg, errStr)
		} else if len(outStr) == 0 {
			t.Fatalf("failed to %s, invalid answer", msg)
		}
	}
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

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		namespace = "default"
		os.Setenv("WATCH_NAMESPACE", namespace)
	}
	config.operatorNamespace = namespace

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
	resyncPeriod := 5
	testConfig.t.Logf("Watching %s, %s, %s, %d", resource, kind, testConfig.operatorNamespace, resyncPeriod)
	sdk.Watch(resource, kind, testConfig.operatorNamespace, resyncPeriod)
	sdk.Handle(stub.NewHandler())
	go sdk.Run(context.TODO())
}

func createCRD() {
	runShellCmd(fmt.Sprintf("oc apply -f ../../deploy/crd.yaml -n %s", testConfig.operatorNamespace), "create cluster dns CRD")
}

func deleteCRD() {
	runShellCmd(fmt.Sprintf("oc delete crd clusterdnses.dns.openshift.io -n %s", testConfig.operatorNamespace), "delete cluster dns CRD")
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
		testConfig.t.Fatalf("failed to %s: %v", msg, err)
	}
	return outBuf.String(), errBuf.String()
}
