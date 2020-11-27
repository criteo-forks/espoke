package common

type Node struct {
	Name    string
	Ip      string
	Port    int
	Cluster string
	Scheme  string
}

type Cluster struct {
	Name    string
	Scheme  string
}