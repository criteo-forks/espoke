// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	log "github.com/sirupsen/logrus"
)

type esnode struct {
	name    string
	ip      string
	port    int
	cluster string
	scheme  string
}

func contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

func updateEverKnownNodes(allEverKnownNodes []string, nodes []esnode) []string {
	for _, node := range nodes {
		// TODO: Replace by a real struct instead of a string concatenation...
		// also allEverKnownNodes leak memory as it never delete old/cleaned items
		serializedNode := fmt.Sprintf("%v|%v", node.name, node.cluster)
		if contains(allEverKnownNodes, serializedNode) == false {
			allEverKnownNodes = append(allEverKnownNodes, serializedNode)
		}
	}
	sort.Strings(allEverKnownNodes)
	return allEverKnownNodes
}

func clusterNameFromTags(serviceTags []string) string {
	for _, tag := range serviceTags {
		splitted := strings.SplitN(tag, "-", 2)
		if splitted[0] == "cluster_name" {
			return splitted[1]
		}
	}
	return ""
}

func schemeFromTags(serviceTags []string) string {
	scheme := "http"
	for _, tag := range serviceTags {
		if tag == "https" {
			scheme = tag
			break
		}
	}

	return scheme
}

func discoverNodesForService(consulTarget string, serviceName string) ([]esnode, error) {
	start := time.Now()

	consulConfig := api.DefaultConfig()
	consulConfig.Address = consulTarget
	consul, err := api.NewClient(consulConfig)
	if err != nil {
		log.Debug("Consul Connection failed: ", err.Error())
		errorsCount.Inc()
		return nil, err
	}

	catalogServices, _, err := consul.Catalog().Service(
		serviceName, "",
		&api.QueryOptions{AllowStale: true, RequireConsistent: false, UseCache: true},
	)
	if err != nil {
		log.Error("Consul Discovery failed: ", err.Error())
		errorsCount.Inc()
		return nil, err
	}

	var nodeList []esnode
	for _, svc := range catalogServices {
		var addr string = svc.Address
		if svc.ServiceAddress != "" {
			addr = svc.ServiceAddress
		}

		log.Debug("Service discovered: ", svc.Node, " (", addr, ":", svc.ServicePort, ")")
		nodeList = append(nodeList, esnode{
			name:    svc.Node,
			ip:      addr,
			port:    svc.ServicePort,
			scheme:  schemeFromTags(svc.ServiceTags),
			cluster: clusterNameFromTags(svc.ServiceTags),
		})
	}

	nodesCount := len(nodeList)
	nodeCount.Set(float64(nodesCount))
	log.Debug(nodesCount, " nodes found")

	consulDiscoveryDurationSummary.Observe(float64(time.Since(start).Nanoseconds()))
	return nodeList, nil
}
