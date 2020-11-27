// GNU General Public License version 3

package probe

import (
	"crypto/tls"
	"fmt"
	"github.com/criteo-forks/espoke/common"
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
	consulApi     string

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

	probingURL := fmt.Sprintf("http://%v:%v/api/status", node.Ip, node.Port)
	log.Debug("Start probing ", node.Name)

	resp, err := client.Get(probingURL)
	if err != nil {
		log.Debug("Probing failed for ", node.Name, ": ", probingURL, " ", err.Error())
		log.Error(err)
		common.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		common.ErrorsCount.Inc()
		return err
	}

	log.Debug("Probe result for ", node.Name, ": ", resp.Status)
	if resp.StatusCode != 200 {
		log.Error("Probing failed for ", node.Name, ": ", probingURL, " ", resp.Status)
		common.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		common.ErrorsCount.Inc()
		return fmt.Errorf("kibana Probing failed")
	}

	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		common.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		return fmt.Errorf("kibana Probing failed: %s", readErr)
	}

	var p fastjson.Parser
	json, jsonErr := p.Parse(string(body))
	if jsonErr != nil {
		common.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		return fmt.Errorf("kibana Probing failed: %s", jsonErr)
	}
	nodeState := string(json.GetStringBytes("status", "overall", "state"))
	if nodeState != "green" {
		common.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		return fmt.Errorf("kibana Probing failed: node not in a green/healthy state")
	}

	common.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(1)

	return nil
}

func NewKibanaProbe(clusterName, consulApi string, clusterConfig common.Cluster, consulPeriod, probePeriod, cleaningPeriod time.Duration, controlChan chan bool) (KibanaProbe, error) {
	var allEverKnownKibanaNodes []string
	kibanaNodesList, err := common.DiscoverNodesForService(consulApi, clusterConfig.Name)
	if err != nil {
		common.ErrorsCount.Inc()
		log.Fatal("Impossible to discover kibana nodes during bootstrap, exiting")
	}
	allEverKnownKibanaNodes = common.UpdateEverKnownNodes(allEverKnownKibanaNodes, kibanaNodesList)

	return KibanaProbe{
		clusterName:   clusterName,
		consulApi:     consulApi,
		clusterConfig: clusterConfig,

		timeout: probePeriod - 2*time.Second,

		updateDiscoveryTicker: time.NewTicker(consulPeriod),
		executeProbingTicker:  time.NewTicker(probePeriod),
		cleanMetricsTicker:    time.NewTicker(cleaningPeriod),

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
			common.CleanMetrics(kibana.kibanaNodesList, kibana.allEverKnownKibanaNodes)
			return nil

		case <-kibana.cleanMetricsTicker.C:
			log.Info("Cleaning Prometheus metrics for unreferenced nodes")
			common.CleanMetrics(kibana.kibanaNodesList, kibana.allEverKnownKibanaNodes)

		case <-kibana.updateDiscoveryTicker.C:
			// Kibana
			log.Debug("Starting updating Kibana nodes list")
			kibanaUpdatedList, err := common.DiscoverNodesForService(kibana.consulApi, kibana.clusterConfig.Name)
			if err != nil {
				log.Error("Unable to update Kibana nodes, using last known state")
				common.ErrorsCount.Inc()
				continue
			}

			log.Info("Updating kibana nodes list")
			kibana.allEverKnownKibanaNodes = common.UpdateEverKnownNodes(kibana.allEverKnownKibanaNodes, kibanaUpdatedList)
			kibana.kibanaNodesList = kibanaUpdatedList

		case <-kibana.executeProbingTicker.C:
			log.Debug("Starting probing Kibana nodes")

			sem := new(sync.WaitGroup)
			for _, node := range kibana.kibanaNodesList {
				sem.Add(1)
				go func(kibanaNode common.Node) {
					defer sem.Done()
					probeKibanaNode(&kibanaNode, kibana.timeout)
				}(node)

			}
			sem.Wait()
		}
	}
}
