// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package probe

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/criteo-forks/espoke/common"
	"github.com/pkg/errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	elasticsearch7 "github.com/elastic/go-elasticsearch/v7"
	log "github.com/sirupsen/logrus"
)

var (
	DURABILITY_INDEX_NAME  = ".espoke.durability"
	SEARCH_INDEX_NAME      = ".espoke.search"
	NUMBER_ITEMS_DURBILITY = 101 // TODO add more docs
	DATA_ES_DOC            = "While the exact amount of text data in a kilobyte (KB) or megabyte (MB) can vary " +
		"depending on the nature of a document, a kilobyte can hold about half of a page of text, while a megabyte " +
		"holds about 500 pages of text."
)

type EsDocument struct {
	Name     string
	EventTye string
	Team     string
	Counter  int
	Data     string
}

type EsProbe struct {
	clusterName   string
	clusterConfig common.Cluster
	config        *common.Config
	client        *elasticsearch7.Client

	timeout time.Duration

	updateDiscoveryTicker *time.Ticker
	cleanMetricsTicker    *time.Ticker
	executeProbingTicker  *time.Ticker

	esNodesList         []common.Node
	allEverKnownEsNodes []string

	controlChan chan bool
}

// TODO inc .ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
func probeElasticsearchNode(node *common.Node, timeout time.Duration, username, password string) error {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{
		Timeout: timeout,
	}

	probingURL := fmt.Sprintf("%v://%v:%v/_cat/health?v", node.Scheme, node.Ip, node.Port)
	log.Debug("Start probing ", node.Name)

	start := time.Now()
	req, err := http.NewRequest("GET", probingURL, nil)
	req.SetBasicAuth(username, password)
	resp, err := client.Do(req)
	if err != nil {
		log.Debug("Probing failed for ", node.Name, ": ", probingURL, " ", err.Error())
		log.Error(err)
		common.ElasticNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		common.ErrorsCount.Inc()
		return err
	}
	durationNanoSec := float64(time.Since(start).Nanoseconds())

	log.Debug("Probe result for ", node.Name, ": ", resp.Status)
	if resp.StatusCode != 200 {
		log.Error("Probing failed for ", node.Name, ": ", probingURL, " ", resp.Status)
		common.ElasticNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		common.ErrorsCount.Inc()
		return fmt.Errorf("ES Probing failed")
	}

	common.ElasticNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(1)
	common.NodeCatLatencySummary.WithLabelValues(node.Cluster, node.Name).Observe(durationNanoSec)

	return nil
}

