//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os/exec"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	operatorclient "github.com/openshift/cluster-dns-operator/pkg/operator/client"
	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/utils/pointer"
)

// lookForStringInPodExec looks for expectedString in the output of command
// executed in the specified pod container every 2 seconds until the timeout
// is reached or the string is found. Returns an error if the string was not found.
func lookForStringInPodExec(ns, pod, container string, command []string, expectedString string, timeout time.Duration) error {
	cmdPath, err := exec.LookPath("oc")
	if err != nil {
		return err
	}
	args := []string{"exec", pod, "-c", container, fmt.Sprintf("--namespace=%v", ns), "--"}
	args = append(args, command...)
	if err := lookForString(cmdPath, args, expectedString, timeout); err != nil {
		return err
	}
	return nil
}

// lookForStringInPodLog looks for the given string in the log of the
// specified pod container every 2 seconds until the timeout is reached
// or the string is found. Returns an error if the string was not found.
func lookForStringInPodLog(ns, pod, container, expectedString string, timeout time.Duration) error {
	cmdPath, err := exec.LookPath("oc")
	if err != nil {
		return err
	}
	args := []string{"logs", pod, "-c", container, fmt.Sprintf("--namespace=%v", ns)}
	if err := lookForString(cmdPath, args, expectedString, timeout); err != nil {
		return err
	}
	return nil
}

// lookForStringInPodLog looks for the given string in the log of the
// specified pod container every 2 seconds until the timeout is reached
// or the string is found. Returns an error if the string was not found.
func lookForSubStringsInPodLog(ns, pod, container string, timeout time.Duration, expectedStrings ...string) error {
	cmdPath, err := exec.LookPath("oc")
	if err != nil {
		return err
	}
	args := []string{"logs", pod, "-c", container, fmt.Sprintf("--namespace=%v", ns)}
	if bool, err := lookForSubStrings(cmdPath, args, timeout, expectedStrings); err != nil && !bool {
		return err
	}
	return nil
}

// lookForString looks for the given string using cmd and args every
// 2 seconds until the timeout is reached or the string is found.
// Returns an error if the string was not found.
func lookForString(cmd string, args []string, expectedString string, timeout time.Duration) error {
	err := wait.PollImmediate(2*time.Second, timeout, func() (bool, error) {
		result, err := runCmd(cmd, args)
		//fmt.Printf("\n result %v", result)
		if err != nil {
			return false, nil
		}
		if !strings.Contains(result, expectedString) {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to find %q", expectedString)
	}
	return nil
}
func lookForSubStrings(cmd string, args []string, timeout time.Duration, expectedStrings []string) (bool, error) {
	err := wait.PollImmediate(2*time.Second, timeout, func() (bool, error) {
		result, err := runCmd(cmd, args)
		if err != nil {
			return false, nil
		}
		slicedResult := strings.Split(result, "\"")
		slicedResultToString := strings.Join(slicedResult, " ")
		if bool, err := checkSubStrings(slicedResultToString, expectedStrings); err != nil && !bool {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to find %q", expectedStrings)
	}
	return true, nil
}

func checkSubStrings(str string, subs []string) (bool, error) {
	isCompleteMatch := true

	for _, sub := range subs {
		if strings.Contains(str, sub) {
		} else {
			isCompleteMatch = false
		}
	}

	if !isCompleteMatch {
		return false, fmt.Errorf("failed to find a match %q", strings.Join(subs, ""))
	}

	return isCompleteMatch, nil
}

// runCmd runs command cmd with arguments args and returns the output
// of the command or an error.
func runCmd(cmd string, args []string) (string, error) {
	execCmd := exec.Command(cmd, args...)
	result, err := execCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run command %q with args %q: %v", cmd, args, err)
	}
	return string(result), nil
}

// upstreamContainer returns a Container definition configured for
// the test upstream resolver.
func upstreamContainer(container, image string) corev1.Container {
	dnsPorts := []corev1.ContainerPort{
		{
			Name:          "dns",
			ContainerPort: int32(5353),
			Protocol:      corev1.Protocol("UDP"),
		},
		{
			Name:          "dns-tcp",
			ContainerPort: int32(5353),
			Protocol:      corev1.Protocol("TCP"),
		},
	}
	healthPort := intstr.IntOrString{
		IntVal: int32(8080),
	}
	getAction := &corev1.HTTPGetAction{
		Path:   "/health",
		Port:   healthPort,
		Scheme: "HTTP",
	}
	healthProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: getAction,
		},
		InitialDelaySeconds: int32(10),
		TimeoutSeconds:      int32(10),
	}
	configVolume := corev1.VolumeMount{
		Name:      "config-volume",
		ReadOnly:  true,
		MountPath: "/etc/coredns",
	}

	return corev1.Container{
		Name:           container,
		Image:          image,
		Command:        []string{"coredns"},
		Args:           []string{"-conf", "/etc/coredns/Corefile"},
		Ports:          dnsPorts,
		VolumeMounts:   []corev1.VolumeMount{configVolume},
		LivenessProbe:  healthProbe,
		ReadinessProbe: healthProbe,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: pointer.Bool(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			RunAsNonRoot: pointer.Bool(false),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
	}
}

// upstreamPod returns a Pod definition configured for the test
// upstream resolver.
func upstreamPod(name, ns, image, cfgMap string) *corev1.Pod {
	coreContainer := upstreamContainer(name, image)
	volMode := int32(420)
	volSrc := &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: cfgMap,
		},
		Items: []corev1.KeyToPath{
			{
				Key:  "Corefile",
				Path: "Corefile",
			},
		},
		DefaultMode: &volMode,
	}
	cfgVol := corev1.Volume{
		Name: "config-volume",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: volSrc,
		},
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{"test": "upstream"},
		},
		Spec: corev1.PodSpec{
			Volumes:            []corev1.Volume{cfgVol},
			Containers:         []corev1.Container{coreContainer},
			ServiceAccountName: "dns",
		},
	}
}

