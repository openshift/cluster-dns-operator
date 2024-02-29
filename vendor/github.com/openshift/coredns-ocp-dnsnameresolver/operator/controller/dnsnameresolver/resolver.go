package dnsnameresolver

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
	networkv1alpha1 "github.com/openshift/api/network/v1alpha1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultMaxTTL    = 30 * time.Minute
	defaultMinTTL    = 5 * time.Second
	defaultPort      = "53"
	maxCoreDNSPodIPs = 5
)

// dnsResolvedName contains details about a DNS resolved name.
type dnsResolvedName struct {
	minNextLookupTime time.Time
	regularObjExists  bool
	wildcardObjExists bool
	numIPs            int
}

// Resolver keeps track of the DNS names, the corresponding IP addresses and TTLs. It sends DNS resolution
// requests to CoreDNS pods whenever TTL of any IP address associated to a DNS name expires.
type Resolver struct {
	corednsEndpointSliceCache cache.Cache
	serviceName               string
	dnsNames                  map[string]dnsResolvedName
	regularObjInfo            map[string]string
	wildcardObjInfo           map[string]sets.Set[string]
	dnsLock                   sync.Mutex

	added   chan struct{}
	deleted chan string
}

// NewResolver initializes and returns a new resolver instance.
func NewResolver(cache cache.Cache, serviceName string) *Resolver {
	return &Resolver{
		corednsEndpointSliceCache: cache,
		serviceName:               serviceName,
		dnsNames:                  make(map[string]dnsResolvedName),
		regularObjInfo:            make(map[string]string),
		wildcardObjInfo:           make(map[string]sets.Set[string]),
		added:                     make(chan struct{}, 1),
		deleted:                   make(chan string, 1),
	}
}

// Start starts a goroutine for sending DNS lookup requests of the DNS names on expiration of the corresponding TTLs.
func (res *Resolver) Start() {
	var nextDNSName, deletedDNSName string
	var nextLookupTime time.Time
	var exists bool
	var numIPs int
	// Initially the check for nextDNSName to be looked up will happen after default maximum TTL. If a DNS name
	// gets added before that then the nextDNSName and timeTillNextLookup will be updated.
	timeTillNextLookup := defaultMaxTTL
	go func() {
		timer := time.NewTicker(timeTillNextLookup)
		defer timer.Stop()
		for {
			select {
			case <-res.added:
				// Whenever DNS name(s) are added or updated, the nextDNSName and timeTillNextLookup
				// needs to be updated.
			case <-timer.C:
				// After every tick, DNS lookup needs to be performed for nextDNSName, if it is not empty.
				// The nextDNSName and timeTillNextLookup also needs to be updated.
				if len(nextDNSName) > 0 {
					if err := res.lookupDNSName(nextDNSName, numIPs); err != nil {
						controllerLog.Info("Warning: Encountered error while looking up DNS name", "DNS name", nextDNSName, "Error", err)
					}
				}
			case deletedDNSName = <-res.deleted:
				// If the deleted DNS name is the DNS name that will be looked up next, then
				// the nextDNSName and timeTillNextLookup needs to be updated. If another
				// DNS name is deleted then nothing is needed to be done.
				if nextDNSName != deletedDNSName {
					continue
				}
			}
			// Get the DNS name whose TTL will expire first. Based on the nextLookupTime for the nextDNSName,
			// the timeTillNextLookup will be updated appropriately.
			nextDNSName, nextLookupTime, numIPs, exists = res.getNextDNSNameDetails()
			remainingDuration := time.Until(nextLookupTime)
			if !exists || remainingDuration > defaultMaxTTL {
				// If no DNS name is found OR If the remaining duration is greater than default maximum TTL, then perform DNS lookup
				// after default maximum TTL.
				timeTillNextLookup = defaultMaxTTL
			} else if remainingDuration.Seconds() > 0 {
				// If the remaining duration is positive and less than default maximum TTL, then perform DNS lookup
				// after the remaining duration.
				timeTillNextLookup = remainingDuration
			} else {
				// TTL of the DNS name has already expired, so send DNS lookup request as soon as possible.
				timeTillNextLookup = 1 * time.Millisecond
			}
			timer.Reset(timeTillNextLookup)
		}
	}()
}

