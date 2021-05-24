package status

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComputeOperatorProgressingCondition(t *testing.T) {
	type versions struct {
		operator, operand string
	}

	testCases := []struct {
		description       string
		dnsMissing        bool
		dnsProgressing    bool
		oldVersions       versions
		newVersions       versions
		curVersions       versions
		expectProgressing configv1.ConditionStatus
	}{
		{
			description:       "dns does not exist",
			dnsMissing:        true,
			expectProgressing: configv1.ConditionTrue,
		},
		{
			description:       "dns progressing",
			dnsProgressing:    true,
			expectProgressing: configv1.ConditionTrue,
		},
		{
			description:       "versions match",
			oldVersions:       versions{"v1", "dns-v1"},
			newVersions:       versions{"v1", "dns-v1"},
			curVersions:       versions{"v1", "dns-v1"},
			expectProgressing: configv1.ConditionFalse,
		},
		{
			description:       "operator upgrade in progress",
			oldVersions:       versions{"v1", "dns-v1"},
			newVersions:       versions{"v2", "dns-v1"},
			curVersions:       versions{"v1", "dns-v1"},
			expectProgressing: configv1.ConditionTrue,
		},
		{
			description:       "operand upgrade in progress",
			oldVersions:       versions{"v1", "dns-v1"},
			newVersions:       versions{"v1", "dns-v2"},
			curVersions:       versions{"v1", "dns-v1"},
			expectProgressing: configv1.ConditionTrue,
		},
		{
			description:       "operator and operand upgrade in progress",
			oldVersions:       versions{"v1", "dns-v1"},
			newVersions:       versions{"v2", "dns-v2"},
			curVersions:       versions{"v1", "dns-v1"},
			expectProgressing: configv1.ConditionTrue,
		},
		{
			description:       "operator upgrade done",
			oldVersions:       versions{"v1", "dns-v1"},
			newVersions:       versions{"v2", "dns-v1"},
			curVersions:       versions{"v2", "dns-v1"},
			expectProgressing: configv1.ConditionFalse,
		},
		{
			description:       "operand upgrade done",
			oldVersions:       versions{"v1", "dns-v1"},
			newVersions:       versions{"v1", "dns-v2"},
			curVersions:       versions{"v1", "dns-v2"},
			expectProgressing: configv1.ConditionFalse,
		},
		{
			description:       "operator and operand upgrade done",
			oldVersions:       versions{"v1", "dns-v1"},
			newVersions:       versions{"v2", "dns-v2"},
			curVersions:       versions{"v2", "dns-v2"},
			expectProgressing: configv1.ConditionFalse,
		},
		{
			description:       "operator upgrade in progress, operand upgrade done",
			oldVersions:       versions{"v1", "dns-v1"},
			newVersions:       versions{"v2", "dns-v2"},
			curVersions:       versions{"v1", "dns-v2"},
			expectProgressing: configv1.ConditionTrue,
		},
	}

	for _, tc := range testCases {
		var (
			haveDNS bool
			dns     *operatorv1.DNS
		)
		if !tc.dnsMissing {
			haveDNS = true
			progressingStatus := operatorv1.ConditionFalse
			if tc.dnsProgressing {
				progressingStatus = operatorv1.ConditionTrue
			}
			dns = &operatorv1.DNS{
				Status: operatorv1.DNSStatus{
					Conditions: []operatorv1.OperatorCondition{{
						Type:   operatorv1.OperatorStatusTypeProgressing,
						Status: progressingStatus,
					}},
				},
			}
		}
		oldVersions := map[string]string{
			OperatorVersionName:     tc.oldVersions.operator,
			CoreDNSVersionName:      tc.oldVersions.operand,
			OpenshiftCLIVersionName: tc.oldVersions.operand,
			KubeRBACProxyName:       tc.oldVersions.operand,
		}
		newVersions := map[string]string{
			OperatorVersionName:     tc.newVersions.operator,
			CoreDNSVersionName:      tc.newVersions.operand,
			OpenshiftCLIVersionName: tc.newVersions.operand,
			KubeRBACProxyName:       tc.newVersions.operand,
		}
		curVersions := map[string]string{
			OperatorVersionName:     tc.curVersions.operator,
			CoreDNSVersionName:      tc.curVersions.operand,
			OpenshiftCLIVersionName: tc.curVersions.operand,
			KubeRBACProxyName:       tc.curVersions.operand,
		}

		expected := configv1.ClusterOperatorStatusCondition{
			Type:   configv1.OperatorProgressing,
			Status: tc.expectProgressing,
		}

		actual := computeOperatorProgressingCondition(haveDNS, dns, oldVersions, newVersions, curVersions)
		conditionsCmpOpts := []cmp.Option{
			cmpopts.IgnoreFields(configv1.ClusterOperatorStatusCondition{}, "LastTransitionTime", "Reason", "Message"),
		}
		if !cmp.Equal(actual, expected, conditionsCmpOpts...) {
			t.Errorf("%q: expected %#v, got %#v", tc.description, expected, actual)
		}
	}
}

