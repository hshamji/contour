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
	ratelimit_config_v3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	admission_control_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/admission_control/v3"
	envoy_config_filter_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	ratelimit_filter_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	http "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	envoy_types_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/types"
)

// LocalRateLimitConfig returns a config for the HTTP local rate
// limit filter.
func LocalRateLimitConfig(config *dag.LocalRateLimitPolicy, statPrefix string) *any.Any {
	if config == nil {
		return nil
	}

	c := &envoy_config_filter_http_local_ratelimit_v3.LocalRateLimit{
		StatPrefix: statPrefix,
		TokenBucket: &envoy_type_v3.TokenBucket{
			MaxTokens:     config.MaxTokens,
			TokensPerFill: protobuf.UInt32(config.TokensPerFill),
			FillInterval:  protobuf.Duration(config.FillInterval),
		},
		ResponseHeadersToAdd: headerValueList(config.ResponseHeadersToAdd, false),
		FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
		FilterEnforced: &envoy_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
	}

	// Envoy defaults to 429 (Too Many Requests) if this is not specified.
	if config.ResponseStatusCode > 0 {
		c.Status = &envoy_type_v3.HttpStatus{Code: envoy_type_v3.StatusCode(config.ResponseStatusCode)}
	}

	return protobuf.MustMarshalAny(c)
}

// GlobalRateLimits converts DAG RateLimitDescriptors to Envoy RateLimits.
func GlobalRateLimits(descriptors []*dag.RateLimitDescriptor) []*envoy_route_v3.RateLimit {
	var rateLimits []*envoy_route_v3.RateLimit
	for _, descriptor := range descriptors {
		var rl envoy_route_v3.RateLimit

		for _, entry := range descriptor.Entries {
			switch {
			case entry.GenericKey != nil:
				rl.Actions = append(rl.Actions, &envoy_route_v3.RateLimit_Action{
					ActionSpecifier: &envoy_route_v3.RateLimit_Action_GenericKey_{
						GenericKey: &envoy_route_v3.RateLimit_Action_GenericKey{
							DescriptorKey:   entry.GenericKey.Key,
							DescriptorValue: entry.GenericKey.Value,
						},
					},
				})
			case entry.HeaderMatch != nil:
				rl.Actions = append(rl.Actions, &envoy_route_v3.RateLimit_Action{
					ActionSpecifier: &envoy_route_v3.RateLimit_Action_RequestHeaders_{
						RequestHeaders: &envoy_route_v3.RateLimit_Action_RequestHeaders{
							HeaderName:    entry.HeaderMatch.HeaderName,
							DescriptorKey: entry.HeaderMatch.Key,
						},
					},
				})
			case entry.HeaderValueMatch != nil:
				rl.Actions = append(rl.Actions, &envoy_route_v3.RateLimit_Action{
					ActionSpecifier: &envoy_route_v3.RateLimit_Action_HeaderValueMatch_{
						HeaderValueMatch: &envoy_route_v3.RateLimit_Action_HeaderValueMatch{
							DescriptorValue: entry.HeaderValueMatch.Value,
							ExpectMatch:     wrapperspb.Bool(entry.HeaderValueMatch.ExpectMatch),
							Headers:         headerMatcher(entry.HeaderValueMatch.Headers),
						},
					},
				})
			case entry.RemoteAddress != nil:
				rl.Actions = append(rl.Actions, &envoy_route_v3.RateLimit_Action{
					ActionSpecifier: &envoy_route_v3.RateLimit_Action_RemoteAddress_{
						RemoteAddress: &envoy_route_v3.RateLimit_Action_RemoteAddress{},
					},
				})
			}
		}

		rateLimits = append(rateLimits, &rl)
	}

	return rateLimits
}

// GlobalRateLimitConfig stores configuration for
// an HTTP global rate limiting filter.
type GlobalRateLimitConfig struct {
	ExtensionService        types.NamespacedName
	FailOpen                bool
	Timeout                 timeout.Setting
	Domain                  string
	EnableXRateLimitHeaders bool
}

