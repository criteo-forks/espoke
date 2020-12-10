// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package probe

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/criteo-forks/espoke/common"
	"github.com/google/uuid"
	"github.com/hashicorp/consul/api"
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
	DATA_ES_DOC = "While the exact amount of text data in a kilobyte (KB) or megabyte (MB) can vary " +
		"depending on the nature of a document, a kilobyte can hold about half of a page of text, while a megabyte " +
		"holds about 500 pages of text."
)

const millisecondInMinute = 60_000

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

	consulClient *api.Client

	timeout time.Duration

	updateDiscoveryTicker                 *time.Ticker
	cleanMetricsTicker                    *time.Ticker
	executeClusterDurabilityProbingTicker *time.Ticker
	executeClusterLatencyProbingTicker    *time.Ticker
	executeNodeProbingTicker              *time.Ticker

	esNodesList         []common.Node
	allEverKnownEsNodes []string

	controlChan chan bool
}

func NewEsProbe(clusterName, endpoint string, clusterConfig common.Cluster, config *common.Config, consulClient *api.Client, controlChan chan bool) (EsProbe, error) {
	var allEverKnownEsNodes []string
	esNodesList, err := common.DiscoverNodesForService(consulClient, clusterConfig.Name)
	if err != nil {
		return EsProbe{}, errors.Wrapf(err, "Impossible to discover ES nodes during bootstrap for cluster %s", clusterName)
	}
	allEverKnownEsNodes = common.UpdateEverKnownNodes(allEverKnownEsNodes, esNodesList)

	client, err := initEsClient(clusterConfig.Scheme, endpoint, config.ElasticsearchUser, config.ElasticsearchPassword)
	if err != nil {
		return EsProbe{}, errors.Wrapf(err, "Failed to init elasticsearch client for cluster %s", clusterName)
	}

	return EsProbe{
		clusterName:   clusterName,
		clusterConfig: clusterConfig,
		config:        config,
		client:        client,

		consulClient: consulClient,

		timeout: config.ProbePeriod - 2*time.Second,

		updateDiscoveryTicker:                 time.NewTicker(config.ConsulPeriod),
		executeClusterDurabilityProbingTicker: time.NewTicker(config.ProbePeriod),
		executeClusterLatencyProbingTicker:    time.NewTicker(time.Duration(millisecondInMinute/config.LatencyProbeRatePerMin) * time.Millisecond),
		executeNodeProbingTicker:              time.NewTicker(config.ProbePeriod),
		cleanMetricsTicker:                    time.NewTicker(config.CleaningPeriod),

		esNodesList:         esNodesList,
		allEverKnownEsNodes: allEverKnownEsNodes,

		controlChan: controlChan,
	}, nil
}

func (es *EsProbe) PrepareEsProbing() error {
	// TODO: recreate latency index
	// Check index available
	if err := es.createMissingIndex(es.config.ElasticsearchDurabilityIndex); err != nil {
		return err
	}
	if err := es.createMissingIndex(es.config.ElasticsearchLatencyIndex); err != nil {
		return err
	}

	// Count docs on durability index and put docs if needed
	number_of_current_durability_documents, _, err := es.countNumberOfDurabilityDocs()
	if err != nil {
		return err
	}

	if err := es.fillDurabilityBucketWithMissingDocs(number_of_current_durability_documents); err != nil {
		return err
	}

	return nil
}

