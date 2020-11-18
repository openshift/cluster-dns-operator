module github.com/openshift/cluster-dns-operator

go 1.13

require (
	github.com/apparentlymart/go-cidr v1.0.0
	github.com/go-logr/logr v0.3.0 // indirect
	github.com/go-logr/zapr v0.2.0 // indirect
	github.com/google/go-cmp v0.4.0
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/kevinburke/go-bindata v3.11.0+incompatible
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/openshift/api v0.0.0-20201117184740-859beeffd973
	github.com/sirupsen/logrus v1.4.2
	golang.org/x/net v0.0.0-20201110031124-69a78807bb2b // indirect
	golang.org/x/text v0.3.4 // indirect
	k8s.io/api v0.19.4
	k8s.io/apimachinery v0.19.4
	k8s.io/client-go v0.19.2
	k8s.io/klog/v2 v2.4.0 // indirect
	sigs.k8s.io/controller-runtime v0.6.3
	sigs.k8s.io/structured-merge-diff/v4 v4.0.2 // indirect
)
