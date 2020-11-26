package watcher

import (
	"github.com/criteo-forks/espoke/common"
	"github.com/criteo-forks/espoke/probe"
	"github.com/criteo-forks/espoke/prometheus"
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
)

// Watcher manages the pool of S3 endpoints to monitor
type Watcher struct {
	elasticsearchConsulService string
	kibanaConsulService        string
	consulApi                  string

	probePeriod time.Duration

	updateDiscoveryTicker *time.Ticker
	cleanMetricsTicker    *time.Ticker
	executeProbingTicker  *time.Ticker

	esNodesList         []common.Esnode
	allEverKnownEsNodes []string

	kibanaNodesList         []common.Esnode
	allEverKnownKibanaNodes []string

	elasticsearchCluster map[string](chan bool)
	kibanaCluster        map[string](chan bool)
}

// NewWatcher creates a new watcher and prepare the consul client
func NewWatcher(elasticsearchConsulService, kibanaConsulService, consulApi string, consulPeriod, cleaningPeriod, probePeriod time.Duration) Watcher {
	log.Info("Discovering ES nodes for the first time")
	var allEverKnownEsNodes []string
	esNodesList, err := discoverNodesForService(consulApi, elasticsearchConsulService)
	if err != nil {
		prometheus.ErrorsCount.Inc()
		log.Fatal("Impossible to discover ES datanodes during bootstrap, exiting")
	}
	allEverKnownEsNodes = updateEverKnownNodes(allEverKnownEsNodes, esNodesList)

	var allEverKnownKibanaNodes []string
	kibanaNodesList, err := discoverNodesForService(consulApi, kibanaConsulService)
	if err != nil {
		prometheus.ErrorsCount.Inc()
		log.Fatal("Impossible to discover kibana nodes during bootstrap, exiting")
	}
	allEverKnownKibanaNodes = updateEverKnownNodes(allEverKnownKibanaNodes, kibanaNodesList)

	log.Info("Initializing tickers")
	if consulPeriod < 60*time.Second {
		log.Warning("Refreshing discovery more than once a minute is not allowed, fallback to 60s")
		consulPeriod = 60 * time.Second
	}
	log.Info("Discovery update interval: ", consulPeriod.String())

	if probePeriod < 20*time.Second {
		log.Warning("Probing elasticsearch nodes more than 3 times a minute is not allowed, fallback to 20s")
		probePeriod = 20 * time.Second
	}
	log.Info("Probing interval: ", probePeriod.String())

	if cleaningPeriod < 240*time.Second {
		log.Warning("Cleaning Metrics faster than every 4 minutes is not allowed, fallback to 240s")
		cleaningPeriod = 240 * time.Second
	}
	log.Info("Metrics pruning interval: ", cleaningPeriod.String())

	return Watcher{
		elasticsearchConsulService: elasticsearchConsulService,
		kibanaConsulService:        kibanaConsulService,
		consulApi:                  consulApi,

		probePeriod: probePeriod,

		updateDiscoveryTicker: time.NewTicker(consulPeriod),
		cleanMetricsTicker:    time.NewTicker(cleaningPeriod),
		executeProbingTicker:  time.NewTicker(probePeriod),

		esNodesList:         esNodesList,
		allEverKnownEsNodes: allEverKnownEsNodes,

		kibanaNodesList:         kibanaNodesList,
		allEverKnownKibanaNodes: allEverKnownKibanaNodes,

		elasticsearchCluster: make(map[string]chan bool),
		kibanaCluster:        make(map[string]chan bool),
	}
}

// WatchPools poll consul services with specified tag and create
// probe gorountines
func (w *Watcher) WatchPools() {
	for {
		select {
		case <-w.cleanMetricsTicker.C:
			log.Info("Cleaning Prometheus metrics for unreferenced nodes")
			prometheus.CleanMetrics(w.esNodesList, w.allEverKnownEsNodes)
			prometheus.CleanMetrics(w.kibanaNodesList, w.allEverKnownKibanaNodes)

		case <-w.updateDiscoveryTicker.C:
			// Elasticsearch
			log.Debug("Starting updating ES nodes list")
			updatedList, err := discoverNodesForService(w.consulApi, w.elasticsearchConsulService)
			if err != nil {
				log.Error("Unable to update ES nodes, using last known state")
				prometheus.ErrorsCount.Inc()
				continue
			}

			log.Info("Updating ES nodes list")
			w.allEverKnownEsNodes = updateEverKnownNodes(w.allEverKnownEsNodes, updatedList)
			w.esNodesList = updatedList

			// Kibana
			log.Debug("Starting updating Kibana nodes list")
			kibanaUpdatedList, err := discoverNodesForService(w.consulApi, w.kibanaConsulService)
			if err != nil {
				log.Error("Unable to update Kibana nodes, using last known state")
				prometheus.ErrorsCount.Inc()
				continue
			}

			log.Info("Updating kibana nodes list")
			w.allEverKnownKibanaNodes = updateEverKnownNodes(w.allEverKnownKibanaNodes, kibanaUpdatedList)
			w.kibanaNodesList = kibanaUpdatedList

		case <-w.executeProbingTicker.C:
			log.Debug("Starting probing ES nodes")

			sem := new(sync.WaitGroup)
			for _, node := range w.esNodesList {
				sem.Add(1)
				go func(esNode common.Esnode) {
					defer sem.Done()
					probe.ProbeElasticsearchNode(&esNode, w.probePeriod)
				}(node)

			}
			sem.Wait()

			log.Debug("Starting probing Kibana nodes")
			for _, node := range w.kibanaNodesList {
				sem.Add(1)
				go func(kibanaNode common.Esnode) {
					defer sem.Done()
					probe.ProbeKibanaNode(&kibanaNode, w.probePeriod)
				}(node)

			}
			sem.Wait()
		}
	}
}
