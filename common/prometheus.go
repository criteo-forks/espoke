// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package common

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	IndexProbeStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_index_probe_status",
			Help: "Indicate index probe status (green is 0, yellow is 1 and red is 2)",
		},
		[]string{"cluster", "index"},
	)

	ClusterDurabilityDocumentsCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_cluster_durability_documents_count",
			Help: "Reports number of documents count in durability index",
		},
		[]string{"cluster"})

	ClusterRestoreDocumentsCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_cluster_restore_documents_count",
			Help: "Reports number of documents count in restore index",
		},
		[]string{"cluster"})

	ClusterDurabilitySearchDocumentsHits = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_cluster_durability_search_documents_hits",
			Help: "Reports number of documents hits from the search on durability index",
		},
		[]string{"cluster", "index"})

	ClusterRestoreCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_cluster_restore_count",
			Help: "Reports number of restore launched",
		},
		[]string{"cluster"})

	ClusterLatencySummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "es_cluster_latency_ms",
			Help:       "Measure latency to do operation",
			MaxAge:     20 * time.Minute, // default value * 2
			AgeBuckets: 20,               // default value * 4
			BufCap:     2000,             // default value * 4
		},
		[]string{"cluster", "index", "operation"},
	)

	ClusterLatencyHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "es_cluster_latency_histogram_ms",
			Help:    "Measure latency to do operation",
			Buckets: []float64{1, 2.5, 5, 7.5, 10, 15, 20, 35, 50, 75, 100, 250, 500, 1000, 5000, 10000},
		},
		[]string{"cluster", "index", "operation"},
	)

	ClusterRestoreErrorsCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "es_cluster_restore_errors_count",
			Help: "Reports errors doing restore with a cluster",
		},
		[]string{"cluster"})

	ClusterErrorsCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "es_cluster_errors_count",
			Help: "Reports Espoke errors doing action with a cluster",
		},
		[]string{"cluster"})

	ErrorsCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "es_probe_errors_count",
		Help: "Reports Espoke internal errors absolute counter since start",
	})

	ElasticNodeAvailabilityGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_node_availability",
			Help: "Reflects elasticsearch node availability : 1 is OK, 0 means node unavailable ",
		},
		[]string{"cluster", "node_name"},
	)

	KibanaNodeAvailabilityGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kibana_node_availability",
			Help: "Reflects kibana node availability : 1 is OK, 0 means node unavailable ",
		},
		[]string{"cluster", "node_name"},
	)

	NodeCatLatencySummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "es_node_cat_latency",
			Help:       "Measure latency to query cat api for every node (quantiles - in ns)",
			MaxAge:     20 * time.Minute, // default value * 2
			AgeBuckets: 20,               // default value * 4
			BufCap:     2000,             // default value * 4
		},
		[]string{"cluster", "node_name"},
	)
)

func StartMetricsEndpoint(metricsPort int) {
	log.Info("Starting Prometheus /metrics endpoint on port ", metricsPort)
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", metricsPort), nil))
	}()
}

// TODO add cluster ones to be cleaned
func CleanNodeMetrics(nodes []Node, allEverKnownNodes []string) {
	for _, nodeSerializedString := range allEverKnownNodes {
		n := strings.SplitN(nodeSerializedString, "|", 2) // [0]: name , [1] cluster

		deleteThisNodeMetrics := true
		for _, node := range nodes {
			if (node.Name == n[0]) && (node.Cluster == n[1]) {
				log.Debug("Metrics are live for node ", n[0], " from cluster ", n[1], " - keeping them")
				deleteThisNodeMetrics = false
				continue
			}
		}
		if deleteThisNodeMetrics {
			log.Info("Metrics removed for vanished node ", n[0], " from cluster ", n[1])
			ElasticNodeAvailabilityGauge.DeleteLabelValues(n[1], n[0])
			NodeCatLatencySummary.DeleteLabelValues(n[1], n[0])
			KibanaNodeAvailabilityGauge.DeleteLabelValues(n[1], n[0])
		}
	}
}

func CleanClusterMetrics(clusterName string, indexes []string) {
	ClusterDurabilityDocumentsCount.DeleteLabelValues(clusterName)
	ClusterErrorsCount.DeleteLabelValues(clusterName)
	ClusterRestoreCount.DeleteLabelValues(clusterName)
	ClusterRestoreErrorsCount.DeleteLabelValues(clusterName)
	ClusterRestoreDocumentsCount.DeleteLabelValues(clusterName)
	for _, index := range indexes {
		IndexProbeStatus.DeleteLabelValues(clusterName, index)
		ClusterDurabilitySearchDocumentsHits.DeleteLabelValues(clusterName, index)
		for _, operation := range []string{"count", "index", "get", "search", "delete"} {
			ClusterLatencySummary.DeleteLabelValues(clusterName, index, operation)
			ClusterLatencyHistogram.DeleteLabelValues(clusterName, index, operation)
		}
	}
}