func (es *EsProbe) StartEsProbing() error {
	for {
		select {
		case <-es.controlChan:
			log.Infof("Terminating es probe on %s", es.clusterName)
			es.cleanMetricsTicker.Stop()
			es.updateDiscoveryTicker.Stop()
			es.executeClusterDurabilityProbingTicker.Stop()
			es.executeClusterLatencyProbingTicker.Stop()
			es.executeNodeProbingTicker.Stop()
			common.CleanNodeMetrics(es.esNodesList, es.allEverKnownEsNodes)
			common.CleanClusterMetrics(es.clusterName, []string{es.config.ElasticsearchDurabilityIndex, es.config.ElasticsearchLatencyIndex})
			return nil

		case <-es.cleanMetricsTicker.C:
			//TODO move this to the update node and only remove the node deleted
			log.Infof("Cleaning Prometheus metrics for unreferenced nodes for cluster %s", es.clusterName)
			common.CleanNodeMetrics(es.esNodesList, es.allEverKnownEsNodes)

		case <-es.updateDiscoveryTicker.C:
			// Elasticsearch
			log.Infof("Starting updating ES nodes list on cluster %s", es.clusterName)
			updatedList, err := common.DiscoverNodesForService(es.consulClient, es.clusterConfig.Name)
			if err != nil {
				log.Error("Unable to update ES nodes, using last known state:", err)
				common.ErrorsCount.Inc()
				continue
			}

			log.Infof("Updating ES nodes list on cluster %s", es.clusterName)
			es.allEverKnownEsNodes = common.UpdateEverKnownNodes(es.allEverKnownEsNodes, updatedList)
			es.esNodesList = updatedList

		case <-es.executeClusterDurabilityProbingTicker.C:
			sem := new(sync.WaitGroup)
			log.Infof("Starting probing durability for cluster %s", es.clusterName)
			// Send index state green=> 0, yellow=>...
			sem.Add(1)
			// Check index status
			go func() {
				defer sem.Done()
				if err := es.setIndexStatus(es.config.ElasticsearchDurabilityIndex); err != nil {
					log.Error(err)
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
				}
			}()
			// Durability check
			// TODO read documents and compare to expected values
			sem.Add(1)
			go func() {
				defer sem.Done()
				number_of_current_durability_documents, durationMilliSec, err := es.countNumberOfDurabilityDocs()
				if err != nil {
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
					log.Error(err)
				}
				common.ClusterLatencySummary.WithLabelValues(es.clusterName, es.config.ElasticsearchDurabilityIndex, "count").Observe(durationMilliSec)
				common.ClusterLatencyHistogram.WithLabelValues(es.clusterName, es.config.ElasticsearchDurabilityIndex, "count").Observe(durationMilliSec)
				common.ClusterDurabilityDocumentsCount.WithLabelValues(es.clusterName).Set(number_of_current_durability_documents)

				if err := es.searchDurabilityDocuments(); err != nil {
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
					log.Error(err)
				}
			}()
			sem.Wait()
		case <-es.executeClusterLatencyProbingTicker.C:
			sem := new(sync.WaitGroup)
			log.Debugf("Starting probing latency cluster %s", es.clusterName)
			// Send index state green=> 0, yellow=>...
			sem.Add(1)
			// Check index status
			go func() {
				defer sem.Done()
				if err := es.setIndexStatus(es.config.ElasticsearchLatencyIndex); err != nil {
					log.Error(err)
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
				}
			}()
			// TODO later search check -> move it to a special tick to do it more often
			// Ingestion/Get/Delete latency
			sem.Add(1)
			go func() {
				defer sem.Done()
				// Set event
				uuid := uuid.New()
				documentID := fmt.Sprintf("search-document-%s", uuid)
				esDoc := &EsDocument{
					Name:     documentID,
					Counter:  1,
					EventTye: "search",
					Team:     "nosql",
					Data:     DATA_ES_DOC,
				}
				durationMilliSec, err := es.indexDocument(es.config.ElasticsearchLatencyIndex, documentID, esDoc)
				if err != nil {
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
					log.Error(err)
				}
				common.ClusterLatencySummary.WithLabelValues(es.clusterName, es.config.ElasticsearchLatencyIndex, "index").Observe(durationMilliSec)
				common.ClusterLatencyHistogram.WithLabelValues(es.clusterName, es.config.ElasticsearchLatencyIndex, "index").Observe(durationMilliSec)

				// Get event
				if err := es.getDocument(es.config.ElasticsearchLatencyIndex, documentID); err != nil {
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
					log.Error(err)
				}

				// Delete event
				if err := es.deleteDocument(es.config.ElasticsearchLatencyIndex, documentID); err != nil {
					common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
					log.Error(err)
				}
			}()
			sem.Wait()
		case <-es.executeNodeProbingTicker.C:
			sem := new(sync.WaitGroup)
			log.Infof("Starting probing ES nodes for cluster %s", es.clusterName)
			for _, node := range es.esNodesList {
				sem.Add(1)
				go func(esNode common.Node) {
					defer sem.Done()
					if err := probeElasticsearchNode(&esNode, es.timeout, es.config.ElasticsearchUser, es.config.ElasticsearchPassword); err != nil {
						common.ElasticNodeAvailabilityGauge.WithLabelValues(esNode.Cluster, esNode.Name).Set(0)
						common.ClusterErrorsCount.WithLabelValues(es.clusterName).Add(1)
						log.Error(err)
					}
				}(node)
			}
			sem.Wait()
		}
	}
}

