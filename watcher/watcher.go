package watcher

import (
	"github.com/criteo-forks/espoke/common"
	"github.com/criteo-forks/espoke/probe"
	log "github.com/sirupsen/logrus"
	"time"
)

// Watcher manages the pool of S3 endpoints to monitor
type Watcher struct {
	elasticsearchConsulTag string
	kibanaConsulTag        string
	consulApi              string

	consulPeriod   time.Duration
	probePeriod    time.Duration
	cleaningPeriod time.Duration

	elasticsearchClusters map[string](chan bool)
	kibanaClusters        map[string](chan bool)
}

// NewWatcher creates a new watcher and prepare the consul client
func NewWatcher(elasticsearchConsulTag, kibanaConsulTag, consulApi string, consulPeriod, cleaningPeriod, probePeriod time.Duration) Watcher {
	log.Info("Discovering ES nodes for the first time")

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
		elasticsearchConsulTag: elasticsearchConsulTag,
		kibanaConsulTag:        kibanaConsulTag,
		consulApi:              consulApi,

		consulPeriod:   consulPeriod,
		probePeriod:    probePeriod,
		cleaningPeriod: cleaningPeriod,

		elasticsearchClusters: make(map[string]chan bool),
		kibanaClusters:        make(map[string]chan bool),
	}
}

// WatchPools poll consul services with specified tag and create
// probe gorountines
func (w *Watcher) WatchPools() error {

	for {
		// Elasticsearch service
		esServicesFromConsul, err := common.GetServices(w.consulApi, w.elasticsearchConsulTag)
		if err != nil {
			return err
		}

		esWatchedServices := w.getWatchedServices(w.elasticsearchClusters)

		esServicesToAdd, sServicesToRemove := w.getServicesToModify(esServicesFromConsul, esWatchedServices)
		w.flushOldProbes(sServicesToRemove, w.elasticsearchClusters)
		w.createNewEsProbes(esServicesToAdd)

		// Kibana service
		kibanaServicesFromConsul, err := common.GetServices(w.consulApi, w.kibanaConsulTag)
		if err != nil {
			return err
		}

		kibanaWatchedServices := w.getWatchedServices(w.kibanaClusters)

		kibanaServicesToAdd, kibanaServicesToRemove := w.getServicesToModify(kibanaServicesFromConsul, kibanaWatchedServices)
		w.flushOldProbes(kibanaServicesToRemove, w.kibanaClusters)
		w.createNewKibanaProbes(kibanaServicesToAdd)

		time.Sleep(w.consulPeriod)
	}
	return nil
}

func (w *Watcher) getWatchedServices(watchedClusters map[string](chan bool)) []string {
	currentServices := []string{}

	for k, _ := range watchedClusters {
		currentServices = append(currentServices, k)
	}
	return currentServices
}

func (w *Watcher) createNewEsProbes(servicesToAdd map[string]common.Cluster) {
	var probeChan chan bool
	for cluster, clusterConfig := range servicesToAdd {
		log.Printf("Creating new es probe for: %s", cluster)
		probeChan = make(chan bool)
		esProbe, err := probe.NewEsProbe(cluster, w.consulApi, clusterConfig, w.consulPeriod, w.probePeriod, w.cleaningPeriod, probeChan)

		if err != nil {
			log.Println("Error while creating probe:", err)
			continue
		}

		err = esProbe.PrepareEsProbing()
		if err != nil {
			log.Println("Error while preparing probe:", err)
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
		esProbe, err := probe.NewKibanaProbe(cluster, w.consulApi, clusterConfig, w.consulPeriod, w.probePeriod, w.cleaningPeriod, probeChan)

		if err != nil {
			log.Println("Error while creating probe:", err)
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
		log.Printf("Removing old probe for: %s", name)
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