// Add is called whenever a DNSNameResolver object is added or updated.
func (res *Resolver) Add(
	dnsName string,
	resolvedAddresses []networkv1alpha1.DNSNameResolverResolvedAddress,
	matchesRegular bool,
	objName string,
) {
	res.dnsLock.Lock()
	defer res.dnsLock.Unlock()

	// Get the details of the resolved name corresponding to the DNS name, if it exists.
	resolvedName, exists := res.dnsNames[dnsName]

	// If the resolved name corresponding to the DNS name exists, ensure that the DNSNameResolver
	// object matches the existing information. Otherwise, don't proceed.
	if exists {
		if resolvedName.regularObjExists && matchesRegular {
			// If the DNSNameResolver object for the regular DNS name object exists and the
			// current Add call is also for an object corresponding to a regular DNS name,
			// then check if the existing DNS name matches the current one.
			matchingDNSName, found := res.regularObjInfo[objName]
			if !found || dnsName != matchingDNSName {
				return
			}
		} else if resolvedName.wildcardObjExists && !matchesRegular {
			// If the DNSNameResolver object for the wildcard DNS name object exists and the
			// current Add call is also for an object corresponding to a wildcard DNS name,
			// then check if the current DNS name exists in the set of DNS names matching the
			// wildcard DNS name.
			dnsNamesMatchingWildcard, found := res.wildcardObjInfo[objName]
			if !found || !dnsNamesMatchingWildcard.Has(dnsName) {
				return
			}
		}
	}

	if len(resolvedAddresses) == 0 {
		// If no IP address is currently associated with the DNS name and the corresponding
		// resolved name details also do not exist, then set the next lookup time for the
		// DNS name as the default maximum TTL. Spawn a goroutine to send a DNS lookup
		// request for the DNS name.
		if !exists {
			resolvedName.minNextLookupTime = time.Now().Add(defaultMaxTTL)
			go res.lookupDNSName(dnsName, 0)
		}
	} else {
		// If some IP addresses are associated with the DNS name, then determine the minimum
		// next lookup time among the associated IP addresses.
		first := false
		for _, resolvedAddress := range resolvedAddresses {
			isBeforeMinNextLookupTime := resolvedAddress.LastLookupTime.Time.Add(
				time.Second * time.Duration(resolvedAddress.TTLSeconds)).Before(resolvedName.minNextLookupTime)
			if !first || isBeforeMinNextLookupTime {
				resolvedName.minNextLookupTime =
					resolvedAddress.LastLookupTime.Time.Add(time.Second * time.Duration(resolvedAddress.TTLSeconds))
				first = true
			}
		}
	}

	// Get the number of IP addresses associated with the DNS name.
	resolvedName.numIPs = len(resolvedAddresses)

	// Check if the DNSNameResolver object is corresponding to a regular DNS name. If so, set
	// regularObjExists to true and add the DNS name corresponding to the DNSNameResolver
	// object name in the regularObjInfo map. Otherwise, set wildcardObjExists to true and
	// add the DNS name to the DNS name set matching the wildcard DNS name and add the set
	// corresponding to the DNSNameResolver object name in the wildcardObjInfo map.
	if matchesRegular {
		resolvedName.regularObjExists = true
		res.regularObjInfo[objName] = dnsName
	} else {
		resolvedName.wildcardObjExists = true
		dnsNamesMatchingWildcard, exists := res.wildcardObjInfo[objName]
		if !exists {
			dnsNamesMatchingWildcard = sets.New[string]()
		}
		dnsNamesMatchingWildcard.Insert(dnsName)
		res.wildcardObjInfo[objName] = dnsNamesMatchingWildcard
	}

	// Add the updated resolved name to the dnsNames map corresponding to the DNS name.
	res.dnsNames[dnsName] = resolvedName

	// Send a signal to the added channel indicating that details corresponding to a DNS
	// name have been added or updated.
	res.added <- struct{}{}
}

// Delete is called whenever a DNSNameResolver object is deleted.
func (res *Resolver) Delete(objName string) {
	res.dnsLock.Lock()
	defer res.dnsLock.Unlock()

	var matchesRegular bool
	dnsNameList := []string{}

	regularDNSName, regularExists := res.regularObjInfo[objName]
	if regularExists {
		// Check if the deleted object was for a regular DNS name. If so, add the DNS name
		// to dnsNameList slice. Additionally, remove the corresponding entry of the object
		// from regularObjInfo map.
		matchesRegular = true
		dnsNameList = append(dnsNameList, regularDNSName)
		delete(res.regularObjInfo, objName)
	} else {
		// If the deleted object was not for a regular DNS name, then check if the deleted
		// object was for a wildcard DNS name. If so, add the DNS names matching the wildcard
		// DNS name to dnsNameList slice. Additionally, remove the corresponding entry of the
		// object from wildcardObjInfo map.
		wildcardDNSNames, wildcardExists := res.wildcardObjInfo[objName]
		if !wildcardExists {
			return
		}
		dnsNameList = append(dnsNameList, wildcardDNSNames.UnsortedList()...)
		delete(res.wildcardObjInfo, objName)
		matchesRegular = false
	}

	// Iterate through the dnsNameList slice.
	for _, dnsName := range dnsNameList {
		// Get the resolved name details corresponding to the DNS name.
		resolvedName, exists := res.dnsNames[dnsName]
		// If the corresponding resolved name details is not found,
		// continue with the next DNS name.
		if !exists {
			continue
		}

		// If the object was for a regular DNS name, then unset the regularObjName
		// field of the resolved name. Otherwise, unset the wildcardObjName field
		// of the resolved name.
		if matchesRegular {
			resolvedName.regularObjExists = false
		} else {
			resolvedName.wildcardObjExists = false
		}

		// If both the regularObjName and wildcardObjName fields are unset, then
		// there are no DNSNameResolver object that references the DNS name.
		// Remove the details of the resolved name corresponding to the DNS name
		// from the dnsNames map. Also, send the DNS name to the deleted channel
		// indicating that the details corresponding to the DNS name have been
		// deleted. Otherwise, add the updated resolved name to the dnsNames map
		// corresponding to the DNS name.
		if !resolvedName.regularObjExists && !resolvedName.wildcardObjExists {
			delete(res.dnsNames, dnsName)
			res.deleted <- dnsName
		} else {
			res.dnsNames[dnsName] = resolvedName
		}
	}
}

