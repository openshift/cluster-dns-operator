module github.com/openshift/cluster-dns-operator

go 1.15

require (
	github.com/apparentlymart/go-cidr v1.0.0
	github.com/google/go-cmp v0.5.2
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/kevinburke/go-bindata v3.11.0+incompatible
	github.com/openshift/api v0.0.0-20210325163602-e37aaed4c278
	github.com/sirupsen/logrus v1.6.0
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	sigs.k8s.io/controller-runtime v0.8.3
)