func initEsClient(scheme, endpoint, username, passsword string) (*elasticsearch7.Client, error) {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	cfg := elasticsearch7.Config{
		Addresses: []string{
			fmt.Sprintf("%v://%v", scheme, endpoint),
		},
		Username: username,
		Password: passsword,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	es, err := elasticsearch7.NewClient(cfg)
	if err != nil {
		log.Fatalf("Error creating the client: %s", err)
		return nil, err
	}
	return es, nil
}

func NewEsProbe(clusterName, endpoint string, clusterConfig common.Cluster, config *common.Config, controlChan chan bool) (EsProbe, error) {
	var allEverKnownEsNodes []string
	esNodesList, err := common.DiscoverNodesForService(config.ConsulApi, clusterConfig.Name)
	if err != nil {
		common.ErrorsCount.Inc()
		log.Fatal("Impossible to discover ES nodes during bootstrap, exiting")
	}
	allEverKnownEsNodes = common.UpdateEverKnownNodes(allEverKnownEsNodes, esNodesList)

	client, err := initEsClient(clusterConfig.Scheme, endpoint, config.ElasticsearchUser, config.ElasticsearchPassword)
	if err != nil {
		return EsProbe{}, err
	}

	return EsProbe{
		clusterName:   clusterName,
		clusterConfig: clusterConfig,
		config:        config,
		client:        client,

		timeout: config.ProbePeriod - 2*time.Second,

		updateDiscoveryTicker: time.NewTicker(config.ConsulPeriod),
		executeProbingTicker:  time.NewTicker(config.ProbePeriod),
		cleanMetricsTicker:    time.NewTicker(config.CleaningPeriod),

		esNodesList:         esNodesList,
		allEverKnownEsNodes: allEverKnownEsNodes,

		controlChan: controlChan,
	}, nil
}

func (es *EsProbe) PrepareEsProbing() error {
	// TODO: recreate latency index
	// Check index available
	if err := es.createMissingIndex(DURABILITY_INDEX_NAME); err != nil {
		return err
	}
	if err := es.createMissingIndex(SEARCH_INDEX_NAME); err != nil {
		return err
	}

	// Count docs on durability index and put docs if needed
	number_of_current_durability_documents, _, err := es.countNumberOfDurabilityDocs()
	if err != nil {
		return err
	}

	if err := es.indexDurabilityDocuments(number_of_current_durability_documents); err != nil {
		return err
	}

	return nil
}

func (es *EsProbe) StartEsProbing() error {
	for {
		select {
		case <-es.controlChan:
			log.Println("Terminating es probe on ", es.clusterName)
			es.cleanMetricsTicker.Stop()
			es.updateDiscoveryTicker.Stop()
			es.executeProbingTicker.Stop()
			common.CleanNodeMetrics(es.esNodesList, es.allEverKnownEsNodes)
			common.CleanClusterMetrics(es.clusterName, []string{DURABILITY_INDEX_NAME, SEARCH_INDEX_NAME})
			return nil

		case <-es.cleanMetricsTicker.C:
			log.Info("Cleaning Prometheus metrics for unreferenced nodes")
			common.CleanNodeMetrics(es.esNodesList, es.allEverKnownEsNodes)

		case <-es.updateDiscoveryTicker.C:
			// Elasticsearch
			log.Debug("Starting updating ES nodes list")
			updatedList, err := common.DiscoverNodesForService(es.config.ConsulApi, es.clusterConfig.Name)
			if err != nil {
				log.Error("Unable to update ES nodes, using last known state")
				common.ErrorsCount.Inc()
				continue
			}

			log.Info("Updating ES nodes list")
			es.allEverKnownEsNodes = common.UpdateEverKnownNodes(es.allEverKnownEsNodes, updatedList)
			es.esNodesList = updatedList

		case <-es.executeProbingTicker.C:
			sem := new(sync.WaitGroup)
			log.Debug("Starting probing ES cluster")
			// Send index state green=> 0, yellow=>...
			sem.Add(1)
			// Check index status
			go func() {
				defer sem.Done()
				if err := es.setIndexStatus(DURABILITY_INDEX_NAME); err != nil {
					log.Error(err)
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
				}
				if err := es.setIndexStatus(SEARCH_INDEX_NAME); err != nil {
					log.Error(err)
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
				}
			}()
			// Durability check
			sem.Add(1)
			go func() {
				defer sem.Done()
				number_of_current_durability_documents, durationNanoSec, err := es.countNumberOfDurabilityDocs()
				if err != nil {
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
					log.Error(err)

				}
				common.ClusterLatencySummary.WithLabelValues(es.clusterName, DURABILITY_INDEX_NAME, "count").Observe(durationNanoSec)
				common.ClusterDurabilityDocumentsCount.WithLabelValues(es.clusterName).Set(number_of_current_durability_documents)

				if err := es.searchDurabilityDocuments(); err != nil {
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
					log.Error(err)
				}
			}()
			// TODO later search check -> move it to a special tick to do it more often
			// Ingestion/Get/Delete latency
			sem.Add(1)
			go func() {
				defer sem.Done()
				// Send event
				documentID := "search-document-1"
				esDoc := &EsDocument{
					Name:     documentID,
					Counter:  1,
					EventTye: "search",
					Team:     "nosql",
					Data:     DATA_ES_DOC,
				}
				durationNanoSec, err := es.indexDocument(SEARCH_INDEX_NAME, documentID, esDoc)
				if err != nil {
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
					log.Error(err)
				}
				common.ClusterLatencySummary.WithLabelValues(es.clusterName, SEARCH_INDEX_NAME, "index").Observe(durationNanoSec)

				// Get event
				if err := es.getDocument(SEARCH_INDEX_NAME, documentID); err != nil {
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
					log.Error(err)
				}

				// Delete event
				if err := es.deleteDocument(SEARCH_INDEX_NAME, documentID); err != nil {
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
					log.Error(err)
				}
			}()

			log.Debug("Starting probing ES nodes")
			for _, node := range es.esNodesList {
				sem.Add(1)
				go func(esNode common.Node) {
					defer sem.Done()
					probeElasticsearchNode(&esNode, es.timeout, es.config.ElasticsearchUser, es.config.ElasticsearchPassword)
				}(node)
			}
			sem.Wait()
		}
	}
}

func (es *EsProbe) deleteDocument(index, documentID string) error {
	start := time.Now()
	res, err := es.client.Delete(
		index,
		documentID)
	durationNanoSec := float64(time.Since(start).Nanoseconds())

	if err != nil {
		return errors.Wrapf(err, "Failed to delete document %s on %s:%s", documentID, es.clusterName, index)
	}
	defer res.Body.Close()

	if res.IsError() {
		return errors.Errorf("Error delete document %s on %s:%s: %s", documentID, es.clusterName, index, res.String())
	}

	common.ClusterLatencySummary.WithLabelValues(es.clusterName, index, "delete").Observe(durationNanoSec)
	return nil
}

func (es *EsProbe) getDocument(index, documentID string) error {
	start := time.Now()
	res, err := es.client.Get(
		index,
		documentID)
	durationNanoSec := float64(time.Since(start).Nanoseconds())

	if err != nil {
		return errors.Wrapf(err, "Failed to get document %s on %s:%s", documentID, es.clusterName, index)
	}
	defer res.Body.Close()

	if res.IsError() {
		return errors.Errorf("Error get document %s on %s:%s: %s", documentID, es.clusterName, index, res.String())
	}

	common.ClusterLatencySummary.WithLabelValues(es.clusterName, index, "get").Observe(durationNanoSec)
	return nil
}

func (es *EsProbe) countNumberOfDurabilityDocs() (float64, float64, error) {
	var r map[string]interface{}
	start := time.Now()
	res, err := es.client.Count(
		es.client.Count.WithIndex(DURABILITY_INDEX_NAME),
	)
	durationNanoSec := float64(time.Since(start).Nanoseconds())

	if err != nil {
		return 0, 0, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return 0, 0, errors.Errorf("Error counting number of durability documents: %s", res.String())
	}

	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return 0, 0, errors.Wrapf(err, "Error parsing the durability docs count response body as json")
	}

	number_of_current_durability_documents, ok := r["count"].(float64)
	if !ok {
		return 0, 0, errors.Errorf("Durability response count doesn't contains count field")
	}
	return number_of_current_durability_documents, durationNanoSec, nil
}

