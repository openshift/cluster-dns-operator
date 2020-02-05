package main

import (
	"context"
	"net"
	"strings"
	"testing"
)

var tests = []struct {
	domain   string
	services []string
	ips      []string
	input    string
	expect   string
}{
	{
		domain:   "cluster.local",
		services: []string{"foo.svc"},
		ips:      []string{"172.30.29.198", "2607:f8b0:4002:c09::71"},
		input: `
# Host Database
127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4
::1         localhost localhost.localdomain localhost6 localhost6.localdomain6
`,
		expect: `
# Host Database
127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4
::1         localhost localhost.localdomain localhost6 localhost6.localdomain6
172.30.29.198 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
2607:f8b0:4002:c09::71 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
`,
	},
	{
		domain:   "cluster.local",
		services: []string{"foo.svc"},
		ips:      []string{"172.30.29.199", "2607:f8b0:4002:c09::72"},
		input: `
# Host Database
127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4
::1         localhost localhost.localdomain localhost6 localhost6.localdomain6
172.30.29.198 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
2607:f8b0:4002:c09::71 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
`,
		expect: `
# Host Database
127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4
::1         localhost localhost.localdomain localhost6 localhost6.localdomain6
172.30.29.199 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
2607:f8b0:4002:c09::72 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
`,
	},
	{
		domain:   "cluster.local",
		services: []string{"foo.svc"},
		ips:      []string{"2607:f8b0:4002:c09::72"},
		input: `
# Host Database
127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4
::1         localhost localhost.localdomain localhost6 localhost6.localdomain6
172.30.29.198 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
2607:f8b0:4002:c09::71 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
`,
		expect: `
# Host Database
127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4
::1         localhost localhost.localdomain localhost6 localhost6.localdomain6
2607:f8b0:4002:c09::72 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
`,
	},
	{
		domain:   "cluster.local",
		services: []string{"foo.svc"},
		ips:      []string{},
		input: `
# Host Database
127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4
::1         localhost localhost.localdomain localhost6 localhost6.localdomain6
172.30.29.198 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
2607:f8b0:4002:c09::71 foo.svc foo.svc.cluster.local # openshift-generated-node-resolver
`,
		expect: `
# Host Database
127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4
::1         localhost localhost.localdomain localhost6 localhost6.localdomain6
`,
	},
}

type resolveWith func(ctx context.Context, host string) ([]net.IPAddr, error)

func (fn resolveWith) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return fn(ctx, host)
}

func TestGenerate(t *testing.T) {
	for _, test := range tests {
		updater := &HostsUpdater{
			ClusterDomain: test.domain,
			Services:      test.services,
			resolver: resolveWith(func(ctx context.Context, host string) ([]net.IPAddr, error) {
				var ips []net.IPAddr
				for _, ip := range test.ips {
					ips = append(ips, net.IPAddr{IP: net.ParseIP(ip)})
				}
				return ips, nil
			}),
		}
		actual, err := updater.generate(strings.NewReader(test.input))
		if err != nil {
			t.Error(err)
		} else {
			if actual != test.expect {
				t.Errorf("expected:\n%s\ngot:\n%s", test.expect, actual)
			}
		}
	}
}
