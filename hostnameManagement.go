package main

import (
	"fmt"
	"net"
	"strings"

	v1 "k8s.io/api/core/v1"
)

func CheckHostnamesConfiguration(ep *v1.EndpointSubset, hosts []string) bool {
	shouldUpdate := false

	if CheckRemovedHost(ep, hosts) {
		shouldUpdate = true
	}

	if ResolveHosts(ep, hosts) {
		shouldUpdate = true
	}

	return shouldUpdate
}

func CheckRemovedHost(ep *v1.EndpointSubset, hosts []string) bool {
	shouldUpdate := false

	updateAddr, newSet := CheckRemovedHostAddrSet(ep.Addresses, hosts)
	if updateAddr {
		ep.Addresses = newSet
		shouldUpdate = true
	}

	updateAddr, newSet = CheckRemovedHostAddrSet(ep.NotReadyAddresses, hosts)
	if updateAddr {
		ep.NotReadyAddresses = newSet
		shouldUpdate = true
	}

	return shouldUpdate
}

func CheckRemovedHostAddrSet(addrSet []v1.EndpointAddress, hosts []string) (bool, []v1.EndpointAddress) {
	idxToRemove := make([]int, 0)
	shouldUpdate := false

	for idx, addr := range addrSet {
		if addr.Hostname != "" && !matchesHost(addr.Hostname, hosts) {
			idxToRemove = append(idxToRemove, idx)
			shouldUpdate = true
		}
	}
	for _, id := range idxToRemove {
		addrSet = remove(addrSet, id)
	}
	return shouldUpdate, addrSet
}

func ResolveHosts(ep *v1.EndpointSubset, hosts []string) bool {
	shouldUpdate := false

	for _, hostname := range hosts {
		ip, err := net.LookupIP(hostname)
		if err != nil {
			fmt.Println(err)
			continue
		}
		ipString := ip[0].String()
		CheckDNSChange(ep, ipString, hostname)
		shouldAddHost := true
		for _, addr := range ep.Addresses {
			if addr.IP == ipString {
				shouldAddHost = false
				break
			}
		}
		for _, addr := range ep.NotReadyAddresses {
			if addr.IP == ipString {
				shouldAddHost = false
				break
			}
		}
		if shouldAddHost {
			AddNewAddress(ep, ipString, hostname)
			shouldUpdate = true
		}
	}
	return shouldUpdate
}

func CheckDNSChange(ep *v1.EndpointSubset, ip string, hostname string) {
	for idx, addr := range ep.Addresses {
		if addr.Hostname == hostname && addr.IP != ip {
			ep.Addresses = remove(ep.Addresses, idx)
			break
		}
	}
	for idx, addr := range ep.NotReadyAddresses {
		if addr.Hostname == hostname && addr.IP != ip {
			ep.NotReadyAddresses = remove(ep.NotReadyAddresses, idx)
			break
		}
	}
}

func matchesHost(hostToCheck string, hosts []string) bool {
	for _, host := range hosts {
		if hostToCheck == strings.Split(host, ".")[0] {
			return true
		}
	}
	return false
}

func remove(s []v1.EndpointAddress, i int) []v1.EndpointAddress {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}
