package dnsnameresolver

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	networkv1alpha1 "github.com/openshift/api/network/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsWildcard(t *testing.T) {
	tests := []struct {
		dnsName        string
		expectedOutput bool
	}{
		// success
		{
			dnsName:        "*.example.com",
			expectedOutput: true,
		},
		{
			dnsName:        "*.sub1.example.com",
			expectedOutput: true,
		},
		// negative
		{
			dnsName:        "www.example.com",
			expectedOutput: false,
		},
		{
			dnsName:        "sub2.sub1.example.com",
			expectedOutput: false,
		},
	}

	for _, tc := range tests {
		actualOutput := isWildcard(tc.dnsName)
		assert.Equal(t, tc.expectedOutput, actualOutput)
	}
}

func TestRemovalOfIPsRequired(t *testing.T) {
	tests := []struct {
		name           string
		status         *networkv1alpha1.DNSNameResolverStatus
		expectedStatus *networkv1alpha1.DNSNameResolverStatus
		expectedOutput bool
	}{
		{
			name: "None of the TTL of the IP addresses expired",
			status: &networkv1alpha1.DNSNameResolverStatus{
				ResolvedNames: []networkv1alpha1.DNSNameResolverResolvedName{
					{
						DNSName: "*.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.1",
								TTLSeconds:     10,
								LastLookupTime: &v1.Time{Time: time.Now()},
							},
							{
								IP:             "1.1.1.2",
								TTLSeconds:     8,
								LastLookupTime: &v1.Time{Time: time.Now()},
							},
						},
					},
					{
						DNSName: "www.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.3",
								TTLSeconds:     30,
								LastLookupTime: &v1.Time{Time: time.Now()},
							},
						},
					},
				},
			},
			expectedStatus: &networkv1alpha1.DNSNameResolverStatus{
				ResolvedNames: []networkv1alpha1.DNSNameResolverResolvedName{
					{
						DNSName: "*.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:         "1.1.1.1",
								TTLSeconds: 10,
							},
							{
								IP:         "1.1.1.2",
								TTLSeconds: 8,
							},
						},
					},
					{
						DNSName: "www.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:         "1.1.1.3",
								TTLSeconds: 30,
							},
						},
					},
				},
			},
			expectedOutput: false,
		},
		{
			name: "The TTL of IP addresses expired, but grace period not over",
			status: &networkv1alpha1.DNSNameResolverStatus{
				ResolvedNames: []networkv1alpha1.DNSNameResolverResolvedName{
					{
						DNSName: "*.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.1",
								TTLSeconds:     10,
								LastLookupTime: &v1.Time{Time: time.Now().Add(-12 * time.Second)},
							},
							{
								IP:             "1.1.1.2",
								TTLSeconds:     8,
								LastLookupTime: &v1.Time{Time: time.Now().Add(-12 * time.Second)},
							},
						},
					},
					{
						DNSName: "www.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.3",
								TTLSeconds:     30,
								LastLookupTime: &v1.Time{Time: time.Now().Add(-12 * time.Second)},
							},
						},
					},
				},
			},
			expectedStatus: &networkv1alpha1.DNSNameResolverStatus{
				ResolvedNames: []networkv1alpha1.DNSNameResolverResolvedName{
					{
						DNSName: "*.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:         "1.1.1.1",
								TTLSeconds: 10,
							},
							{
								IP:         "1.1.1.2",
								TTLSeconds: 8,
							},
						},
					},
					{
						DNSName: "www.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:         "1.1.1.3",
								TTLSeconds: 30,
							},
						},
					},
				},
			},
			expectedOutput: false,
		},
		{
			name: "The TTL of IP addresses expired, and grace period of one IP address is also over",
			status: &networkv1alpha1.DNSNameResolverStatus{
				ResolvedNames: []networkv1alpha1.DNSNameResolverResolvedName{
					{
						DNSName: "*.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.1",
								TTLSeconds:     10,
								LastLookupTime: &v1.Time{Time: time.Now().Add(-14 * time.Second)},
							},
							{
								IP:             "1.1.1.2",
								TTLSeconds:     8,
								LastLookupTime: &v1.Time{Time: time.Now().Add(-14 * time.Second)},
							},
						},
					},
					{
						DNSName: "www.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.3",
								TTLSeconds:     30,
								LastLookupTime: &v1.Time{Time: time.Now().Add(-14 * time.Second)},
							},
						},
					},
				},
			},
			expectedStatus: &networkv1alpha1.DNSNameResolverStatus{
				ResolvedNames: []networkv1alpha1.DNSNameResolverResolvedName{
					{
						DNSName: "*.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:         "1.1.1.1",
								TTLSeconds: 10,
							},
						},
					},
					{
						DNSName: "www.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:         "1.1.1.3",
								TTLSeconds: 30,
							},
						},
					},
				},
			},
			expectedOutput: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actualOutput := removalOfIPsRequired(tc.status)
			assert.Equal(t, tc.expectedOutput, actualOutput)
			cmpOpts := []cmp.Option{
				cmpopts.IgnoreFields(networkv1alpha1.DNSNameResolverResolvedAddress{}, "LastLookupTime"),
				cmpopts.EquateApproxTime(100 * time.Millisecond),
				cmpopts.SortSlices(func(elem1, elem2 networkv1alpha1.DNSNameResolverResolvedAddress) bool {
					return elem1.IP > elem2.IP
				}),
			}
			diff := cmp.Diff(tc.expectedStatus, tc.status, cmpOpts...)
			if diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestReconcileRequired(t *testing.T) {
	tests := []struct {
		name                  string
		status                *networkv1alpha1.DNSNameResolverStatus
		expectedTTLExpired    bool
		expectedRemainingTime time.Duration
	}{
		{
			name: "TTL expired of one IP address of a resolved name",
			status: &networkv1alpha1.DNSNameResolverStatus{
				ResolvedNames: []networkv1alpha1.DNSNameResolverResolvedName{
					{
						DNSName: "*.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.1",
								TTLSeconds:     10,
								LastLookupTime: &v1.Time{Time: time.Now()},
							},
						},
					},
					{
						DNSName: "www.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.2",
								TTLSeconds:     4,
								LastLookupTime: &v1.Time{Time: time.Now().Add(-5 * time.Second)},
							},
						},
					},
				},
			},
			expectedTTLExpired:    true,
			expectedRemainingTime: ipRemovalGracePeriod - 1*time.Second,
		},
		{
			name: "TTL expired of IP addresses of different resolved names, return minimum remaining time till grace period",
			status: &networkv1alpha1.DNSNameResolverStatus{
				ResolvedNames: []networkv1alpha1.DNSNameResolverResolvedName{
					{
						DNSName: "*.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.1",
								TTLSeconds:     10,
								LastLookupTime: &v1.Time{Time: time.Now().Add(-12 * time.Second)},
							},
						},
					},
					{
						DNSName: "www.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.2",
								TTLSeconds:     4,
								LastLookupTime: &v1.Time{Time: time.Now().Add(-5 * time.Second)},
							},
						},
					},
				},
			},
			expectedTTLExpired:    true,
			expectedRemainingTime: ipRemovalGracePeriod - 2*time.Second,
		},
		{
			name: "TTL not expired of any IP address",
			status: &networkv1alpha1.DNSNameResolverStatus{
				ResolvedNames: []networkv1alpha1.DNSNameResolverResolvedName{
					{
						DNSName: "*.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.1",
								TTLSeconds:     10,
								LastLookupTime: &v1.Time{Time: time.Now()},
							},
						},
					},
					{
						DNSName: "www.example.com.",
						ResolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
							{
								IP:             "1.1.1.2",
								TTLSeconds:     4,
								LastLookupTime: &v1.Time{Time: time.Now()},
							},
						},
					},
				},
			},
			expectedTTLExpired:    false,
			expectedRemainingTime: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actualTTLExpired, actualRemainingTime := reconcileRequired(tc.status)
			assert.Equal(t, tc.expectedTTLExpired, actualTTLExpired)
			if actualTTLExpired {
				cmOpts := []cmp.Option{
					cmpopts.EquateApproxTime(100 * time.Millisecond),
				}
				if !cmp.Equal(time.Now().Add(actualRemainingTime), time.Now().Add(tc.expectedRemainingTime), cmOpts...) {
					t.Fatalf("expected remaining time: %v, actual remaining time: %v", tc.expectedRemainingTime, actualRemainingTime)
				}
			}
		})
	}
}
