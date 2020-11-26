// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package prometheus

import (
	"fmt"
	"github.com/criteo-forks/espoke/common"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	ErrorsCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "es_probe_errors_count",
		Help: "Reports Espoke internal errors absolute counter since start",
	})

	NodeCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "es_node_count",
		Help: "Reports current discovered nodes amount",
	})

	ElasticNodeAvailabilityGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_node_availability",
			Help: "Reflects node availabity : 1 is OK, 0 means node unavailable ",
		},
		[]string{"cluster", "nodename"},
	)

	KibanaNodeAvailabilityGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kibana_node_availability",
			Help: "Reflects node availabity : 1 is OK, 0 means node unavailable ",
		},
		[]string{"cluster", "nodename"},
	)

	NodeCatLatencySummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "es_node_cat_latency",
			Help:       "Measure latency to query cat api for every node (quantiles - in ns)",
			MaxAge:     20 * time.Minute, // default value * 2
			AgeBuckets: 20,               // default value * 4
			BufCap:     2000,             // default value * 4
		},
		[]string{"cluster", "nodename"},
	)

	ConsulDiscoveryDurationSummary = promauto.NewSummary(prometheus.SummaryOpts{
		Name:       "es_probe_consul_discovery_duration",
		Help:       "Time spent for discovering nodes using Consul API (in ns)",
		MaxAge:     20 * time.Minute, // default value * 2
		AgeBuckets: 20,               // default value * 4
		BufCap:     2000,             // default value * 4
	})

	CleaningMetricsDurationSummary = promauto.NewSummary(prometheus.SummaryOpts{
		Name:       "es_probe_metrics_cleaning_duration",
		Help:       "Time spent for cleaning vanished nodes metrics (in ns)",
		MaxAge:     120 * time.Minute, // default value * 6
		AgeBuckets: 20,                // default value * 4
		BufCap:     2000,              // default value * 4
	})
)

func StartMetricsEndpoint(metricsPort int) {
	log.Info("Starting Prometheus /metrics endpoint on port ", metricsPort)
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", metricsPort), nil))
	}()
}

func CleanMetrics(nodes []common.Esnode, allEverKnownNodes []string) error {
	start := time.Now()

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

	durationNanosec := float64(time.Since(start).Nanoseconds())
	CleaningMetricsDurationSummary.Observe(durationNanosec)
	return nil
}
