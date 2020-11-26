// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package probe

import (
	"fmt"
	"github.com/criteo-forks/espoke/common"
	"github.com/criteo-forks/espoke/prometheus"
	"io/ioutil"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/valyala/fastjson"
)

func ProbeElasticsearchNode(node *common.Esnode, updateProbingPeriod time.Duration) error {
	client := &http.Client{
		Timeout: updateProbingPeriod - 2*time.Second,
	}

	probingURL := fmt.Sprintf("%v://%v:%v/_cat/indices?v", node.Scheme, node.Ip, node.Port)
	log.Debug("Start probing ", node.Name)

	start := time.Now()
	resp, err := client.Get(probingURL)
	if err != nil {
		log.Debug("Probing failed for ", node.Name, ": ", probingURL, " ", err.Error())
		log.Error(err)
		prometheus.ElasticNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		prometheus.ErrorsCount.Inc()
		return err
	}
	durationNanosec := float64(time.Since(start).Nanoseconds())

	log.Debug("Probe result for ", node.Name, ": ", resp.Status)
	if resp.StatusCode != 200 {
		log.Error("Probing failed for ", node.Name, ": ", probingURL, " ", resp.Status)
		prometheus.ElasticNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		prometheus.ErrorsCount.Inc()
		return fmt.Errorf("ES Probing failed")
	}

	prometheus.ElasticNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(1)
	prometheus.NodeCatLatencySummary.WithLabelValues(node.Cluster, node.Name).Observe(durationNanosec)

	return nil
}

func ProbeKibanaNode(node *common.Esnode, updateProbingPeriod time.Duration) error {
	client := &http.Client{
		Timeout: updateProbingPeriod - 2*time.Second,
	}

	probingURL := fmt.Sprintf("http://%v:%v/api/status", node.Ip, node.Port)
	log.Debug("Start probing ", node.Name)

	resp, err := client.Get(probingURL)
	if err != nil {
		log.Debug("Probing failed for ", node.Name, ": ", probingURL, " ", err.Error())
		log.Error(err)
		prometheus.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		prometheus.ErrorsCount.Inc()
		return err
	}

	log.Debug("Probe result for ", node.Name, ": ", resp.Status)
	if resp.StatusCode != 200 {
		log.Error("Probing failed for ", node.Name, ": ", probingURL, " ", resp.Status)
		prometheus.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		prometheus.ErrorsCount.Inc()
		return fmt.Errorf("kibana Probing failed")
	}

	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		prometheus.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		return fmt.Errorf("kibana Probing failed: %s", readErr)
	}

	var p fastjson.Parser
	json, jsonErr := p.Parse(string(body))
	if jsonErr != nil {
		prometheus.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		return fmt.Errorf("kibana Probing failed: %s", jsonErr)
	}
	nodeState := string(json.GetStringBytes("status", "overall", "state"))
	if nodeState != "green" {
		prometheus.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(0)
		return fmt.Errorf("kibana Probing failed: node not in a green/healthy state")
	}

	prometheus.KibanaNodeAvailabilityGauge.WithLabelValues(node.Cluster, node.Name).Set(1)

	return nil
}
