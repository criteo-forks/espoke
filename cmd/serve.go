// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package cmd

import (
	"os"
	"time"

	"github.com/criteo-forks/espoke/common"
	"github.com/criteo-forks/espoke/watcher"

	log "github.com/sirupsen/logrus"
)

type ServeCmd struct {
	ConsulApi                                string        `default:"127.0.0.1:8500" help:"127.0.0.1:8500" help:"consul target api host:port" short:"a"`
	ConsulPeriod                             time.Duration `default:"120s" help:"nodes discovery update interval"`
	ProbePeriod                              time.Duration `default:"30s" help:"elasticsearch nodes probing interval for durability and nodes checks"`
	RestorePeriod                            time.Duration `default:"24h" help:"elasticsearch restore probing interval"`
	CleaningPeriod                           time.Duration `default:"600s" help:"prometheus metrics cleaning interval (for vanished nodes)"`
	ElasticsearchConsulTag                   string        `default:"maintenance-elasticsearch" help:"elasticsearch consul tag"`
	ElasticsearchEndpointSuffix              string        `default:".service.{dc}.foo.bar" help:"Suffix to add after the consul service name to create a valid domain name"`
	ElasticsearchEndpointPort                int           `default:"0" help:"Elasticsearch port used for cluster level calls"`
	ElasticsearchUser                        string        `help:"Elasticsearch username"`
	ElasticsearchPassword                    string        `help:"Elasticsearch password"`
	ElasticsearchDurabilityIndex             string        `default:".espoke.durability" help:"Elasticsearch durability index"`
	ElasticsearchLatencyIndex                string        `default:".espoke.latency" help:"Elasticsearch latency index"`
	ElasticsearchNumberOfDurabilityDocuments int           `default:"100000" help:"Number of documents to stored in the durability index"`
	ElasticsearchRestore                     bool          `default:"false" help:"Perform Elasticsearch restore test"`
	ElasticsearchRestoreSnapshotRepository   string        `default:"ceph_s3" help:"Name of the Elasticsearch snapshot repository"`
	ElasticsearchRestoreSnapshotPolicy       string        `default:"probe-snapshot-sm" help:"Name of the Elasticsearch snapshot policy"`
	LatencyProbeRatePerMin                   int           `default:"120" help:"Rate of latency probing per minute (how many checks are done in a minute)"`
	KibanaConsulTag                          string        `default:"maintenance-kibana" help:"kibana consul tag"`
	MetricsPort                              int           `default:"2112" help:"port where prometheus will expose metrics to" short:"p"`
	LogLevel                                 string        `default:"info" help:"log level" yaml:"log_level" short:"l"`
	Opensearch                               bool          `default:"true" help:"Probe monitors opensearch clusters"`
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

	if r.ElasticsearchRestore {
		log.Info("Restore interval: ", r.RestorePeriod.String())
	}

	if r.Opensearch {
		log.Info("Opensearch: yes")
	}

	config := &common.Config{
		ElasticsearchConsulTag:                   r.ElasticsearchConsulTag,
		ElasticsearchEndpointSuffix:              r.ElasticsearchEndpointSuffix,
		ElasticsearchEndpointPort:                r.ElasticsearchEndpointPort,
		ElasticsearchUser:                        r.ElasticsearchUser,
		ElasticsearchPassword:                    r.ElasticsearchPassword,
		ElasticsearchDurabilityIndex:             r.ElasticsearchDurabilityIndex,
		ElasticsearchLatencyIndex:                r.ElasticsearchLatencyIndex,
		ElasticsearchNumberOfDurabilityDocuments: r.ElasticsearchNumberOfDurabilityDocuments,
		ElasticsearchRestore:                     r.ElasticsearchRestore,
		ElasticsearchRestoreSnapshotRepository:   r.ElasticsearchRestoreSnapshotRepository,
		ElasticsearchRestoreSnapshotPolicy:       r.ElasticsearchRestoreSnapshotPolicy,
		LatencyProbeRatePerMin:                   r.LatencyProbeRatePerMin,
		KibanaConsulTag:                          r.KibanaConsulTag,
		ConsulApi:                                r.ConsulApi,
		ConsulPeriod:                             r.ConsulPeriod,
		ProbePeriod:                              r.ProbePeriod,
		RestorePeriod:                            r.RestorePeriod,
		CleaningPeriod:                           r.CleaningPeriod,
		Opensearch:                               r.Opensearch,
	}

	w, err := watcher.NewWatcher(config)
	if err != nil {
		return err
	}
	return w.WatchPools()
}
