// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServicesMapper maps pod names to the names of the services targeting the pod
// keyed by the namespace a pod belongs to. This data structure allows for O(1)
// lookups of services given a namespace and pod name.
//
// The data is stored in the following schema:
// {
// 	"namespace": {
// 		"pod": [ "svc1", "svc2", "svc3" ]
// 	}
// }
type ServicesMapper map[string]map[string][]string

// Get returns the list of services for a given namespace and pod name.
func (m ServicesMapper) Get(ns, podName string) ([]string, bool) {
	pods, ok := m[ns]
	if !ok {
		return nil, false
	}
	svcs, ok := pods[podName]
	if !ok {
		return nil, false
	}
	return svcs, true
}

// Set updates the list of services for a given namespace and pod name.
func (m ServicesMapper) Set(ns, podName string, svcs []string) {
	if _, ok := m[ns]; !ok {
		m[ns] = make(map[string][]string)
	}
	m[ns][podName] = svcs
}

// mapOnIp matches pods to services via IP. It supports Kubernetes 1.4+
func (m ServicesMapper) mapOnIp(nodeName string, pods v1.PodList, endpointList v1.EndpointsList) error {
	ipToEndpoints := make(map[string][]string)    // maps the IP address from an endpoint (pod) to associated services ex: "10.10.1.1" : ["service1","service2"]
	podToIp := make(map[string]map[string]string) // maps pod names to its IP address keyed by the namespace a pod belongs to

	if pods.Items == nil {
		return fmt.Errorf("empty podlist received for nodeName %q", nodeName)
	}
	if nodeName == "" {
		log.Debugf("Service mapper was given an empty node name. Mapping might be incorrect.")
	}

	for _, pod := range pods.Items {
		if pod.Status.PodIP == "" {
			log.Debugf("PodIP is empty, ignoring pod %s in namespace %s", pod.Name, pod.Namespace)
			continue
		}
		if _, ok := podToIp[pod.Namespace]; !ok {
			podToIp[pod.Namespace] = make(map[string]string)
		}
		podToIp[pod.Namespace][pod.Name] = pod.Status.PodIP
	}
	for _, svc := range endpointList.Items {
		for _, endpointsSubsets := range svc.Subsets {
			if endpointsSubsets.Addresses == nil {
				log.Tracef("A subset of endpoints from %s could not be evaluated", svc.Name)
				continue
			}
			for _, edpt := range endpointsSubsets.Addresses {
				if edpt.NodeName != nil && *edpt.NodeName == nodeName {
					ipToEndpoints[edpt.IP] = append(ipToEndpoints[edpt.IP], svc.Name)
				}
			}
		}
	}
	for ns, pods := range podToIp {
		for name, ip := range pods {
			if svcs, found := ipToEndpoints[ip]; found {
				m.Set(ns, name, svcs)
			}
		}
	}
	return nil
}

// mapOnRef matches pods to services via endpoint TargetRef objects. It supports Kubernetes 1.3+
func (m ServicesMapper) mapOnRef(nodeName string, endpointList v1.EndpointsList) error {
	uidToPod := make(map[types.UID]v1.ObjectReference)
	uidToServices := make(map[types.UID][]string)

	for _, svc := range endpointList.Items {
		for _, endpointsSubsets := range svc.Subsets {
			for _, edpt := range endpointsSubsets.Addresses {
				if edpt.TargetRef == nil {
					log.Debug("Empty TargetRef on endpoint %s of service %s, skipping", edpt.IP, svc.Name)
					continue
				}
				ref := *edpt.TargetRef
				if ref.Kind != "Pod" {
					continue
				}
				if ref.Name == "" || ref.Namespace == "" {
					log.Debug("Incomplete reference for object %s on service %s, skipping", ref.UID, svc.Name)
					continue
				}
				uidToPod[ref.UID] = ref
				uidToServices[ref.UID] = append(uidToServices[ref.UID], svc.Name)
			}
		}
	}
	for uid, svcs := range uidToServices {
		pod, ok := uidToPod[uid]
		if !ok {
			continue
		}
		m.Set(pod.Namespace, pod.Name, svcs)
	}
	return nil
}

// mapServices maps each pod (endpoint) to the metadata associated with it.
// It is on a per node basis to avoid mixing up the services pods are actually connected to if all pods of different nodes share a similar subnet, therefore sharing a similar IP.
func (metaBundle *MetadataMapperBundle) mapServices(nodeName string, pods v1.PodList, endpointList v1.EndpointsList) error {
	metaBundle.m.Lock()
	defer metaBundle.m.Unlock()

	var err error
	if metaBundle.mapOnIP {
		err = metaBundle.Services.mapOnIp(nodeName, pods, endpointList)
	} else { // Default behaviour
		err = metaBundle.Services.mapOnRef(nodeName, endpointList)
	}
	if err != nil {
		return err
	}
	log.Tracef("The services matched %q", fmt.Sprintf("%s", metaBundle.Services))
	return nil
}

// ServicesForPod returns the services mapped to a given pod and namespace.
// If nothing is found, the boolean is false. This call is thread-safe.
func (metaBundle *MetadataMapperBundle) ServicesForPod(ns, podName string) ([]string, bool) {
	metaBundle.m.RLock()
	defer metaBundle.m.RUnlock()

	return metaBundle.Services.Get(ns, podName)
}
