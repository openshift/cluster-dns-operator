package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/operator-framework/operator-sdk/pkg/sdk"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestMain(m *testing.M) {
	// When the e2e tests are run, we have no guarantees that the cluster
	// dns operator is started up. First baby step here is to ensure that
	// the cluster dns operator deployment exists.
	if err := waitForDNSOperatorDeployment(); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(74)
	}

	os.Exit(m.Run())
}

// waitForDNSOperatorDeployment waits for the cluster dns operator deployment
// to be created.
func waitForDNSOperatorDeployment() error {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "openshift-cluster-dns-operator",
			Name:      "cluster-dns-operator",
		},
	}

	deploymentExists := func() (bool, error) {
		if err := sdk.Get(d); err != nil {
			return false, nil
		}

		return true, nil
	}

	if err := wait.PollImmediate(5*time.Second, 10*time.Minute, deploymentExists); err != nil {
		return fmt.Errorf("waiting for ClusterDNS operator deployment %s/%s to be created: %v", d.Namespace, d.Name, err)
	}

	return nil
}
