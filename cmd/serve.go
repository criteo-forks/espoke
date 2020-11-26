// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package cmd

import (
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type ServeCmd struct {
	ConsulApi                  string        `default:"127.0.0.1:8500" help:"127.0.0.1:8500" "consul target api host:port" yaml:"consul_api" short:"a"`
	ConsulPeriod               time.Duration `default:"120s" help:"nodes discovery update interval" yaml:"consul_period"`
	CleaningPeriod             time.Duration `default:"600s" help:"prometheus metrics cleaning interval (for vanished nodes)" yaml:"cleaning_period"`
	ProbePeriod                time.Duration `default:"30s" help:"elasticsearch nodes probing interval" yaml:"probe_period"`
	ElasticsearchConsulService string        `default:"elasticsearch-all" help:"elasticsearch consul service" yaml:"elasticsearch_consul_service"`
	KibanaConsulService        string        `default:"kibana-all" help:"kibana consul service" yaml:"kibana_consul_service"`
	MetricsPort                int           `default:"2112" help:"port where prometheus will expose metrics to" yaml:"metrics_port" short:"p"`
	LogLevel                   string        `default:"info" help:"log level" yaml:"log_level" short:"l"`
}

func (r *ServeCmd) Run() error {
	// Init logger
	log.SetOutput(os.Stdout)
	lvl, err := log.ParseLevel(r.LogLevel)
	if err != nil {
		log.Warning("Log level not recognized, fallback to default level (INFO)")
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
	log.Info("Logger initialized")

	log.Info("Entering serve main loop")
	startMetricsEndpoint(r.MetricsPort)

	log.Info("Discovering ES nodes for the first time")
	var allEverKnownEsNodes []string
	esNodesList, err := discoverNodesForService(r.ConsulApi, r.ElasticsearchConsulService)
	if err != nil {
		errorsCount.Inc()
		log.Fatal("Impossible to discover ES datanodes during bootstrap, exiting")
	}
	allEverKnownEsNodes = updateEverKnownNodes(allEverKnownEsNodes, esNodesList)

	var allEverKnownKibanaNodes []string
	kibanaNodesList, err := discoverNodesForService(r.ConsulApi, r.KibanaConsulService)
	if err != nil {
		errorsCount.Inc()
		log.Fatal("Impossible to discover kibana nodes during bootstrap, exiting")
	}
	allEverKnownKibanaNodes = updateEverKnownNodes(allEverKnownKibanaNodes, kibanaNodesList)

	log.Info("Initializing tickers")
	if r.ConsulPeriod < 60*time.Second {
		log.Warning("Refreshing discovery more than once a minute is not allowed, fallback to 60s")
		r.ConsulPeriod = 60 * time.Second
	}
	log.Info("Discovery update interval: ", r.ConsulPeriod.String())

	if r.ProbePeriod < 20*time.Second {
		log.Warning("Probing elasticsearch nodes more than 3 times a minute is not allowed, fallback to 20s")
		r.ProbePeriod = 20 * time.Second
	}
	log.Info("Probing interval: ", r.ProbePeriod.String())

	if r.CleaningPeriod < 240*time.Second {
		log.Warning("Cleaning Metrics faster than every 4 minutes is not allowed, fallback to 240s")
		r.CleaningPeriod = 240 * time.Second
	}
	log.Info("Metrics pruning interval: ", r.CleaningPeriod.String())

	updateDiscoveryTicker := time.NewTicker(r.ConsulPeriod)
	cleanMetricsTicker := time.NewTicker(r.CleaningPeriod)
	executeProbingTicker := time.NewTicker(r.ProbePeriod)

	for {
		select {
		case <-cleanMetricsTicker.C:
			log.Info("Cleaning Prometheus metrics for unreferenced nodes")
			cleanMetrics(esNodesList, allEverKnownEsNodes)
			cleanMetrics(kibanaNodesList, allEverKnownKibanaNodes)

		case <-updateDiscoveryTicker.C:
			// Elasticsearch
			log.Debug("Starting updating ES nodes list")
			updatedList, err := discoverNodesForService(r.ConsulApi, r.ElasticsearchConsulService)
			if err != nil {
				log.Error("Unable to update ES nodes, using last known state")
				errorsCount.Inc()
				continue
			}

			log.Info("Updating ES nodes list")
			allEverKnownEsNodes = updateEverKnownNodes(allEverKnownEsNodes, updatedList)
			esNodesList = updatedList

			// Kibana
			log.Debug("Starting updating Kibana nodes list")
			kibanaUpdatedList, err := discoverNodesForService(r.ConsulApi, r.KibanaConsulService)
			if err != nil {
				log.Error("Unable to update Kibana nodes, using last known state")
				errorsCount.Inc()
				continue
			}

			log.Info("Updating kibana nodes list")
			allEverKnownKibanaNodes = updateEverKnownNodes(allEverKnownKibanaNodes, kibanaUpdatedList)
			kibanaNodesList = kibanaUpdatedList

		case <-executeProbingTicker.C:
			log.Debug("Starting probing ES nodes")

			sem := new(sync.WaitGroup)
			for _, node := range esNodesList {
				sem.Add(1)
				go func(esNode esnode) {
					defer sem.Done()
					probeElasticsearchNode(&esNode, r.ProbePeriod)
				}(node)

			}
			sem.Wait()

			log.Debug("Starting probing Kibana nodes")
			for _, node := range kibanaNodesList {
				sem.Add(1)
				go func(kibanaNode esnode) {
					defer sem.Done()
					probeKibanaNode(&kibanaNode, r.ProbePeriod)
				}(node)

			}
			sem.Wait()
		}
	}
}
