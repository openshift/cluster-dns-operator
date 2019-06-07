module github.com/openshift/cluster-dns-operator

go 1.12

require (
	cloud.google.com/go v0.37.4 // indirect
	github.com/apparentlymart/go-cidr v1.0.0
	github.com/beorn7/perks v1.0.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/protobuf v1.3.1 // indirect
	github.com/google/go-cmp v0.2.0
	github.com/google/gofuzz v1.0.0 // indirect
	github.com/google/uuid v1.1.1 // indirect
	github.com/hashicorp/golang-lru v0.5.1 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/kevinburke/go-bindata v3.11.0+incompatible
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect

	github.com/openshift/api v3.9.1-0.20190607134528-2bb057a838e2+incompatible

	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709 // indirect
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90 // indirect
	github.com/prometheus/common v0.3.0 // indirect
	github.com/prometheus/procfs v0.0.0-20190416084830-8368d24ba045 // indirect
	github.com/sirupsen/logrus v1.4.1
	github.com/spf13/cobra v0.0.4 // indirect
	golang.org/x/crypto v0.0.0-20190411191339-88737f569e3a // indirect
	golang.org/x/oauth2 v0.0.0-20190402181905-9f3314589c9a // indirect
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	google.golang.org/appengine v1.5.0 // indirect

	k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/gengo v0.0.0-20190327210449-e17681d19d3a // indirect
	k8s.io/kube-openapi v0.0.0-20190401085232-94e1e7b7574c // indirect

	sigs.k8s.io/controller-runtime v0.2.0-beta.2
	sigs.k8s.io/controller-tools v0.2.0-beta.2
)
