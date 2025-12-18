package common

import "time"

type Node struct {
	Name    string
	Ip      string
	Pod     string
	Port    int
	Cluster string
	Scheme  string
}

type Cluster struct {
	Name     string
	Scheme   string
	Endpoint string
	Version  string
}

type Config struct {
	ElasticsearchConsulTag                   string
	ElasticsearchEndpointSuffix              string
	ElasticsearchEndpointPort                int
	ElasticsearchUser                        string
	ElasticsearchPassword                    string
	ElasticsearchDurabilityIndex             string
	ElasticsearchNumberOfDurabilityDocuments int
	ElasticsearchRestore                     bool
	ElasticsearchRestoreSnapshotRepository   string
	ElasticsearchRestoreSnapshotPolicy       string
	KibanaConsulTag                          string
	ConsulApi                                string
	ConsulPeriod                             time.Duration
	ProbePeriod                              time.Duration
	RestorePeriod                            time.Duration
	CleaningPeriod                           time.Duration
	Opensearch                               bool
}
