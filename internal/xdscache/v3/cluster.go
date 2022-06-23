// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v3

import (
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"sort"
	"sync"

	envoy_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// ClusterCache manages the contents of the gRPC CDS cache.
type ClusterCache struct {
	mu     sync.Mutex
	values map[string]*envoy_cluster_v3.Cluster
	contour.Cond
}

// Update replaces the contents of the cache with the supplied map.
func (c *ClusterCache) Update(v map[string]*envoy_cluster_v3.Cluster) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.Cond.Notify()
}

// Contents returns a copy of the cache's contents.
func (c *ClusterCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_cluster_v3.Cluster
	for _, v := range c.values {
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (c *ClusterCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_cluster_v3.Cluster
	for _, n := range names {
		// if the cluster is not registered we cannot return
		// a blank cluster because each cluster has a required
		// discovery type; DNS, EDS, etc. We cannot determine the
		// correct value for this property from the cluster's name
		// provided by the query so we must not return a blank cluster.
		if v, ok := c.values[n]; ok {
			values = append(values, v)
		}
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (*ClusterCache) TypeURL() string { return resource.ClusterType }

func (c *ClusterCache) OnChange(root *dag.DAG) {
	clusters := map[string]*envoy_cluster_v3.Cluster{}

	for _, cluster := range root.GetClusters() {
		name := envoy.Clustername(cluster)
		if _, ok := clusters[name]; !ok {
			clusters[name] = envoy_v3.Cluster(cluster)
		}
	}

	for name, ec := range root.GetExtensionClusters() {
		if _, ok := clusters[name]; !ok {
			clusters[name] = envoy_v3.ExtensionCluster(ec)
		}
	}

	//Add the cluster for zipkin here:
	jaegerCluster := envoy_v3.ClusterDefaults()
	jaegerCluster.Name = "jaeger"
	jaegerCluster.LbPolicy = envoy_cluster_v3.Cluster_ROUND_ROBIN

	jaegerCluster.ClusterDiscoveryType = envoy_v3.ClusterDiscoveryType(envoy_cluster_v3.Cluster_STRICT_DNS)
	//jaegerCluster.LoadAssignment = envoy_v3.StaticClusterLoadAssignment(&dag.Service{
	//	Weighted:           dag.WeightedService{
	//		Weight:           100,
	//		ServiceName:      "jaeger",
	//		ServiceNamespace: "otel-collector",
	//		ServicePort:      v1.ServicePort{
	//			//Name:        "",
	//			//Protocol:    "",
	//			//AppProtocol: nil,
	//			Port:        9411,
	//			//TargetPort:  intstr.IntOrString{},
	//			//NodePort:    0,
	//		},
	//	},
	//	Protocol:           "",
	//	MaxConnections:     0,
	//	MaxPendingRequests: 0,
	//	MaxRequests:        0,
	//	MaxRetries:         0,
	//	ExternalName:       "gateway-collector.otel-collector.svc",
	//})

	jaegerEndpoint := make([]*envoy_endpoint_v3.LbEndpoint, 0, 1)

	jaegerEndpoint = append(jaegerEndpoint,
		&envoy_endpoint_v3.LbEndpoint{
			HostIdentifier: &envoy_endpoint_v3.LbEndpoint_Endpoint{
				Endpoint: &envoy_endpoint_v3.Endpoint{
					Address: &envoy_core_v3.Address{
						Address: &envoy_core_v3.Address_SocketAddress{
							SocketAddress: &envoy_core_v3.SocketAddress{
								Protocol:   envoy_core_v3.SocketAddress_TCP,
								Address:    "gateway-collector.otel-collector.svc",
								Ipv4Compat: true,
								PortSpecifier: &envoy_core_v3.SocketAddress_PortValue{
									PortValue: uint32(9411),
								},
							},
						},
					},
				},
			},
		},
	)

	jaegerCluster.LoadAssignment = &envoy_endpoint_v3.ClusterLoadAssignment{
		ClusterName:    "jaeger",
		Endpoints:      []*envoy_endpoint_v3.LocalityLbEndpoints{{
			LbEndpoints: jaegerEndpoint,
		}},
		//NamedEndpoints: nil,
		//Policy:         nil,
	}

	//cluster.EdsClusterConfig = &envoy_cluster_v3.Cluster_EdsClusterConfig{
	//	EdsConfig:   ConfigSource("contour"),
	//	ServiceName: ext.Upstream.ClusterName,
	//}

	clusters["jaeger"] = jaegerCluster

	c.Update(clusters)
}
