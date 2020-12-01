package common

import "time"

type Node struct {
	Name    string
	Ip      string
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
	ElasticsearchConsulTag      string
	ElasticsearchEndpointSuffix string
	ElasticsearchUser           string
	ElasticsearchPassword       string
	KibanaConsulTag             string
	ConsulApi                   string
	ConsulPeriod                time.Duration
	ProbePeriod                 time.Duration
	CleaningPeriod              time.Duration
}