func TestOperatorStatusesEqual(t *testing.T) {
	testCases := []struct {
		description string
		expected    bool
		a, b        configv1.ClusterOperatorStatus
	}{
		{
			description: "zero-valued ClusterOperatorStatus should be equal",
			expected:    true,
		},
		{
			description: "nil and non-nil slices are equal",
			expected:    true,
			a: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{},
			},
		},
		{
			description: "empty slices should be equal",
			expected:    true,
			a: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{},
			},
			b: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{},
			},
		},
		{
			description: "check no change in versions",
			expected:    true,
			a: configv1.ClusterOperatorStatus{
				Versions: []configv1.OperandVersion{
					{
						Name:    "operator",
						Version: "v1",
					},
					{
						Name:    "router",
						Version: "v2",
					},
				},
			},
			b: configv1.ClusterOperatorStatus{
				Versions: []configv1.OperandVersion{
					{
						Name:    "operator",
						Version: "v1",
					},
					{
						Name:    "router",
						Version: "v2",
					},
				},
			},
		},
		{
			description: "condition LastTransitionTime should not be ignored",
			expected:    false,
			a: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:               configv1.OperatorAvailable,
						Status:             configv1.ConditionTrue,
						LastTransitionTime: metav1.Unix(0, 0),
					},
				},
			},
			b: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:               configv1.OperatorAvailable,
						Status:             configv1.ConditionTrue,
						LastTransitionTime: metav1.Unix(1, 0),
					},
				},
			},
		},
		{
			description: "order of versions should not matter",
			expected:    true,
			a: configv1.ClusterOperatorStatus{
				Versions: []configv1.OperandVersion{
					{
						Name:    "operator",
						Version: "v1",
					},
					{
						Name:    "router",
						Version: "v2",
					},
				},
			},
			b: configv1.ClusterOperatorStatus{
				Versions: []configv1.OperandVersion{
					{
						Name:    "router",
						Version: "v2",
					},
					{
						Name:    "operator",
						Version: "v1",
					},
				},
			},
		},
		{
			description: "check missing related objects",
			expected:    false,
			a: configv1.ClusterOperatorStatus{
				RelatedObjects: []configv1.ObjectReference{
					{
						Name: "openshift-ingress",
					},
					{
						Name: "default",
					},
				},
			},
			b: configv1.ClusterOperatorStatus{
				RelatedObjects: []configv1.ObjectReference{
					{
						Name: "default",
					},
				},
			},
		},
		{
			description: "check extra related objects",
			expected:    false,
			a: configv1.ClusterOperatorStatus{
				RelatedObjects: []configv1.ObjectReference{
					{
						Name: "default",
					},
				},
			},
			b: configv1.ClusterOperatorStatus{
				RelatedObjects: []configv1.ObjectReference{
					{
						Name: "openshift-ingress",
					},
					{
						Name: "default",
					},
				},
			},
		},
		{
			description: "check condition reason differs",
			expected:    false,
			a: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:   configv1.OperatorAvailable,
						Status: configv1.ConditionFalse,
						Reason: "foo",
					},
				},
			},
			b: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:   configv1.OperatorAvailable,
						Status: configv1.ConditionFalse,
						Reason: "bar",
					},
				},
			},
		},
		{
			description: "check duplicate with single condition",
			expected:    false,
			a: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:    configv1.OperatorAvailable,
						Message: "foo",
					},
				},
			},
			b: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:    configv1.OperatorAvailable,
						Message: "foo",
					},
					{
						Type:    configv1.OperatorAvailable,
						Message: "foo",
					},
				},
			},
		},
		{
			description: "check duplicate with multiple conditions",
			expected:    false,
			a: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type: configv1.OperatorAvailable,
					},
					{
						Type: configv1.OperatorProgressing,
					},
					{
						Type: configv1.OperatorAvailable,
					},
				},
			},
			b: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type: configv1.OperatorProgressing,
					},
					{
						Type: configv1.OperatorAvailable,
					},
					{
						Type: configv1.OperatorProgressing,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		if actual := operatorStatusesEqual(tc.a, tc.b); actual != tc.expected {
			t.Errorf("%q: expected %v, got %v", tc.description, tc.expected, actual)
		}
	}
}

