package watcher

import (
	"github.com/criteo-forks/espoke/common"
	"github.com/criteo-forks/espoke/probe"
	"github.com/hashicorp/consul/api"
	log "github.com/sirupsen/logrus"
	"time"
)

// Watcher manages the pool of S3 endpoints to monitor
type Watcher struct {
	config *common.Config

	consulClient *api.Client

	elasticsearchClusters map[string](chan bool)
	kibanaClusters        map[string](chan bool)
}

// NewWatcher creates a new watcher and prepare the consul client
func NewWatcher(config *common.Config) (Watcher, error) {
	consulClient, err := common.NewClient(config.ConsulApi)
	if err != nil {
		return Watcher{}, err
	}
	return Watcher{
		config: config,

		consulClient: consulClient,

		elasticsearchClusters: make(map[string]chan bool),
		kibanaClusters:        make(map[string]chan bool),
	}, nil
}

// WatchPools poll consul services with specified tag and create
// probe gorountines
func (w *Watcher) WatchPools() error {
	for {
		// Elasticsearch service
		esServicesFromConsul, err := common.GetServices(w.consulClient, w.config.ElasticsearchConsulTag)
		if err != nil {
			log.Error(err)
			common.ErrorsCount.Inc()
		}

		esWatchedServices := w.getWatchedServices(w.elasticsearchClusters)

		esServicesToAdd, sServicesToRemove := w.getServicesToModify(esServicesFromConsul, esWatchedServices)
		w.flushOldProbes(sServicesToRemove, w.elasticsearchClusters)
		w.createNewEsProbes(esServicesToAdd)

		// Kibana service
		kibanaServicesFromConsul, err := common.GetServices(w.consulClient, w.config.KibanaConsulTag)
		if err != nil {
			log.Error(err)
			common.ErrorsCount.Inc()
		}

		kibanaWatchedServices := w.getWatchedServices(w.kibanaClusters)

		kibanaServicesToAdd, kibanaServicesToRemove := w.getServicesToModify(kibanaServicesFromConsul, kibanaWatchedServices)
		w.flushOldProbes(kibanaServicesToRemove, w.kibanaClusters)
		w.createNewKibanaProbes(kibanaServicesToAdd)

		time.Sleep(w.config.ConsulPeriod)
	}
}

func (w *Watcher) getWatchedServices(watchedClusters map[string](chan bool)) []string {
	var currentServices []string

	for k := range watchedClusters {
		currentServices = append(currentServices, k)
	}
	return currentServices
}

func (w *Watcher) createNewEsProbes(servicesToAdd map[string]common.Cluster) {
	var probeChan chan bool
	for cluster, clusterConfig := range servicesToAdd {
		log.Printf("Creating new es probe for: %s", cluster)

		endpoint, err := common.GetEndpointFromConsul(w.consulClient, clusterConfig.Name, w.config.ElasticsearchEndpointSuffix)
		if err != nil {
			log.Errorf("Could not generate endpoint from consul: %s", err.Error())
			common.ErrorsCount.Inc()
			continue
		}

		probeChan = make(chan bool)
		esProbe, err := probe.NewEsProbe(cluster, endpoint, clusterConfig, w.config, w.consulClient, probeChan)

		if err != nil {
			log.Errorf("Error while creating probe: %s", err.Error())
			common.ErrorsCount.Inc()
			continue
		}

		err = esProbe.PrepareEsProbing()
		if err != nil {
			log.Errorf("Error while preparing probe: %s", err.Error())
			common.ErrorsCount.Inc()
			close(probeChan)
			continue
		}

		w.elasticsearchClusters[cluster] = probeChan
		go esProbe.StartEsProbing()
	}
}
func (w *Watcher) createNewKibanaProbes(servicesToAdd map[string]common.Cluster) {
	var probeChan chan bool
	for cluster, clusterConfig := range servicesToAdd {
		log.Printf("Creating new kibana probe for: %s", cluster)
		probeChan = make(chan bool)
		esProbe, err := probe.NewKibanaProbe(cluster, clusterConfig, w.config, w.consulClient, probeChan)

		if err != nil {
			log.Error(err)
			common.ErrorsCount.Inc()
			continue
		}

		w.kibanaClusters[cluster] = probeChan
		go esProbe.StartKibanaProbing()
	}
}
func (w *Watcher) flushOldProbes(servicesToRemove []string, watchedClusters map[string](chan bool)) {
	var ok bool
	var probeChan chan bool
	for _, name := range servicesToRemove {
		log.Infof("Removing old probe for: %s", name)
		probeChan, ok = watchedClusters[name]
		if ok {
			delete(watchedClusters, name)
			probeChan <- false
			close(probeChan)
		}
	}
}

func (w *Watcher) getServicesToModify(servicesFromConsul map[string]common.Cluster, watchedServices []string) (map[string]common.Cluster, []string) {
	servicesToAdd := make(map[string]common.Cluster)
	for cluster, clusterConfig := range servicesFromConsul {
		if !w.stringInSlice(cluster, watchedServices) {
			servicesToAdd[cluster] = clusterConfig
		}
	}

	var servicesToRemove []string
	for _, cluster := range watchedServices {
		_, ok := servicesFromConsul[cluster]
		if !ok {
			servicesToRemove = append(servicesToRemove, cluster)
		}
	}
	return servicesToAdd, servicesToRemove
}

func (w *Watcher) stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
