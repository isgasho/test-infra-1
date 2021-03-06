// Copyright 2020 gRPC authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package orch

import (
	"strings"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/golang/protobuf/jsonpb"
	"github.com/grpc/test-infra/benchmarks/svc/types"
)

const driverPort int32 = 10000
const serverPort int32 = 10010

// specBuilder creates valid Kubernetes specs for components.
type specBuilder struct {
	session   *types.Session
	component *types.Component
}

// newSpecBuilder creates a SpecBuilder instance for a specific
// session and component.
func newSpecBuilder(s *types.Session, c *types.Component) *specBuilder {
	return &specBuilder{s, c}
}

// Containers lists the containers available in each pod the
// deployment manages. Since sidecars are not used for load testing,
// there will only be one container in the returned slice.
func (sb *specBuilder) Containers() []apiv1.Container {
	return []apiv1.Container{
		{
			Name:  sb.component.Name,
			Image: sb.component.ContainerImage,
			Ports: sb.ContainerPorts(),
			Env:   sb.Env(),
		},
	}
}

// ContainerPorts specifies the ingress ports (TCP) on the pods. For
// the driver and client, this is only the DriverPort. For the server,
// the ServerPort is included.
func (sb *specBuilder) ContainerPorts() []apiv1.ContainerPort {
	var ports []apiv1.ContainerPort

	ports = append(ports, apiv1.ContainerPort{
		Name:          "driver-port",
		Protocol:      apiv1.ProtocolTCP,
		ContainerPort: driverPort,
	})

	if sb.component.Kind == types.ServerComponent {
		ports = append(ports, apiv1.ContainerPort{
			Name:          "server-port",
			Protocol:      apiv1.ProtocolTCP,
			ContainerPort: serverPort,
		})
	}

	return ports
}

// ObjectMeta returns the metadata that should be set on all resources
// created for a specific component. Most importantly, it includes the
// component's name and necessary labels.
func (sb *specBuilder) ObjectMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:   sb.component.Name,
		Labels: sb.Labels(),
	}
}

// Labels returns a map of labels that are added to the resource and
// controller specs. The following labels accompany all SpecBuilder
// generated resources:
//
//   1. `autogen` set to a value of 1, signifying these resources are
//       automatically generated.
//   2. `session-name` set to the internal name of the current session.
//   3. `component-name` set to the internal name of the component.
//   4. `component-kind` set to the kind of component (e.g. "driver").
func (sb *specBuilder) Labels() map[string]string {
	return map[string]string{
		"autogen":        "1",
		"session-name":   sb.session.Name,
		"component-name": sb.component.Name,
		"component-kind": strings.ToLower(sb.component.Kind.String()),
	}
}

// Pod returns a kubernetes pod object which meets the requirements of
// the component.
func (sb *specBuilder) Pod() *apiv1.Pod {
	return &apiv1.Pod{
		ObjectMeta: sb.ObjectMeta(),
		Spec: apiv1.PodSpec{
			Affinity:   sb.Affinity(),
			Containers: sb.Containers(),
			NodeSelector: map[string]string{
				"pool": sb.component.PoolName,
			},
			RestartPolicy: "Never",
		},
	}
}

// Env returns the environment variables that should be set based on
// the type of component.
func (sb *specBuilder) Env() []apiv1.EnvVar {
	var vars []apiv1.EnvVar

	for k, v := range sb.component.Env {
		vars = append(vars, apiv1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	switch sb.component.Kind {
	case types.DriverComponent:
		vars = append(vars, apiv1.EnvVar{
			Name:  "SCENARIO_JSON",
			Value: sb.scenarioJSON(),
		})
	case types.ServerComponent:
		vars = append(vars, apiv1.EnvVar{
			Name:  "WORKER_KIND",
			Value: "server",
		})
	case types.ClientComponent:
		vars = append(vars, apiv1.EnvVar{
			Name:  "WORKER_KIND",
			Value: "client",
		})
	}

	return vars
}

// Affinity returns an affinity object that repels autogenerated pods
// from being scheduled on the same node.
func (sb *specBuilder) Affinity() *apiv1.Affinity {
	return &apiv1.Affinity{
		PodAntiAffinity: &apiv1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []apiv1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "autogen",
								Operator: metav1.LabelSelectorOpExists,
							},
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}
}

func (sb *specBuilder) scenarioJSON() string {
	marshaler := &jsonpb.Marshaler{
		Indent:      "",
		EnumsAsInts: true,
		OrigName:    true,
	}

	json, err := marshaler.MarshalToString(sb.session.Scenario)
	if err != nil {
		return ""
	}

	return json
}
