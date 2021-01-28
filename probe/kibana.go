// GNU General Public License version 3

package probe

import (
	"crypto/tls"
	"fmt"
	"github.com/criteo-forks/espoke/common"
	"github.com/hashicorp/consul/api"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/valyala/fastjson"
)

type KibanaProbe struct {
	clusterName   string
	clusterConfig common.Cluster

	consulClient *api.Client

	timeout time.Duration

	updateDiscoveryTicker *time.Ticker
	cleanMetricsTicker    *time.Ticker
	executeProbingTicker  *time.Ticker

	kibanaNodesList         []common.Node
	allEverKnownKibanaNodes []string

	controlChan chan bool
}

func probeKibanaNode(node *common.Node, timeout time.Duration) error {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{
		Timeout: timeout,
	}

	probingURL := fmt.Sprintf("%v://%v:%v/api/status", node.Scheme, node.Ip, node.Port)
	log.Debug("Start probing ", node.Name)

	resp, err := client.Get(probingURL)
	if err != nil {
		log.Debug("Probing failed for ", node.Name, ": ", probingURL, " ", err.Error())
		return err
	}

	log.Debug("Probe result for ", node.Name, ": ", resp.Status)
	if resp.StatusCode != 200 {
		log.Error("Probing failed for ", node.Name, ": ", probingURL, " ", resp.Status)
		return fmt.Errorf("kibana Probing failed")
	}

	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("kibana Probing failed: %s", readErr)
	}

	var p fastjson.Parser
	json, jsonErr := p.Parse(string(body))
	if jsonErr != nil {
		return fmt.Errorf("kibana Probing failed: %s", jsonErr)
	}
	nodeState := string(json.GetStringBytes("status", "overall", "state"))
	if nodeState != "green" {
		return fmt.Errorf("kibana Probing failed: node not in a green/healthy state")
	}

	common.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(1)

	return nil
}

func NewKibanaProbe(clusterName string, clusterConfig common.Cluster, config *common.Config, consulClient *api.Client, controlChan chan bool) (KibanaProbe, error) {
	var allEverKnownKibanaNodes []string
	kibanaNodesList, err := common.DiscoverNodesForService(consulClient, clusterConfig.Name)
	if err != nil {
		common.ErrorsCount.Inc()
		log.Fatal("Impossible to discover kibana nodes during bootstrap, exiting")
	}
	allEverKnownKibanaNodes = common.UpdateEverKnownNodes(allEverKnownKibanaNodes, kibanaNodesList)

	return KibanaProbe{
		clusterName:   clusterName,
		clusterConfig: clusterConfig,

		consulClient: consulClient,

		timeout: config.ProbePeriod - 2*time.Second,

		updateDiscoveryTicker: time.NewTicker(config.ConsulPeriod),
		executeProbingTicker:  time.NewTicker(config.ProbePeriod),
		cleanMetricsTicker:    time.NewTicker(config.CleaningPeriod),

		kibanaNodesList:         kibanaNodesList,
		allEverKnownKibanaNodes: allEverKnownKibanaNodes,

		controlChan: controlChan,
	}, nil
}

func (kibana *KibanaProbe) StartKibanaProbing() error {
	for {
		select {
		case <-kibana.controlChan:
			log.Println("Terminating kibana probe on ", kibana.clusterName)
			kibana.cleanMetricsTicker.Stop()
			kibana.updateDiscoveryTicker.Stop()
			kibana.executeProbingTicker.Stop()
			common.CleanNodeMetrics(kibana.kibanaNodesList, kibana.allEverKnownKibanaNodes)
			return nil

		case <-kibana.cleanMetricsTicker.C:
			log.Infof("Cleaning Prometheus metrics for unreferenced nodes on cluster %s", kibana.clusterName)
			common.CleanNodeMetrics(kibana.kibanaNodesList, kibana.allEverKnownKibanaNodes)

		case <-kibana.updateDiscoveryTicker.C:
			// Kibana
			log.Debugf("Starting updating Kibana nodes list on cluster %s", kibana.clusterName)
			kibanaUpdatedList, err := common.DiscoverNodesForService(kibana.consulClient, kibana.clusterConfig.Name)
			if err != nil {
				log.Error("Unable to update Kibana nodes, using last known state")
				common.ErrorsCount.Inc()
				continue
			}

			log.Infof("Updating kibana nodes list on cluster %s", kibana.clusterName)
			kibana.allEverKnownKibanaNodes = common.UpdateEverKnownNodes(kibana.allEverKnownKibanaNodes, kibanaUpdatedList)
			kibana.kibanaNodesList = kibanaUpdatedList

		case <-kibana.executeProbingTicker.C:
			log.Debugf("Starting probing Kibana nodes on cluster %s", kibana.clusterName)

			sem := new(sync.WaitGroup)
			for _, node := range kibana.kibanaNodesList {
				sem.Add(1)
				go func(kibanaNode common.Node) {
					defer sem.Done()
					if err := probeKibanaNode(&kibanaNode, kibana.timeout); err != nil {
						log.Errorf("Failed on %s: %s", kibana.clusterName, err.Error())
						common.KibanaNodeAvailabilityGauge.WithLabelValues(kibanaNode.Cluster, kibanaNode.Name).Set(0)
						common.ErrorsCount.Inc()
					}
				}(node)

			}
			sem.Wait()
		}
	}
}
