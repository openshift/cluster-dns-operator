module github.com/openshift/cluster-dns-operator

go 1.16

require (
	github.com/apparentlymart/go-cidr v1.0.0
	github.com/google/go-cmp v0.5.5
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/kevinburke/go-bindata v3.11.0+incompatible
	github.com/openshift/api v0.0.0-20211019100638-b2cbe79f2e4b
	github.com/openshift/build-machinery-go v0.0.0-20210712174854-1bb7fd1518d3
	github.com/sirupsen/logrus v1.7.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/kubectl v0.21.0
	sigs.k8s.io/controller-runtime v0.9.0
)
