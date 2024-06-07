package dnsnameresolver

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
	ocpnetworkv1alpha1 "github.com/openshift/api/network/v1alpha1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultMaxTTL    = 30 * time.Minute
	defaultMinTTL    = 5 * time.Second
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
	port                      string
	dnsNames                  map[string]*dnsResolvedName
	regularObjInfo            map[string]string
	wildcardObjInfo           map[string]sets.Set[string]
	dnsLock                   sync.Mutex

	added   chan dnsDetails
	deleted chan dnsDetails
}

type dnsDetails struct {
	dnsName           string
	resolvedAddresses []ocpnetworkv1alpha1.DNSNameResolverResolvedAddress
	matchesRegular    bool
	objName           string
}

// NewResolver initializes and returns a new resolver instance.
func NewResolver(cache cache.Cache, port string) *Resolver {
	return &Resolver{
		corednsEndpointSliceCache: cache,
		port:                      port,
		dnsNames:                  make(map[string]*dnsResolvedName),
		regularObjInfo:            make(map[string]string),
		wildcardObjInfo:           make(map[string]sets.Set[string]),
		added:                     make(chan dnsDetails),
		deleted:                   make(chan dnsDetails),
	}
}

// Start starts a goroutine for sending DNS lookup requests of the DNS names on expiration of the corresponding TTLs.
func (resolver *Resolver) Start() {
	var (
		nextDNSName    string
		nextLookupTime time.Time
		exists         bool
		numIPs         int
	)
	// Initially the check for nextDNSName to be looked up will happen after default maximum TTL. If a DNS name
	// gets added before that then the nextDNSName and timeTillNextLookup will be updated.
	timeTillNextLookup := defaultMaxTTL
	go func() {
		timer := time.NewTicker(timeTillNextLookup)
		defer timer.Stop()
		for {
			select {
			case addedDNSDetails := <-resolver.added:
				// Whenever DNS name(s) are added or updated, get the DNS name whose TTL will expire first.
				// Based on the nextLookupTime for the nextDNSName, the timeTillNextLookup will be updated
				// appropriately.
				nextDNSName, nextLookupTime, numIPs, exists = resolver.add(addedDNSDetails)
			case <-timer.C:
				// After every tick, DNS lookup needs to be performed for nextDNSName, if it is not empty.
				// The nextDNSName and timeTillNextLookup also needs to be updated.
				if len(nextDNSName) > 0 {
					if err := resolver.lookupDNSNameFromCoreDNS(nextDNSName, numIPs); err != nil {
						controllerLog.Info("Warning: Encountered error while looking up DNS name", "DNS name", nextDNSName, "Error", err)
					}
				}
				// Get the DNS name whose TTL will expire first. Based on the nextLookupTime for the nextDNSName,
				// the timeTillNextLookup will be updated appropriately.
				resolver.dnsLock.Lock()
				nextDNSName, nextLookupTime, numIPs, exists = resolver.getNextDNSNameDetails()
				resolver.dnsLock.Unlock()
			case deletedDNSDetails := <-resolver.deleted:
				// Whenever DNS name(s) are deleted, get the DNS name whose TTL will expire first.
				// Based on the nextLookupTime for the nextDNSName, the timeTillNextLookup will be
				// updated appropriately
				nextDNSName, nextLookupTime, numIPs, exists = resolver.delete(deletedDNSDetails)
			}
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
				// A DNS lookup request has been sent upon TTL expiration of the DNS name. Reset the timer to wait until twice of default
				// minimum TTL to perform the next lookup.
				timeTillNextLookup = 2 * defaultMinTTL
			}
			timer.Reset(timeTillNextLookup)
		}
	}()
}

// AddResolvedName is called whenever a DNSNameResolver object is added or updated.
func (resolver *Resolver) AddResolvedName(dnsDetails dnsDetails) {
	// Send a signal to the added channel indicating that details corresponding to a DNS
	// name have been added or updated.
	resolver.added <- dnsDetails
}

