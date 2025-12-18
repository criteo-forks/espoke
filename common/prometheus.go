// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package common

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	ClusterRestoreDocumentsCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_cluster_restore_documents_count",
			Help: "Reports number of documents count in restore index",
		},
		[]string{"cluster"})

	ClusterRestoreCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_cluster_restore_count",
			Help: "Reports number of restore launched",
		},
		[]string{"cluster"})

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

	KibanaNodeAvailabilityGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kibana_node_availability",
			Help: "Reflects kibana node availability : 1 is OK, 0 means node unavailable ",
		},
		[]string{"cluster", "node_name", "exported_pod"},
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
		n := strings.SplitN(nodeSerializedString, "|", 3) // [0]: name , [1] cluster, [2] pod name

		deleteThisNodeMetrics := true
		for _, node := range nodes {
			if (node.Name == n[0]) && (node.Cluster == n[1]) && (node.Pod == n[2]) {
				log.Debug("Metrics are live for node ", n[0], " and pod ", n[2], " from cluster ", n[1], " - keeping them")
				deleteThisNodeMetrics = false
				continue
			}
		}
		if deleteThisNodeMetrics {
			log.Info("Metrics removed for vanished node ", n[0], " from cluster ", n[1])
			KibanaNodeAvailabilityGauge.DeleteLabelValues(n[1], n[0], n[2])
		}
	}
}

func CleanClusterMetrics(clusterName string, indexes []string) {
	ClusterErrorsCount.DeleteLabelValues(clusterName)
	ClusterRestoreCount.DeleteLabelValues(clusterName)
	ClusterRestoreErrorsCount.DeleteLabelValues(clusterName)
	ClusterRestoreDocumentsCount.DeleteLabelValues(clusterName)
}