// upstreamService returns a Service definition configured for the
// test upstream resolver.
func upstreamService(name, ns string) *corev1.Service {
	svcPorts := []corev1.ServicePort{
		{
			Name:       "dns",
			Protocol:   "UDP",
			Port:       53,
			TargetPort: intstr.IntOrString{IntVal: 5353},
		},
		{
			Name:       "dns-tcp",
			Protocol:   "TCP",
			Port:       53,
			TargetPort: intstr.IntOrString{IntVal: 5353},
		},
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Ports:    svcPorts,
			Selector: map[string]string{"test": "upstream"},
		},
	}
}

// buildConfigMap returns a ConfigMap definition using name
// for the ConfigMap name, ns as the ConfigMap namespace, k
// as the ConfigMap data key and v as the ConfigMap data value.
func buildConfigMap(name, ns, k, v string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string]string{k: v},
	}
}

// buildPod returns a Pod definition using name as the Pod's name, ns as
// the Pod's namespace, image as the Pod container's image and cmd as the
// Pod container's command.
func buildPod(name, ns, image string, cmd []string) *corev1.Pod {
	container := buildContainer(name, image, cmd)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{container},
		},
	}
}

// buildContainer returns a Container definition using name as the
// Container's name, image as the Container's image and cmd as
// Container's command.
func buildContainer(name, image string, cmd []string) corev1.Container {
	return corev1.Container{
		Name:    name,
		Image:   image,
		Command: cmd,
	}
}

func waitForClusterOperatorConditions(t *testing.T, cl client.Client, timeout time.Duration, conditions ...configv1.ClusterOperatorStatusCondition) error {
	return wait.PollImmediate(1*time.Second, timeout, func() (bool, error) {
		co := &configv1.ClusterOperator{}
		coName := operatorcontroller.DNSClusterOperatorName()
		if err := cl.Get(context.TODO(), coName, co); err != nil {
			t.Logf("failed to get DNS cluster operator %s: %v", coName.Name, err)
			return false, nil
		}

		expected := clusterOperatorConditionMap(conditions...)
		current := clusterOperatorConditionMap(co.Status.Conditions...)
		return conditionsMatchExpected(expected, current), nil
	})
}

func waitForDNSConditions(t *testing.T, cl client.Client, timeout time.Duration, name types.NamespacedName, conditions ...operatorv1.OperatorCondition) error {
	// successCount prevents not waiting for a case where DNS is updated but not yet started reporting progressing=true.
	// Without this, waitForDNSConditions returns nil and DNS starts an update, so the next code relaying this function fails sporadically.
	successCount := 0
	return wait.PollImmediate(1*time.Second, timeout, func() (bool, error) {
		dns := &operatorv1.DNS{}
		if err := cl.Get(context.TODO(), name, dns); err != nil {
			t.Logf("failed to get DNS operator %s: %v", name.Name, err)
			return false, nil
		}
		expected := operatorConditionMap(conditions...)
		current := operatorConditionMap(dns.Status.Conditions...)

		if conditionsMatchExpected(expected, current) {
			successCount++
		}

		if successCount > 3 {
			return true, nil
		}
		return false, nil
	})
}