// add is called whenever a DNSNameResolver object is added or updated to get the details
// of the next DNS name to be looked up.
func (resolver *Resolver) add(dnsDetails dnsDetails) (string, time.Time, int, bool) {
	resolver.dnsLock.Lock()
	defer resolver.dnsLock.Unlock()

	// Get the details of the resolved name corresponding to the DNS name, if it exists.
	resolvedName, exists := resolver.dnsNames[dnsDetails.dnsName]

	// If the resolved name corresponding to the DNS name exists, ensure that the DNSNameResolver
	// object matches the existing information. Otherwise, don't proceed.
	if exists {
		if resolvedName.regularObjExists && dnsDetails.matchesRegular {
			// If the DNSNameResolver object for the regular DNS name object exists and the
			// current Add call is also for an object corresponding to a regular DNS name,
			// then check if the existing DNS name matches the current one.
			matchingDNSName, found := resolver.regularObjInfo[dnsDetails.objName]
			if !found || dnsDetails.dnsName != matchingDNSName {
				return resolver.getNextDNSNameDetails()
			}
		} else if resolvedName.wildcardObjExists && !dnsDetails.matchesRegular {
			// If the DNSNameResolver object for the wildcard DNS name object exists and the
			// current Add call is also for an object corresponding to a wildcard DNS name,
			// then check if the current DNS name exists in the set of DNS names matching the
			// wildcard DNS name.
			dnsNamesMatchingWildcard, found := resolver.wildcardObjInfo[dnsDetails.objName]
			if !found || !dnsNamesMatchingWildcard.Has(dnsDetails.dnsName) {
				return resolver.getNextDNSNameDetails()
			}
		}
	} else {
		resolvedName = &dnsResolvedName{}

		// Add the updated resolved name to the dnsNames map corresponding to the DNS name.
		resolver.dnsNames[dnsDetails.dnsName] = resolvedName
	}

	if len(dnsDetails.resolvedAddresses) == 0 {
		// If no IP address is currently associated with the DNS name and the corresponding
		// resolved name details also do not exist, then set the next lookup time for the
		// DNS name as the default maximum TTL. Spawn a goroutine to send a DNS lookup
		// request for the DNS name.
		if !exists {
			resolvedName.minNextLookupTime = time.Now().Add(defaultMaxTTL)
			go resolver.lookupDNSNameFromCoreDNS(dnsDetails.dnsName, 0)
		}
	} else {
		// If some IP addresses are associated with the DNS name, then determine the minimum
		// next lookup time among the associated IP addresses.
		first := false
		for _, resolvedAddress := range dnsDetails.resolvedAddresses {
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
	resolvedName.numIPs = len(dnsDetails.resolvedAddresses)

	// Check if the DNSNameResolver object is corresponding to a regular DNS name. If so, set
	// regularObjExists to true and add the DNS name corresponding to the DNSNameResolver
	// object name in the regularObjInfo map. Otherwise, set wildcardObjExists to true and
	// add the DNS name to the DNS name set matching the wildcard DNS name and add the set
	// corresponding to the DNSNameResolver object name in the wildcardObjInfo map.
	if dnsDetails.matchesRegular {
		resolvedName.regularObjExists = true
		resolver.regularObjInfo[dnsDetails.objName] = dnsDetails.dnsName
	} else {
		resolvedName.wildcardObjExists = true
		dnsNamesMatchingWildcard, exists := resolver.wildcardObjInfo[dnsDetails.objName]
		if !exists {
			dnsNamesMatchingWildcard = sets.New[string]()
		}
		dnsNamesMatchingWildcard.Insert(dnsDetails.dnsName)
		resolver.wildcardObjInfo[dnsDetails.objName] = dnsNamesMatchingWildcard
	}

	return resolver.getNextDNSNameDetails()
}

// DeleteResolvedName is called whenever a DNSNameResolver object is deleted.
func (resolver *Resolver) DeleteResolvedName(dnsDetails dnsDetails) {
	resolver.deleted <- dnsDetails
}

// delete is called whenever a DNSNameResolver object is deleted to get the details
// of the next DNS name to be looked up.
func (resolver *Resolver) delete(dnsDetails dnsDetails) (string, time.Time, int, bool) {
	resolver.dnsLock.Lock()
	defer resolver.dnsLock.Unlock()

	objName := dnsDetails.objName
	var matchesRegular bool
	dnsNameList := []string{}

	regularDNSName, regularExists := resolver.regularObjInfo[objName]
	if regularExists {
		// Check if the deleted object was for a regular DNS name. If so, add the DNS name
		// to dnsNameList slice. Additionally, remove the corresponding entry of the object
		// from regularObjInfo map.
		matchesRegular = true
		dnsNameList = append(dnsNameList, regularDNSName)
		delete(resolver.regularObjInfo, objName)
	} else {
		// If the deleted object was not for a regular DNS name, then check if the deleted
		// object was for a wildcard DNS name. If so, add the DNS names matching the wildcard
		// DNS name to dnsNameList slice. Additionally, remove the corresponding entry of the
		// object from wildcardObjInfo map.
		wildcardDNSNames, wildcardExists := resolver.wildcardObjInfo[objName]
		if !wildcardExists {
			return resolver.getNextDNSNameDetails()
		}
		dnsNameList = append(dnsNameList, wildcardDNSNames.UnsortedList()...)
		delete(resolver.wildcardObjInfo, objName)
		matchesRegular = false
	}

	// Iterate through the dnsNameList slice.
	for _, dnsName := range dnsNameList {

		// Get the resolved name details corresponding to the DNS name.
		resolvedName, exists := resolver.dnsNames[dnsName]
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
			delete(resolver.dnsNames, dnsName)
		}
	}

	return resolver.getNextDNSNameDetails()
}

// getNextDNSNameDetails returns the DNS name with minimum next lookup time.
// It also returns the next lookup time and the number of IP addresses
// associated with the DNS name. If no such DNS name exists, then false is
// returned, otherwise true is returned.
func (resolver *Resolver) getNextDNSNameDetails() (string, time.Time, int, bool) {

	exists := false
	var minNextLookupTime time.Time
	var dns string
	var numIPs int

	for dnsName, resolvedName := range resolver.dnsNames {
		if !exists || resolvedName.minNextLookupTime.Before(minNextLookupTime) {
			exists = true
			minNextLookupTime = resolvedName.minNextLookupTime
			dns = dnsName
			numIPs = resolvedName.numIPs
		}
		// If there are no IP addresses associated with the DNS name and the next lookup
		// time of the DNS name is already past the current time, then reset the next
		// lookup time to the default maximum TTL.
		if resolvedName.numIPs == 0 && !time.Now().Before(resolvedName.minNextLookupTime) {
			resolvedName.minNextLookupTime = time.Now().Add(defaultMaxTTL)
		}
	}
	return dns, minNextLookupTime, numIPs, exists
}

// lookupDNSNameFromCoreDNS sends DNS lookup request(s) to CoreDNS pod(s). The DNS lookup is performed to
// trigger an update, if required, of the DNSNameResolver resources matching the DNS name.
func (resolver *Resolver) lookupDNSNameFromCoreDNS(dnsName string, numIPs int) error {
	// By default, the DNS lookup request will be sent to maxCoreDNSPodIPs number of CoreDNS
	// pods.
	numCoreDNSPodIPs := maxCoreDNSPodIPs
	// If the DNS name has 0 or 1 associated IP addresses, then the DNS lookup request will
	// be sent to only 1 CoreDNS pod.
	if numIPs <= 1 {
		numCoreDNSPodIPs = 1
	}

	// Get the randomly chosen CoreDNS pod IPs.
	coreDNSPodIPs, err := resolver.getRandomCoreDNSPodIPs(numCoreDNSPodIPs)
	if err != nil {
		return err
	}

	dnsClient := &dns.Client{
		Timeout: defaultMinTTL,
	}

	// Send the DNS lookup request to the CoreDNS pods for both A and AAAA type DNS records.
	for _, recordType := range []uint16{dns.TypeA, dns.TypeAAAA} {
		for _, coreDNSPodIP := range coreDNSPodIPs {
			if _, _, err := sendDNSLookupRequest(dnsClient, coreDNSPodIP, resolver.port, dnsName, recordType); err != nil {
				controllerLog.Info(fmt.Sprintf("Failed to lookup DNS name: %s from CoreDNS pod with IP: %s, err: %s", dnsName, coreDNSPodIP, err))
			}
		}
	}
	return nil
}

// sendDNSLookupRequest sends the DNS lookup request for the dnsName to the DNS server on serverIP:serverPort
// using the dnsClient for DNS record type recordType.
func sendDNSLookupRequest(dnsClient *dns.Client, serverIP, serverPort, dnsName string, recordType uint16) (*dns.Msg, time.Duration, error) {
	dnsMsg := &dns.Msg{}
	dnsMsg.SetQuestion(dns.Fqdn(dnsName), recordType)
	serverStr := net.JoinHostPort(serverIP, serverPort)
	return dnsClient.Exchange(dnsMsg, serverStr)
}

// getRandomCoreDNSPodIPs returns randomly chosen CoreDNS pod IPs. The input maxIPs defines
// the upper limit for the number of CoreDNS pod IPs returned.
func (resolver *Resolver) getRandomCoreDNSPodIPs(maxIPs int) ([]string, error) {
	// List all the CoreDNS pod endpointslices.
	epList := &discoveryv1.EndpointSliceList{}
	if err := resolver.corednsEndpointSliceCache.List(context.Background(), epList, &client.ListOptions{}); err != nil {
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
			r1 := rand.New(rand.NewSource(time.Now().UnixNano()))
			len := len(ips)
			index := r1.Intn(len)
			randomIPs = append(randomIPs, ips[index])
			ips[index] = ips[len-1]
			ips = ips[:len-1]
		}
	}

	return randomIPs, nil
}
