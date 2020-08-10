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
	golang.org/x/text v0.3.3 // indirect
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v0.18.3
	sigs.k8s.io/controller-runtime v0.6.0
)