func probeElasticsearchNode(node *common.Node, timeout time.Duration, username, password string) error {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{
		Timeout: timeout,
	}

	probingURL := fmt.Sprintf("%v://%v:%v/_cat/health?v", node.Scheme, node.Ip, node.Port)
	log.Debug("Start probing ", node.Name)

	start := time.Now()
	req, err := http.NewRequest("GET", probingURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(username, password)
	resp, err := client.Do(req)
	if err != nil {
		log.Debug("Probing failed for ", node.Name, ": ", probingURL, " ", err.Error())
		return err
	}
	durationMilliSec := float64(time.Since(start).Milliseconds())

	log.Debug("Probe result for ", node.Name, ": ", resp.Status)
	if resp.StatusCode != 200 {
		log.Error("Probing failed for ", node.Name, ": ", probingURL, " ", resp.Status)
		return fmt.Errorf("ES Probing failed")
	}

	common.ElasticNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(1)
	common.NodeCatLatencySummary.WithLabelValues(node.Cluster, node.Name).Observe(durationMilliSec)

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

func (es *EsProbe) deleteDocument(index, documentID string) error {
	start := time.Now()
	res, err := es.client.Delete(
		index,
		documentID)
	durationMilliSec := float64(time.Since(start).Milliseconds())

	if err != nil {
		return errors.Wrapf(err, "Failed to delete document %s on %s:%s", documentID, es.clusterName, index)
	}
	defer res.Body.Close()

	if res.IsError() {
		return errors.Errorf("Error delete document %s on %s:%s: %s", documentID, es.clusterName, index, res.String())
	}

	common.ClusterLatencySummary.WithLabelValues(es.clusterName, index, "delete").Observe(durationMilliSec)
	common.ClusterLatencyHistogram.WithLabelValues(es.clusterName, index, "delete").Observe(durationMilliSec)

	return nil
}

func (es *EsProbe) getDocument(index, documentID string) error {
	start := time.Now()
	res, err := es.client.Get(
		index,
		documentID)
	durationMilliSec := float64(time.Since(start).Milliseconds())

	if err != nil {
		return errors.Wrapf(err, "Failed to get document %s on %s:%s", documentID, es.clusterName, index)
	}
	defer res.Body.Close()

	if res.IsError() {
		return errors.Errorf("Error get document %s on %s:%s: %s", documentID, es.clusterName, index, res.String())
	}

	common.ClusterLatencySummary.WithLabelValues(es.clusterName, index, "get").Observe(durationMilliSec)
	common.ClusterLatencyHistogram.WithLabelValues(es.clusterName, index, "get").Observe(durationMilliSec)

	return nil
}

func (es *EsProbe) countNumberOfDurabilityDocs() (float64, float64, error) {
	var r map[string]interface{}
	start := time.Now()
	res, err := es.client.Count(
		es.client.Count.WithIndex(es.config.ElasticsearchDurabilityIndex),
	)
	durationMilliSec := float64(time.Since(start).Milliseconds())

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
	return number_of_current_durability_documents, durationMilliSec, nil
}

func (es *EsProbe) fillDurabilityBucketWithMissingDocs(number_of_current_durability_documents float64) error {
	// TODO improve this stage to be faster (bulk?)
	if int(number_of_current_durability_documents) < es.config.ElasticsearchNumberOfDurabilityDocuments+1 {
		esDoc := &EsDocument{
			EventTye: "durability",
			Team:     "nosql",
			Data:     DATA_ES_DOC,
		}
		for i := int(number_of_current_durability_documents) + 1; i < es.config.ElasticsearchNumberOfDurabilityDocuments+1; i++ {
			// Build the request body.
			esDoc.Name = fmt.Sprintf("document-%d", i)
			esDoc.Counter = i

			if _, err := es.indexDocument(es.config.ElasticsearchDurabilityIndex, strconv.Itoa(i), esDoc); err != nil {
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
	durationMilliSec := float64(time.Since(start).Milliseconds())

	if err != nil {
		return 0, errors.Wrapf(err, "Failed to index document in %s:%s %d", es.clusterName, index)
	}
	defer res.Body.Close()

	if res.IsError() {
		return 0, errors.Errorf("Document index creation failed in %s:%s: %s", es.clusterName, index, res.String())
	}
	return durationMilliSec, nil
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
		es.client.Search.WithIndex(es.config.ElasticsearchDurabilityIndex),
		es.client.Search.WithBody(&buf),
		es.client.Search.WithTrackTotalHits(true),
	)
	durationMilliSec := float64(time.Since(start).Milliseconds())

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

	common.ClusterLatencySummary.WithLabelValues(es.clusterName, es.config.ElasticsearchDurabilityIndex, "search").Observe(durationMilliSec)
	common.ClusterLatencyHistogram.WithLabelValues(es.clusterName, es.config.ElasticsearchDurabilityIndex, "search").Observe(durationMilliSec)

	common.ClusterDurabilitySearchDocumentsHits.WithLabelValues(es.clusterName, es.config.ElasticsearchDurabilityIndex).Set(total)
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
	switch index_status {
	case "green":
		indexStatusCode = 0
	case "yellow":
		indexStatusCode = 1
	default:
		indexStatusCode = 2
	}
	common.IndexProbeStatus.WithLabelValues(es.clusterName, index).Set(indexStatusCode)
	return nil
}
