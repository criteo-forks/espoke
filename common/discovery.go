// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package common

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	log "github.com/sirupsen/logrus"
)

func contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

func UpdateEverKnownNodes(allEverKnownNodes []string, nodes []Node) []string {
	for _, node := range nodes {
		// TODO: Replace by a real struct instead of a string concatenation...
		// also allEverKnownNodes leak memory as it never delete old/cleaned items
		serializedNode := fmt.Sprintf("%v|%v", node.Name, node.Cluster)
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

func DiscoverNodesForService(consulTarget string, serviceName string) ([]Node, error) {
	start := time.Now()

	consulConfig := api.DefaultConfig()
	consulConfig.Address = consulTarget
	consul, err := api.NewClient(consulConfig)
	if err != nil {
		log.Debug("Consul Connection failed: ", err.Error())
		ErrorsCount.Inc()
		return nil, err
	}

	catalogServices, _, err := consul.Catalog().Service(
		serviceName, "",
		&api.QueryOptions{AllowStale: true, RequireConsistent: false, UseCache: true},
	)
	if err != nil {
		log.Error("Consul Discovery failed: ", err.Error())
		ErrorsCount.Inc()
		return nil, err
	}

	var nodeList []Node
	for _, svc := range catalogServices {
		var addr string = svc.Address
		if svc.ServiceAddress != "" {
			addr = svc.ServiceAddress
		}

		log.Debug("Service discovered: ", svc.Node, " (", addr, ":", svc.ServicePort, ")")
		nodeList = append(nodeList, Node{
			Name:    svc.Node,
			Ip:      addr,
			Port:    svc.ServicePort,
			Scheme:  schemeFromTags(svc.ServiceTags),
			Cluster: clusterNameFromTags(svc.ServiceTags),
		})
	}

	nodesCount := len(nodeList)
	NodeCount.Set(float64(nodesCount))
	log.Debug(nodesCount, " nodes found")

	ConsulDiscoveryDurationSummary.Observe(float64(time.Since(start).Nanoseconds()))
	return nodeList, nil
}

func GetServices(consulTarget string, consulTag string) (map[string]Cluster, error) {
	consulConfig := api.DefaultConfig()
	consulConfig.Address = consulTarget
	consul, err := api.NewClient(consulConfig)
	if err != nil {
		log.Debug("Consul Connection failed: ", err.Error())
		ErrorsCount.Inc()
		return nil, err
	}
	consulServices, _, _ := consul.Catalog().Services(nil)

	var services = make(map[string]Cluster)
	var service Cluster
	for serviceName := range consulServices {
		for i := range consulServices[serviceName] {
			if consulServices[serviceName][i] == consulTag {
				// Check cluster not already added
				cluster := clusterNameFromTags(consulServices[serviceName])
				// TODO ensure we use https when available?
				_, ok := services[cluster]
				if !ok {
					service = Cluster{
						Name:   serviceName,
						Scheme: schemeFromTags(consulServices[serviceName]),
					}
					services[cluster] = service
				}
				break
			}
		}
	}
	return services, nil
}