func (es *EsProbe) indexDurabilityDocuments(number_of_current_durability_documents float64) error {
	// TODO improve this stage to be faster (bulk?)
	if int(number_of_current_durability_documents) < NUMBER_ITEMS_DURBILITY+1 {
		esDoc := &EsDocument{
			EventTye: "durability",
			Team:     "nosql",
			Data:     DATA_ES_DOC,
		}
		for i := int(number_of_current_durability_documents) + 1; i < NUMBER_ITEMS_DURBILITY+1; i++ {
			// Build the request body.
			esDoc.Name = fmt.Sprintf("document-%d", i)
			esDoc.Counter = i

			if _, err := es.indexDocument(DURABILITY_INDEX_NAME, strconv.Itoa(i), esDoc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (es *EsProbe) indexDocument(index, documentID string, esDoc *EsDocument) (float64, error) {
	jsonDoc, err := json.Marshal(esDoc)
	if err != nil {
		return 0, errors.Wrapf(err, "Failed to create json document in %s:%s", es.clusterName, index)
	}

	start := time.Now()
	res, err := es.client.Index(
		index,
		bytes.NewReader(jsonDoc),
		es.client.Index.WithDocumentID(documentID),
	)
	durationNanoSec := float64(time.Since(start).Nanoseconds())

	if err != nil {
		return 0, errors.Wrapf(err, "Failed to index document in %s:%s %d", es.clusterName, index)
	}
	defer res.Body.Close()

	if res.IsError() {
		return 0, errors.Errorf("Document index creation failed in %s:%s: %s", es.clusterName, index, res.String())
	}
	return durationNanoSec, nil
}

func (es *EsProbe) createMissingIndex(index string) error {
	res, err := es.client.Indices.Exists([]string{index})
	if err != nil {
		return errors.Wrapf(err, "Failed to check if index %s exist", index)
	}
	defer res.Body.Close()

	if res.StatusCode == 404 {
		res, err := es.client.Indices.Create(index)
		if err != nil {
			return errors.Wrapf(err, "Failed to create index %s", index)
		}
		defer res.Body.Close()

		if res.IsError() {
			return errors.Errorf("Index creation for %s response error: %s", index, res.String())
		}
	} else if res.IsError() {
		return errors.Errorf("Index exist check for %s response error: %s", index, res.String())
	}
	return nil
}

func (es *EsProbe) searchDurabilityDocuments() error {
	var buf bytes.Buffer
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"range": map[string]interface{}{
				"Counter": map[string]interface{}{
					"gte": 10,
					"lte": 80,
				},
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return errors.Wrapf(err, "Error encoding search query")
	}

	start := time.Now()
	res, err := es.client.Search(
		es.client.Search.WithIndex(DURABILITY_INDEX_NAME),
		es.client.Search.WithBody(&buf),
		es.client.Search.WithTrackTotalHits(true),
	)
	durationNanoSec := float64(time.Since(start).Nanoseconds())

	if err != nil {
		return errors.Wrapf(err, "Error searching documents on durability index")
	}
	defer res.Body.Close()

	if res.IsError() {
		return errors.Errorf("Durability search response body has an error on durability index for cluster %s", es.clusterName)
	}

	var r map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return errors.Wrapf(err, "Error parsing durability search response body for cluster %s", es.clusterName)
	}

	indices, ok := r["hits"].(map[string]interface{})
	if !ok {
		return errors.Errorf("Durability search response doesn't contains hits field on cluster %s", es.clusterName)
	}

	var total float64
	if strings.HasPrefix(es.clusterConfig.Version, "6") {
		total, ok = indices["total"].(float64)
		if !ok {
			return errors.Errorf("Durability search response doesn't contains hits.total field for %s on cluster %s", es.clusterName)
		}
	} else {
		intermediate_total, ok := indices["total"].(map[string]interface{})
		if !ok {
			return errors.Errorf("Durability search response doesn't contains hits.total field for %s on cluster %s", es.clusterName)
		}
		total, ok = intermediate_total["value"].(float64)
		if !ok {
			return errors.Errorf("Durability search response doesn't contains hits.total field for %s on cluster %s", es.clusterName)
		}
	}

	common.ClusterLatencySummary.WithLabelValues(es.clusterName, DURABILITY_INDEX_NAME, "search").Observe(durationNanoSec)
	common.ClusterSearchDocumentsHits.WithLabelValues(es.clusterName, DURABILITY_INDEX_NAME).Set(total)
	return nil
}

func (es *EsProbe) setIndexStatus(index string) error {
	var r map[string]interface{}
	res, err := es.client.Cluster.Health(
		es.client.Cluster.Health.WithIndex(index),
		es.client.Cluster.Health.WithLevel("indices"),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return errors.Errorf("Error checking index %s on cluster %s status: %s", index, es.clusterName, res.String())
	}

	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return errors.Wrapf(err, "Error reading index status response for %s on cluster %s", index, es.clusterName)
	}

	indices, ok := r["indices"].(map[string]interface{})
	if !ok {
		return errors.Errorf("Index status response doesn't contains indices field for %s on cluster %s", index, es.clusterName)
	}
	index_map, ok := indices[index].(map[string]interface{})
	if !ok {
		return errors.Errorf("Index status response doesn't contains indices.%s field on cluster %s", index, es.clusterName)
	}
	index_status, ok := index_map["status"]
	if !ok {
		return errors.Errorf("Index status response doesn't contains indices.%s.status field on cluster %s", index, es.clusterName)
	}
	var indexStatusCode float64
	if index_status == "green" {
		indexStatusCode = 0
	} else if index_status == "yellow" {
		indexStatusCode = 1
	} else {
		indexStatusCode = 2
	}
	common.ElasticNodeAvailabilityGauge.WithLabelValues(es.clusterName, index).Set(indexStatusCode)
	return nil
}
