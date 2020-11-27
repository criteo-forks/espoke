// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package probe

import (
	"crypto/tls"
	"fmt"
	"github.com/criteo-forks/espoke/common"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type EsProbe struct {
	clusterName   string
	clusterConfig common.Cluster
	consulApi     string

	timeout time.Duration

	updateDiscoveryTicker *time.Ticker
	cleanMetricsTicker    *time.Ticker
	executeProbingTicker  *time.Ticker

	esNodesList         []common.Node
	allEverKnownEsNodes []string

	controlChan chan bool
}

func NewEsProbe(clusterName, consulApi string, clusterConfig common.Cluster, consulPeriod, probePeriod, cleaningPeriod time.Duration, controlChan chan bool) (EsProbe, error) {
	var allEverKnownEsNodes []string
	esNodesList, err := common.DiscoverNodesForService(consulApi, clusterConfig.Name)
	if err != nil {
		common.ErrorsCount.Inc()
		log.Fatal("Impossible to discover ES datanodes during bootstrap, exiting")
	}
	allEverKnownEsNodes = common.UpdateEverKnownNodes(allEverKnownEsNodes, esNodesList)

	return EsProbe{
		clusterName:   clusterName,
		consulApi:     consulApi,
		clusterConfig: clusterConfig,

		timeout: probePeriod - 2*time.Second,

		updateDiscoveryTicker: time.NewTicker(consulPeriod),
		executeProbingTicker:  time.NewTicker(probePeriod),
		cleanMetricsTicker:    time.NewTicker(cleaningPeriod),

		esNodesList:         esNodesList,
		allEverKnownEsNodes: allEverKnownEsNodes,

		controlChan: controlChan,
	}, nil
}

func (es *EsProbe) PrepareEsProbing() error {
	// TODO: create durability index + recreate latency index
	return nil
}

func (es *EsProbe) StartEsProbing() error {
	// TODO: do cluster probing
	for {
		select {
		case <-es.controlChan:
			log.Println("Terminating es probe on ", es.clusterName)
			es.cleanMetricsTicker.Stop()
			es.updateDiscoveryTicker.Stop()
			es.executeProbingTicker.Stop()
			common.CleanMetrics(es.esNodesList, es.allEverKnownEsNodes)
			return nil
		case <-es.cleanMetricsTicker.C:
			log.Info("Cleaning Prometheus metrics for unreferenced nodes")
			common.CleanMetrics(es.esNodesList, es.allEverKnownEsNodes)

		case <-es.updateDiscoveryTicker.C:
			// Elasticsearch
			log.Debug("Starting updating ES nodes list")
			updatedList, err := common.DiscoverNodesForService(es.consulApi, es.clusterConfig.Name)
			if err != nil {
				log.Error("Unable to update ES nodes, using last known state")
				common.ErrorsCount.Inc()
				continue
			}

			log.Info("Updating ES nodes list")
			es.allEverKnownEsNodes = common.UpdateEverKnownNodes(es.allEverKnownEsNodes, updatedList)
			es.esNodesList = updatedList

		case <-es.executeProbingTicker.C:
			log.Debug("Starting probing ES nodes")

			sem := new(sync.WaitGroup)
			for _, node := range es.esNodesList {
				sem.Add(1)
				go func(esNode common.Node) {
					defer sem.Done()
					probeElasticsearchNode(&esNode, es.timeout)
				}(node)

			}
			sem.Wait()
		}
	}
}

func probeElasticsearchNode(node *common.Node, timeout time.Duration) error {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{
		Timeout: timeout,
	}

	probingURL := fmt.Sprintf("%v://%v:%v/_cat/indices?v", node.Scheme, node.Ip, node.Port)
	log.Debug("Start probing ", node.Name)

	start := time.Now()
	resp, err := client.Get(probingURL)
	if err != nil {
		log.Debug("Probing failed for ", node.Name, ": ", probingURL, " ", err.Error())
		log.Error(err)
		common.ElasticNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		common.ErrorsCount.Inc()
		return err
	}
	durationNanosec := float64(time.Since(start).Nanoseconds())

	log.Debug("Probe result for ", node.Name, ": ", resp.Status)
	if resp.StatusCode != 200 {
		log.Error("Probing failed for ", node.Name, ": ", probingURL, " ", resp.Status)
		common.ElasticNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		common.ErrorsCount.Inc()
		return fmt.Errorf("ES Probing failed")
	}

	common.ElasticNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(1)
	common.NodeCatLatencySummary.WithLabelValues(node.Cluster, node.Name).Observe(durationNanosec)

	return nil
}
