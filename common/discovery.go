// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package common

import (
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"sort"
	"strings"
)

//TODO detect probe stuck
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

func NewClient(consulTarget string) (*api.Client, error) {
	consulConfig := api.DefaultConfig()
	consulConfig.Address = consulTarget
	consul, err := api.NewClient(consulConfig)
	if err != nil {
		ErrorsCount.Inc()
		return nil, errors.Wrapf(err, "Failed to create consul client with target %s", consulTarget)
	}
	return consul, nil
}

func DiscoverNodesForService(consul *api.Client, serviceName string) ([]Node, error) {
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
		var addr = svc.Address
		if svc.ServiceAddress != "" {
			addr = svc.ServiceAddress
		}

		log.Debug("Service discovered: ", svc.Node, " (", addr, ":", svc.ServicePort, ")")
		nodeList = append(nodeList, Node{
			Name:    svc.Node,
			Ip:      addr,
			Port:    svc.ServicePort,
			Scheme:  schemeFromTags(svc.ServiceTags),
			Cluster: valueFromTags("cluster_name", svc.ServiceTags),
		})
	}

	nodesCount := len(nodeList)
	log.Debug(nodesCount, " nodes found")

	return nodeList, nil
}

func GetServices(consul *api.Client, consulTag string) (map[string]Cluster, error) {
	consulServices, _, err := consul.Catalog().Services(nil)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to get services from consul")
	}

	var services = make(map[string]Cluster)
	var service Cluster
	for serviceName := range consulServices {
		for i := range consulServices[serviceName] {
			if consulServices[serviceName][i] == consulTag {
				// Check cluster not already added
				cluster := valueFromTags("cluster_name", consulServices[serviceName])
				// TODO ensure we use https when available?
				_, ok := services[cluster]
				if !ok {
					service = Cluster{
						Name:    serviceName,
						Scheme:  schemeFromTags(consulServices[serviceName]),
						Version: valueFromTags("version", consulServices[serviceName]),
					}
					services[cluster] = service
				}
				break
			}
		}
	}
	return services, nil
}

func GetEndpointFromConsul(consul *api.Client, name, endpointSuffix string) (string, error) {
	endpoint := ""

	health := consul.Health()
	serviceEntries, _, _ := health.Service(name, "", false, nil)

	port, err := getServicePort(serviceEntries)
	if err != nil {
		return endpoint, err
	}
	dc, err := getDatacenter(serviceEntries)
	endpointSuffixWithDC := strings.ReplaceAll(endpointSuffix, "{dc}", dc)
	endpoint = fmt.Sprintf("%s%s:%d", name, endpointSuffixWithDC, port)

	return endpoint, nil
}

// getServicePort return the first port found in the service or 80
func getServicePort(serviceEntries []*api.ServiceEntry) (int, error) {
	if len(serviceEntries) == 0 {
		return 80, errors.New("Consul service is empty")
	}
	return serviceEntries[0].Service.Port, nil
}

func getDatacenter(serviceEntries []*api.ServiceEntry) (string, error) {
	if len(serviceEntries) == 0 {
		return "", errors.New("Consul service is empty")
	}
	return serviceEntries[0].Node.Datacenter, nil
}

func valueFromTags(prefix string, serviceTags []string) string {
	for _, tag := range serviceTags {
		splitted := strings.SplitN(tag, "-", 2)
		if splitted[0] == prefix {
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

func contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}