func clusterOperatorConditionMap(conditions ...configv1.ClusterOperatorStatusCondition) map[string]string {
	conds := map[string]string{}
	for _, cond := range conditions {
		conds[string(cond.Type)] = string(cond.Status)
	}
	return conds
}

func operatorConditionMap(conditions ...operatorv1.OperatorCondition) map[string]string {
	conds := map[string]string{}
	for _, cond := range conditions {
		conds[cond.Type] = string(cond.Status)
	}
	return conds
}

func conditionsMatchExpected(expected, actual map[string]string) bool {
	filtered := map[string]string{}
	for k := range actual {
		if _, comparable := expected[k]; comparable {
			filtered[k] = actual[k]
		}
	}
	return reflect.DeepEqual(expected, filtered)
}

func getClusterDNSOperatorPods(cl client.Client) (*corev1.PodList, error) {
	dnsOperatorDeployment := &appsv1.Deployment{}
	if err := cl.Get(context.TODO(), operatorcontroller.DefaultDNSOperatorDeploymentName(), dnsOperatorDeployment); err != nil {
		return nil, fmt.Errorf("failed to get deployment %s/%s: %v", dnsOperatorDeployment.Namespace, dnsOperatorDeployment.Name, err)
	}
	operatorSelector, err := metav1.LabelSelectorAsSelector(dnsOperatorDeployment.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("daemonset %s/%s has invalid spec.selector: %v", dnsOperatorDeployment.Namespace, dnsOperatorDeployment.Name, err)
	}

	dnsOperatorPods := &corev1.PodList{}
	if err := cl.List(context.TODO(), dnsOperatorPods, client.MatchingLabelsSelector{Selector: operatorSelector}, client.InNamespace(dnsOperatorDeployment.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list pods for dns deployment %s/%s: %v", dnsOperatorDeployment.Namespace, dnsOperatorDeployment.Name, err)
	}

	return dnsOperatorPods, nil
}

func upstreamTLSPod(name, ns, image string, configMap *corev1.ConfigMap) *corev1.Pod {
	coreContainer := upstreamContainer(name, image)
	volumeName := configMap.Name
	volumeMode := int32(420)

	items := []corev1.KeyToPath{}
	for k := range configMap.Data {
		items = append(items, corev1.KeyToPath{Key: k, Path: k})
	}
	volume := corev1.Volume{
		Name: "config-volume",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: volumeName,
				},
				Items:       items,
				DefaultMode: &volumeMode,
			},
		},
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{"test": "upstream-tls"},
		},
		Spec: corev1.PodSpec{
			Volumes:    []corev1.Volume{volume},
			Containers: []corev1.Container{coreContainer},
		},
	}
}

func getClient() (client.Client, error) {
	// Get a kube client.
	kubeConfig, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kube config: %v", err)
	}
	kubeClient, err := operatorclient.NewClient(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube client: %v", err)
	}
	return kubeClient, nil
}

// encodeCert returns a PEM block encoding the given certificate.
func encodeCert(cert *x509.Certificate) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}))
}

// encodeKey returns a PEM block encoding the given key.
func encodeKey(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

// generateServerCA generates and returns a CA certificate and key.
func generateServerCA() (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	root := &x509.Certificate{
		Subject:               pkix.Name{CommonName: "operator-e2e"},
		SignatureAlgorithm:    x509.SHA256WithRSA,
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		SerialNumber:          big.NewInt(1),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	der, err := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	certs, err := x509.ParseCertificates(der)
	if err != nil {
		return nil, nil, err
	}
	if len(certs) != 1 {
		return nil, nil, fmt.Errorf("expected a single certificate from x509.ParseCertificates, got %d: %v", len(certs), certs)
	}

	return certs[0], key, nil
}

// generateServerCertificate generates and returns a client certificate and key
// where the certificate is signed by the provided CA certificate.
func generateServerCertificate(caCert *x509.Certificate, caKey *rsa.PrivateKey, cn string) (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"OpenShift"},
		},
		SignatureAlgorithm:    x509.SHA256WithRSA,
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		SerialNumber:          big.NewInt(1),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{cn},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	certs, err := x509.ParseCertificates(derBytes)
	if err != nil {
		return nil, nil, err
	}
	if len(certs) != 1 {
		return nil, nil, fmt.Errorf("expected a single certificate from x509.ParseCertificates, got %d: %v", len(certs), certs)
	}

	return certs[0], key, nil
}
