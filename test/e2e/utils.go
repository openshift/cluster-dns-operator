// +build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
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

// lookForString looks for the given string using cmd and args every
// 2 seconds until the timeout is reached or the string is found.
// Returns an error if the string was not found.
func lookForString(cmd string, args []string, expectedString string, timeout time.Duration) error {
	err := wait.PollImmediate(2*time.Second, timeout, func() (bool, error) {
		result, err := runCmd(cmd, args)
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
		Handler: corev1.Handler{
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
