// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package cmd

import (
	"github.com/criteo-forks/espoke/common"
	"github.com/criteo-forks/espoke/watcher"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
)

type ServeCmd struct {
	ConsulApi                   string        `default:"127.0.0.1:8500" help:"127.0.0.1:8500" "consul target api host:port" yaml:"consul_api" short:"a"`
	ConsulPeriod                time.Duration `default:"120s" help:"nodes discovery update interval" yaml:"consul_period"`
	ProbePeriod                 time.Duration `default:"30s" help:"elasticsearch nodes probing interval" yaml:"probe_period"`
	CleaningPeriod              time.Duration `default:"600s" help:"prometheus metrics cleaning interval (for vanished nodes)" yaml:"cleaning_period"`
	ElasticsearchConsulTag      string        `default:"maintenance-elasticsearch" help:"elasticsearch consul tag" yaml:"elasticsearch_consul_service"`
	ElasticsearchEndpointSuffix string        `default:".service.{dc}.foo.bar" help:"Suffix to add after the consul service name to create a valid domain name" yaml:"elasticsearch_endpoint_suffix"`
	ElasticsearchUser           string        `help:"Elasticsearch username" yaml:"elasticsearch_user"`
	ElasticsearchPassword       string        `help:"Elasticsearch password" yaml:"elasticsearch_password"`
	KibanaConsulTag             string        `default:"kibana" help:"maintenance-kibana consul tag" yaml:"kibana_consul_service"`
	MetricsPort                 int           `default:"2112" help:"port where prometheus will expose metrics to" yaml:"metrics_port" short:"p"`
	LogLevel                    string        `default:"info" help:"log level" yaml:"log_level" short:"l"`
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
	common.StartMetricsEndpoint(r.MetricsPort)

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

	config := &common.Config{
		ElasticsearchConsulTag:      r.ElasticsearchConsulTag,
		ElasticsearchEndpointSuffix: r.ElasticsearchEndpointSuffix,
		ElasticsearchUser:           r.ElasticsearchUser,
		ElasticsearchPassword:       r.ElasticsearchPassword,
		KibanaConsulTag:             r.KibanaConsulTag,
		ConsulApi:                   r.ConsulApi,
		ConsulPeriod:                r.ConsulPeriod,
		ProbePeriod:                 r.ProbePeriod,
		CleaningPeriod:              r.CleaningPeriod,
	}

	w, err:= watcher.NewWatcher(config)
	if err != nil {
		return err
	}
	return w.WatchPools()
}