// GlobalRateLimitFilter returns a configured HTTP global rate limit filter,
// or nil if config is nil.
func GlobalRateLimitFilter(config *GlobalRateLimitConfig) *http.HttpFilter {
	if config == nil {
		return nil
	}

	return &http.HttpFilter{
		Name: wellknown.HTTPRateLimit,
		ConfigType: &http.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&ratelimit_filter_v3.RateLimit{
				Domain:          config.Domain,
				Timeout:         envoy.Timeout(config.Timeout),
				FailureModeDeny: !config.FailOpen,
				RateLimitService: &ratelimit_config_v3.RateLimitServiceConfig{
					GrpcService: &envoy_core_v3.GrpcService{
						TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
								ClusterName: dag.ExtensionClusterName(config.ExtensionService),
							},
						},
					},
					TransportApiVersion: envoy_core_v3.ApiVersion_V3,
				},
				EnableXRatelimitHeaders: enableXRateLimitHeaders(config.EnableXRateLimitHeaders),
			}),
		},
	}
}

func enableXRateLimitHeaders(enable bool) ratelimit_filter_v3.RateLimit_XRateLimitHeadersRFCVersion {
	if enable {
		return ratelimit_filter_v3.RateLimit_DRAFT_VERSION_03
	}
	return ratelimit_filter_v3.RateLimit_OFF
}

func AdmissionControlFilter(config *dag.RateLimitPolicy) *http.HttpFilter {
	if config == nil {
		return nil
	}
	if config.Admission == nil {
		return nil
	}

	return &http.HttpFilter{
		//envoy.filters.http.ratelimit
		//Name: wellknown.HTTPRateLimit,
		Name: "envoy.filters.http.admissionControl",
		ConfigType: &http.HttpFilter_TypedConfig{
			TypedConfig: protobuf.MustMarshalAny(&admission_control_v3.AdmissionControl{
				Enabled: &envoy_core_v3.RuntimeFeatureFlag{
					DefaultValue: &wrapperspb.BoolValue{Value: true},
					RuntimeKey:   "admission_control.enabled",
				},
				EvaluationCriteria: &admission_control_v3.AdmissionControl_SuccessCriteria_{
					SuccessCriteria: &admission_control_v3.AdmissionControl_SuccessCriteria{
						HttpCriteria: &admission_control_v3.AdmissionControl_SuccessCriteria_HttpCriteria{
							HttpSuccessStatus: []*envoy_types_v3.Int32Range{
								&envoy_types_v3.Int32Range{
									Start: 100,
									End:   400,
								},
								&envoy_types_v3.Int32Range{
									Start: 404,
									End:   404,
								},
							},
						},
						GrpcCriteria: &admission_control_v3.AdmissionControl_SuccessCriteria_GrpcCriteria{
							GrpcSuccessStatus: []uint32{0}},
					},
				},
				SamplingWindow: &durationpb.Duration{
					Seconds: int64(config.Admission.SamplingWindow),
					Nanos:   0,
				},
				Aggression: &envoy_core_v3.RuntimeDouble{
					DefaultValue: float64(config.Admission.Aggression),
					RuntimeKey:   "admission_control.aggression",
				},
				SrThreshold: &envoy_core_v3.RuntimePercent{
					DefaultValue: &envoy_types_v3.Percent{Value: float64(config.Admission.SuccessThreshold)},
					RuntimeKey:   "admission_control.sr_threshol",
				},
				RpsThreshold: &envoy_core_v3.RuntimeUInt32{
					DefaultValue: 95,
					RuntimeKey:   "admission_control.rps_threshold",
				},
				MaxRejectionProbability: &envoy_core_v3.RuntimePercent{
					DefaultValue: &envoy_types_v3.Percent{Value: 99.0},
					RuntimeKey:   "admission_control.max_rejection_probability",
				},
			}),
		},
	}
}
