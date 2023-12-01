package dnsnameresolver

import (
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	networkv1alpha1 "github.com/openshift/api/network/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type addParams struct {
	dnsName           string
	resolvedAddresses []networkv1alpha1.DNSNameResolverResolvedAddress
	matchesRegular    bool
	objName           string
}

type deleteParams struct {
	objName          string
	isDNSNameRemoved bool
	numRemoved       int
}

func TestResolver(t *testing.T) {
	tests := []struct {
		name                   string
		action                 string
		parameters             interface{}
		expectedNextDNSName    string
		expectedNextLookupTime time.Time
		expectedNumIPs         int
		expectedOutput         bool
	}{
		{
			name:   "Add a resolved name belonging to a regular DNSNameResolver object",
			action: "Add",
			parameters: &addParams{
				dnsName: "www.example.com.",
				resolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
					{
						IP:             "1.1.1.1",
						TTLSeconds:     10,
						LastLookupTime: &v1.Time{Time: time.Now()},
					},
				},
				matchesRegular: true,
				objName:        "regular",
			},
			expectedNextDNSName:    "www.example.com.",
			expectedNextLookupTime: time.Now().Add(10 * time.Second),
			expectedNumIPs:         1,
			expectedOutput:         true,
		},
		{
			name:   "Add a wildcard resolved name belonging to a wildcard DNSNameResolver object",
			action: "Add",
			parameters: &addParams{
				dnsName: "*.example.com.",
				resolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
					{
						IP:             "1.1.1.2",
						TTLSeconds:     8,
						LastLookupTime: &v1.Time{Time: time.Now()},
					},
					{
						IP:             "1.1.1.3",
						TTLSeconds:     8,
						LastLookupTime: &v1.Time{Time: time.Now()},
					},
				},
				matchesRegular: false,
				objName:        "wildcard",
			},
			expectedNextDNSName:    "*.example.com.",
			expectedNextLookupTime: time.Now().Add(8 * time.Second),
			expectedNumIPs:         2,
			expectedOutput:         true,
		},
		{
			name:   "Add a regular resolved name belonging to a wildcard DNSNameResolver object",
			action: "Add",
			parameters: &addParams{
				dnsName: "www.example.com.",
				resolvedAddresses: []networkv1alpha1.DNSNameResolverResolvedAddress{
					{
						IP:             "1.1.1.1",
						TTLSeconds:     10,
						LastLookupTime: &v1.Time{Time: time.Now()},
					},
				},
				matchesRegular: false,
				objName:        "wildcard",
			},
			expectedNextDNSName:    "*.example.com.",
			expectedNextLookupTime: time.Now().Add(8 * time.Second),
			expectedNumIPs:         2,
			expectedOutput:         true,
		},
		{
			name:   "Delete the regular DNSNameResolver object",
			action: "Delete",
			parameters: &deleteParams{
				objName:          "regular",
				isDNSNameRemoved: false,
			},
			expectedNextDNSName:    "*.example.com.",
			expectedNextLookupTime: time.Now().Add(8 * time.Second),
			expectedNumIPs:         2,
			expectedOutput:         true,
		},
		{
			name:   "Delete the wildcard DNSNameResolver object",
			action: "Delete",
			parameters: &deleteParams{
				objName:          "wildcard",
				isDNSNameRemoved: true,
				numRemoved:       2,
			},
			expectedNextDNSName: "",
			expectedNumIPs:      0,
			expectedOutput:      false,
		},
	}

	// Create the Resolver object.
	res := NewResolver(nil, "")
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			switch tc.action {
			case "Add":
				// Get the parameters for the Add action.
				params := tc.parameters.(*addParams)

				// Call Add with the parameters in a separate goroutine.
				go res.Add(params.dnsName, params.resolvedAddresses, params.matchesRegular, params.objName)

				// Wait for the signal on the res.added channel.
				<-res.added
			case "Delete":
				// Get the parameters for the Delete action.
				params := tc.parameters.(*deleteParams)

				// Call Delete with the parameters in a separate goroutine. Use wait group to wait for the
				// call to complete.
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					res.Delete(params.objName)
					wg.Done()
				}()

				// If DNS names are removed then wait for the signals on the res.deleted channel.
				if params.isDNSNameRemoved {
					for i := 0; i < params.numRemoved; i++ {
						<-res.deleted
					}
				}

				// Wait for the Delete call to complete.
				wg.Wait()
			default:
				assert.FailNow(t, "unknown action")
			}

			// Get the details of the next DNS name to be looked up.
			nextDNSName, nextLookupTime, numIPs, exists := res.getNextDNSNameDetails()
			assert.Equal(t, tc.expectedNextDNSName, nextDNSName)
			assert.Equal(t, tc.expectedNumIPs, numIPs)
			assert.Equal(t, tc.expectedOutput, exists)
			if exists {
				cmpOpts := cmpopts.EquateApproxTime(100 * time.Millisecond)
				diff := cmp.Diff(tc.expectedNextLookupTime, nextLookupTime, cmpOpts)
				if diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}