// getNextDNSNameDetails returns the DNS name with minimum next lookup time.
// It also returns the next lookup time and the number of IP addresses
// associated with the DNS name. If no such DNS name exists, then false is
// returned, otherwise true is returned.
func (res *Resolver) getNextDNSNameDetails() (string, time.Time, int, bool) {
	res.dnsLock.Lock()
	defer res.dnsLock.Unlock()

	exists := false
	var minNextLookupTime time.Time
	var dns string
	var numIPs int

	for dnsName, resolvedName := range res.dnsNames {
		if !exists || resolvedName.minNextLookupTime.Before(minNextLookupTime) {
			exists = true
			minNextLookupTime = resolvedName.minNextLookupTime
			dns = dnsName
			numIPs = resolvedName.numIPs
		}
	}
	return dns, minNextLookupTime, numIPs, exists
}

// lookupDNSName sends a DNS lookup request to CoreDNS pod(s). The DNS lookup is performed to
// trigger an update, if required, of the DNSNameResolver resources matching the DNS name.
func (res *Resolver) lookupDNSName(dnsName string, numIPs int) error {
	// By default, the DNS lookup request will be sent to maxCoreDNSPodIPs number of CoreDNS
	// pods.
	numCoreDNSPodIPs := maxCoreDNSPodIPs
	// If the DNS name has 0 or 1 associated IP addresses, then the DNS lookup request will
	// be sent to only 1 CoreDNS pod.
	if numIPs <= 1 {
		numCoreDNSPodIPs = 1
	}

	// Get the randomly chosen CoreDNS pod IPs.
	coreDNSPodIPs, err := res.getRandomCoreDNSPodIP(numCoreDNSPodIPs)
	if err != nil {
		return err
	}

	// Send the DNS lookup request to the CoreDNS pods for both A and AAAA type DNS records.
	for _, recordType := range []uint16{dns.TypeA, dns.TypeAAAA} {
		for _, coreDNSPodIP := range coreDNSPodIPs {
			dnsMsg := &dns.Msg{}
			dnsMsg.SetQuestion(dns.Fqdn(dnsName), recordType)
			serverStr := net.JoinHostPort(coreDNSPodIP, defaultPort)
			dnsClient := &dns.Client{
				Timeout: defaultMinTTL,
			}
			if _, _, err := dnsClient.Exchange(dnsMsg, serverStr); err != nil {
				controllerLog.Info(fmt.Sprintf("Failed to lookup DNS name: %s from CoreDNS pod with IP: %s, err: %s", dnsName, coreDNSPodIP, err))
			}
		}
	}
	return nil
}

// getRandomCoreDNSPodIP returns randomly chosen CoreDNS pod IPs. The input maxIPs defines
// the upper limit for the number of CoreDNS pod IPs returned.
func (res *Resolver) getRandomCoreDNSPodIP(maxIPs int) ([]string, error) {
	// List all the CoreDNS pod endpointslices.
	epList := &discoveryv1.EndpointSliceList{}
	if err := res.corednsEndpointSliceCache.List(context.Background(), epList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	// Get all the CoreDNS pod IPs from the list.
	var ips []string
	for _, epSlice := range epList.Items {
		for _, ep := range epSlice.Endpoints {
			if ep.Conditions.Ready != nil && !*ep.Conditions.Ready {
				continue
			}
			ips = append(ips, ep.Addresses...)
		}
	}

	// If no IP is found the return an error.
	if len(ips) == 0 {
		return nil, fmt.Errorf("no ips found for the coredns pods")
	}

	var randomIPs []string
	r1 := rand.New(rand.NewSource(time.Now().UnixNano()))

	if len(ips) <= maxIPs {
		// If the number of CoreDNS pod IPs is less than equal to maxIPs,
		// then return all the CoreDNS pod IPs.
		randomIPs = ips
	} else {
		// If the number of CoreDNS pod IPs is greater than maxIPs, then
		// return randomly chosen maxIPs number of CoreDNS pod IPs
		for i := 0; i < maxIPs; i++ {
			// Randomly select an IP address from the list of IPs and add
			// it to the randomIPs slice. Then replace the element present
			// at the randomly selected index with the last element of the
			// ips slice and reduce the length of the ips slice by 1.
			len := len(ips)
			index := r1.Intn(len)
			randomIPs = append(randomIPs, ips[index])
			ips[index] = ips[len-1]
			ips = ips[:len-1]
		}
	}

	return randomIPs, nil
}
