module github.com/openshift/cluster-dns-operator

go 1.12

require (
	github.com/apparentlymart/go-cidr v1.0.0
	github.com/go-logr/zapr v0.1.1 // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/google/go-cmp v0.3.0
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/kevinburke/go-bindata v3.11.0+incompatible
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect

	github.com/openshift/api v3.9.1-0.20191105205100-610e47be70db+incompatible
	github.com/sirupsen/logrus v1.4.2
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect

	// kubernetes-1.16.0
	k8s.io/api v0.0.0-20190918155943-95b840bb6a1f
	k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655
	k8s.io/client-go v0.0.0-20190918160344-1fbdaa4c8d90

	sigs.k8s.io/controller-runtime v0.3.1-0.20191011155846-b2bc3490f2e3
)
