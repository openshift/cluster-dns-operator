module github.com/openshift/cluster-dns-operator

go 1.15

require (
	github.com/apparentlymart/go-cidr v1.0.0
	github.com/google/go-cmp v0.5.2
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/kevinburke/go-bindata v3.11.0+incompatible
	github.com/openshift/api v0.0.0-20210405165116-47be53705a13
	github.com/sirupsen/logrus v1.6.0
	k8s.io/api v0.21.0-rc.0
	k8s.io/apimachinery v0.21.0-rc.0
	k8s.io/client-go v0.21.0-rc.0
	sigs.k8s.io/controller-runtime v0.9.0-alpha.1
)

replace github.com/openshift/api => github.com/rfredette/api v0.0.0-20210408165110-1f0361fb45cf
