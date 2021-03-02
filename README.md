# espoke - A prometheus blackbox probe for ES clusters

## Infos

* Only works using a opinionated consul model as discovery method
* Expose prometheus metrics from a blank search on every ES indexes of every datanodes

```
Start ES discovering mecanism & probe periodically requests against all ES nodes.
Expose all measures using a prometheus compliant HTTP endpoint.

Flags:
  -h, --help    Show context-sensitive help.

Commands:
  serve
    espoke is a whitebox probing tool for Elasticsearch clusters

Serve Flags:
  -h, --help                    Show context-sensitive help.

  -a, --consul-api="127.0.0.1:8500"
                                127.0.0.1:8500
      --consul-period=120s      nodes discovery update interval
      --probe-period=30s        elasticsearch nodes probing interval for
                                durability and nodes checks
      --restore-period=24h      elasticsearch restore probing interval
      --cleaning-period=600s    prometheus metrics cleaning interval (for
                                vanished nodes)
      --elasticsearch-consul-tag="maintenance-elasticsearch"
                                elasticsearch consul tag
      --elasticsearch-endpoint-suffix=".service.{dc}.foo.bar"
                                Suffix to add after the consul service name to
                                create a valid domain name
      --elasticsearch-user=STRING
                                Elasticsearch username
      --elasticsearch-password=STRING
                                Elasticsearch password
      --elasticsearch-durability-index=".espoke.durability"
                                Elasticsearch durability index
      --elasticsearch-latency-index=".espoke.latency"
                                Elasticsearch latency index
      --elasticsearch-number-of-durability-documents=100000
                                Number of documents to stored in the durability
                                index
      --elasticsearch-restore   Perform Elasticsearch restore test
      --elasticsearch-restore-snapshot-repository="ceph_s3"
                                Name of the Elasticsearch snapshot repository
      --elasticsearch-restore-snapshot-policy="probe-snapshot"
                                Name of the Elasticsearch snapshot policy
      --latency-probe-rate-per-min=120
                                Rate of latency probing per minute (how many
                                checks are done in a minute)
      --kibana-consul-tag="maintenance-kibana"
                                kibana consul tag
  -p, --metrics-port=2112       port where prometheus will expose metrics to
  -l, --log-level="info"        log level      
```

## Metrics

```
# HELP es_cluster_durability_documents_count Reports number of documents count in durability index
# TYPE es_cluster_durability_documents_count gauge
es_cluster_durability_documents_count{cluster="cluster"} 101
# HELP es_cluster_durability_search_documents_hits Reports number of documents hits from the search on durability index
# TYPE es_cluster_durability_search_documents_hits gauge
es_cluster_durability_search_documents_hits{cluster="cluster",index=".espoke.durability"} 71
# HELP es_cluster_latency_histogram_ms Measure latency to do operation
# TYPE es_cluster_latency_histogram_ms histogram
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="1"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="2.5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="7.5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="10"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="15"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="20"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="35"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="50"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="75"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="100"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="250"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="500"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="1000"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="5000"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="10000"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="count",le="+Inf"} 1
es_cluster_latency_histogram_ms_sum{cluster="cluster",index=".espoke.durability",operation="count"} 33
es_cluster_latency_histogram_ms_count{cluster="cluster",index=".espoke.durability",operation="count"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="1"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="2.5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="7.5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="10"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="15"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="20"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="35"} 66
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="50"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="75"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="100"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="250"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="500"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="1000"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="5000"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="10000"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="index",le="+Inf"} 77
es_cluster_latency_histogram_ms_sum{cluster="cluster",index=".espoke.durability",operation="index"} 2390
es_cluster_latency_histogram_ms_count{cluster="cluster",index=".espoke.durability",operation="index"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="1"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="2.5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="7.5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="10"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="15"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="20"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="35"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="50"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="75"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="100"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="250"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="500"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="1000"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="5000"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="10000"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.durability",operation="search",le="+Inf"} 1
es_cluster_latency_histogram_ms_sum{cluster="cluster",index=".espoke.durability",operation="search"} 14
es_cluster_latency_histogram_ms_count{cluster="cluster",index=".espoke.durability",operation="search"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="1"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="2.5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="7.5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="10"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="15"} 1
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="20"} 2
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="35"} 74
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="50"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="75"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="100"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="250"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="500"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="1000"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="5000"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="10000"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="delete",le="+Inf"} 77
es_cluster_latency_histogram_ms_sum{cluster="cluster",index=".espoke.latency",operation="delete"} 2190
es_cluster_latency_histogram_ms_count{cluster="cluster",index=".espoke.latency",operation="delete"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="1"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="2.5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="7.5"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="10"} 0
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="15"} 23
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="20"} 63
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="35"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="50"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="75"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="100"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="250"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="500"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="1000"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="5000"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="10000"} 77
es_cluster_latency_histogram_ms_bucket{cluster="cluster",index=".espoke.latency",operation="get",le="+Inf"} 77
es_cluster_latency_histogram_ms_sum{cluster="cluster",index=".espoke.latency",operation="get"} 1380
es_cluster_latency_histogram_ms_count{cluster="cluster",index=".espoke.latency",operation="get"} 77
# HELP es_cluster_latency_ms Measure latency to do operation
# TYPE es_cluster_latency_ms summary
es_cluster_latency_ms_sum{cluster="cluster",index=".espoke.durability",operation="count"} 33
es_cluster_latency_ms_count{cluster="cluster",index=".espoke.durability",operation="count"} 1
# HELP es_index_probe_status Indicate index probe status (green is 0, yellow is 1 and red is 2)
# TYPE es_index_probe_status gauge
es_index_probe_status{cluster="cluster",index=".espoke.durability"} 0
es_index_probe_status{cluster="cluster",index=".espoke.latency"} 0
# HELP es_node_availability Reflects elasticsearch node availability : 1 is OK, 0 means node unavailable 
# TYPE es_node_availability gauge
es_node_availability{cluster="cluster",node_name="node_name"} 1
# HELP es_cluster_restore_count Reports number of restore launched
# TYPE es_cluster_restore_count gauge
es_cluster_restore_count{cluster="cluster"} 2
# HELP es_cluster_restore_documents_count Reports number of documents count in restore index
# TYPE es_cluster_restore_documents_count gauge
es_cluster_restore_documents_count{cluster="cluster"} 100000
# HELP es_node_cat_latency Measure latency to query cat api for every node (quantiles - in ns)
# TYPE es_node_cat_latency summary
es_node_cat_latency_sum{cluster="cluster",node_name="node_name"} 25
es_node_cat_latency_count{cluster="cluster",node_name="node_name"} 1
# HELP kibana_node_availability Reflects kibana node availability : 1 is OK, 0 means node unavailable 
# TYPE kibana_node_availability gauge
kibana_node_availability{cluster="cluster",node_name="node_name"} 1
```
