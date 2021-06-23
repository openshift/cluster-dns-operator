module github.com/openshift/cluster-dns-operator

go 1.16

require (
	github.com/apparentlymart/go-cidr v1.0.0
	github.com/google/go-cmp v0.5.5
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/kevinburke/go-bindata v3.11.0+incompatible
	github.com/openshift/api v0.0.0-20210601190906-e782d76c7bc4
	github.com/openshift/build-machinery-go v0.0.0-20210423112049-9415d7ebd33e
	github.com/sirupsen/logrus v1.7.0
	golang.org/x/net v0.0.0-20210510120150-4163338589ed // indirect
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
	k8s.io/kubectl v0.21.0
	sigs.k8s.io/controller-runtime v0.9.0
	sigs.k8s.io/structured-merge-diff/v4 v4.1.1 // indirect
)