// TestComputeCurrentVersions verifies that computeCurrentVersions returns the
// expected values.
func TestComputeCurrentVersions(t *testing.T) {
	newPod := func(available bool, containers []corev1.Container) corev1.Pod {
		readyStatus := corev1.ConditionFalse
		if available {
			readyStatus = corev1.ConditionTrue
		}
		anHourAgo := metav1.Time{Time: clock.Now().Add(-1 * time.Hour)}
		return corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: containers,
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{{
					Type:               corev1.PodReady,
					Status:             readyStatus,
					LastTransitionTime: anHourAgo,
				}},
			},
		}
	}
	dnsPod := func(available bool, version string) corev1.Pod {
		containers := []corev1.Container{
			{
				Name:  "dns",
				Image: version,
			},
			{
				Name:  "kube-rbac-proxy",
				Image: version,
			},
		}
		return newPod(available, containers)
	}
	nodeResolverPod := func(available bool, version string) corev1.Pod {
		containers := []corev1.Container{
			{Name: "dns-node-resolver", Image: version},
		}
		return newPod(available, containers)
	}
	type versions struct {
		operator      string
		coredns       string
		kubeRBACProxy string
		openshiftCLI  string
	}

	testCases := []struct {
		description      string
		oldVersions      versions
		newVersions      versions
		dnsPods          []corev1.Pod
		nodeResolverPods []corev1.Pod
		expectedVersions versions
	}{
		{
			description: "initialize versions, operands at level",
			oldVersions: versions{"unknown", "unknown", "unknown", "unknown"},
			newVersions: versions{"v1", "v1", "v1", "v1"},
			dnsPods: []corev1.Pod{
				dnsPod(true, "v1"),
				dnsPod(true, "v1"),
				dnsPod(true, "v1"),
			},
			nodeResolverPods: []corev1.Pod{
				nodeResolverPod(true, "v1"),
				nodeResolverPod(true, "v1"),
				nodeResolverPod(true, "v1"),
			},
			expectedVersions: versions{"v1", "v1", "v1", "v1"},
		},
		{
			description: "starting upgrade",
			oldVersions: versions{"v1", "v1", "v1", "v1"},
			newVersions: versions{"v2", "v2", "v2", "v2"},
			dnsPods: []corev1.Pod{
				dnsPod(true, "v1"),
				dnsPod(true, "v1"),
				dnsPod(true, "v1"),
			},
			nodeResolverPods: []corev1.Pod{
				nodeResolverPod(true, "v1"),
				nodeResolverPod(true, "v1"),
				nodeResolverPod(true, "v1"),
			},
			expectedVersions: versions{"v1", "v1", "v1", "v1"},
		},
		{
			description: "partly upgraded",
			oldVersions: versions{"v1", "v1", "v1", "v1"},
			newVersions: versions{"v2", "v2", "v2", "v2"},
			dnsPods: []corev1.Pod{
				dnsPod(true, "v2"),
				dnsPod(false, "v1"),
				dnsPod(true, "v1"),
				dnsPod(true, "v1"),
			},
			nodeResolverPods: []corev1.Pod{
				nodeResolverPod(true, "v2"),
				nodeResolverPod(false, "v1"),
				nodeResolverPod(true, "v1"),
				nodeResolverPod(true, "v1"),
			},
			expectedVersions: versions{"v1", "v1", "v1", "v1"},
		},
		{
			description: "partly upgraded, extra pods",
			oldVersions: versions{"v1", "v1", "v1", "v1"},
			newVersions: versions{"v2", "v2", "v2", "v2"},
			dnsPods: []corev1.Pod{
				dnsPod(false, "v3"),
				dnsPod(true, "v3"),
				dnsPod(false, "v2"),
				dnsPod(true, "v2"),
				dnsPod(true, "v2"),
				dnsPod(true, "v1"),
				dnsPod(true, "v1"),
				dnsPod(true, "v1"),
			},
			nodeResolverPods: []corev1.Pod{
				nodeResolverPod(false, "v3"),
				nodeResolverPod(true, "v3"),
				nodeResolverPod(true, "v3"),
				nodeResolverPod(false, "v2"),
				nodeResolverPod(true, "v2"),
				nodeResolverPod(true, "v1"),
				nodeResolverPod(true, "v1"),
				nodeResolverPod(true, "v1"),
			},
			expectedVersions: versions{"v1", "v1", "v1", "v1"},
		},
		{
			description: "dns upgraded, node-resolver upgrading",
			oldVersions: versions{"v1", "v1", "v1", "v1"},
			newVersions: versions{"v2", "v2", "v2", "v2"},
			dnsPods: []corev1.Pod{
				dnsPod(true, "v2"),
				dnsPod(true, "v2"),
				dnsPod(true, "v2"),
			},
			nodeResolverPods: []corev1.Pod{
				nodeResolverPod(true, "v2"),
				nodeResolverPod(true, "v2"),
				nodeResolverPod(true, "v1"),
			},
			expectedVersions: versions{"v1", "v2", "v2", "v1"},
		},
		{
			description: "dns upgrading, node-resolver upgraded",
			oldVersions: versions{"v1", "v1", "v1", "v1"},
			newVersions: versions{"v2", "v2", "v2", "v2"},
			dnsPods: []corev1.Pod{
				dnsPod(true, "v1"),
				dnsPod(true, "v1"),
				dnsPod(true, "v2"),
			},
			nodeResolverPods: []corev1.Pod{
				nodeResolverPod(true, "v2"),
				nodeResolverPod(true, "v2"),
				nodeResolverPod(true, "v2"),
			},
			expectedVersions: versions{"v1", "v1", "v1", "v2"},
		},
		{
			description: "dns upgraded, node-resolver upgraded",
			oldVersions: versions{"v1", "v1", "v1", "v1"},
			newVersions: versions{"v2", "v2", "v2", "v2"},
			dnsPods: []corev1.Pod{
				dnsPod(true, "v2"),
				dnsPod(true, "v2"),
				dnsPod(true, "v2"),
			},
			nodeResolverPods: []corev1.Pod{
				nodeResolverPod(true, "v2"),
				nodeResolverPod(true, "v2"),
				nodeResolverPod(true, "v2"),
			},
			expectedVersions: versions{"v2", "v2", "v2", "v2"},
		},
		{
			description: "upgraded, extra pods",
			oldVersions: versions{"v1", "v1", "v1", "v1"},
			newVersions: versions{"v2", "v2", "v2", "v2"},
			dnsPods: []corev1.Pod{
				dnsPod(true, "v2"),
				dnsPod(true, "v2"),
				dnsPod(true, "v2"),
				dnsPod(true, "v3"),
			},
			nodeResolverPods: []corev1.Pod{
				nodeResolverPod(true, "v1"),
				nodeResolverPod(true, "v2"),
				nodeResolverPod(true, "v2"),
				nodeResolverPod(true, "v2"),
			},
			expectedVersions: versions{"v2", "v2", "v2", "v2"},
		},
	}

	for _, tc := range testCases {
		oldVersions := computeOldVersions([]configv1.OperandVersion{
			{
				Name:    OperatorVersionName,
				Version: tc.oldVersions.operator,
			},
			{
				Name:    CoreDNSVersionName,
				Version: tc.oldVersions.coredns,
			},
			{
				Name:    OpenshiftCLIVersionName,
				Version: tc.oldVersions.openshiftCLI,
			},
			{
				Name:    KubeRBACProxyName,
				Version: tc.oldVersions.kubeRBACProxy,
			},
		})
		newVersions := computeNewVersions(
			tc.newVersions.operator,
			tc.newVersions.coredns,
			tc.newVersions.openshiftCLI,
			tc.newVersions.kubeRBACProxy,
		)
		dnsDaemonSet := &appsv1.DaemonSet{
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: int32(3),
			},
		}
		nodeResolverDaemonSet := &appsv1.DaemonSet{
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: int32(3),
			},
		}
		expectedVersions := map[string]string{
			OperatorVersionName:     tc.expectedVersions.operator,
			CoreDNSVersionName:      tc.expectedVersions.coredns,
			OpenshiftCLIVersionName: tc.expectedVersions.openshiftCLI,
			KubeRBACProxyName:       tc.expectedVersions.kubeRBACProxy,
		}
		versions := computeCurrentVersions(oldVersions, newVersions, dnsDaemonSet, nodeResolverDaemonSet, tc.dnsPods, tc.nodeResolverPods)
		if !cmp.Equal(versions, expectedVersions, cmpopts.EquateEmpty()) {
			t.Errorf("%q: expected %v, got %v", tc.description, expectedVersions, versions)
		}
	}
}
