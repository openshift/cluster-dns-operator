module github.com/openshift/cluster-dns-operator

go 1.13

require (
	github.com/apparentlymart/go-cidr v1.0.0
	github.com/go-logr/zapr v0.1.1 // indirect
	github.com/google/go-cmp v0.3.0
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/kevinburke/go-bindata v3.11.0+incompatible
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect

	github.com/openshift/api v0.0.0-20200324173355-9b3bdf846ea1

	github.com/sirupsen/logrus v1.4.2

	k8s.io/api v0.18.0-rc.1
	k8s.io/apimachinery v0.18.0-rc.1
	k8s.io/client-go v0.18.0-rc.1

	sigs.k8s.io/controller-runtime v0.3.1-0.20191011155846-b2bc3490f2e3
)

replace (
	// Remove when https://github.com/kubernetes-sigs/controller-runtime/pull/836 merges.
	sigs.k8s.io/controller-runtime => github.com/munnerz/controller-runtime v0.1.8-0.20200318092001-e22ac1073450
	// Remove when https://github.com/kubernetes-sigs/controller-tools/pull/424 merges.
	sigs.k8s.io/controller-tools => github.com/munnerz/controller-tools v0.1.10-0.20200323145043-a2d268fbf03d
)
