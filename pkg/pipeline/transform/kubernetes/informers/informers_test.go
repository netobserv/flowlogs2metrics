/*
 * Copyright (C) 2022 IBM, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package informers

import (
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/netobserv/flowlogs-pipeline/pkg/operational"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/transform/kubernetes/cni"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetInfo(t *testing.T) {
	metrics := operational.NewMetrics(&config.MetricsSettings{})
	kubeData := Informers{indexerHitMetric: metrics.CreateIndexerHitCounter()}
	pidx, hidx, sidx, ridx := SetupIndexerMocks(&kubeData)
	pidx.MockPod("1.2.3.4", "AA:BB:CC:DD:EE:FF", "eth0", "pod1", "podNamespace", "10.0.0.1", nil)
	pidx.MockPod("1.2.3.5", "", "", "pod2", "podNamespace", "10.0.0.1", &Owner{Name: "rs1", Type: "ReplicaSet"})
	pidx.FallbackNotFound()
	ridx.MockReplicaSet("rs1", "podNamespace", Owner{Name: "dep1", Type: "Deployment"})
	ridx.FallbackNotFound()
	sidx.MockService("1.2.3.100", "svc1", "svcNamespace")
	sidx.FallbackNotFound()
	hidx.MockNode("10.0.0.1", "node1")
	hidx.FallbackNotFound()

	// Test get orphan pod
	info, err := kubeData.GetInfo(nil, "1.2.3.4", "")
	require.NoError(t, err)
	pod1 := Info{
		Type: "Pod",
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: "podNamespace",
		},
		HostName:         "node1",
		HostIP:           "10.0.0.1",
		Owner:            Owner{Name: "pod1", Type: "Pod"},
		NetworkName:      "primary",
		ips:              []string{"1.2.3.4"},
		secondaryNetKeys: []string{"~~AA:BB:CC:DD:EE:FF"},
	}
	require.Equal(t, pod1, *info)

	// Test get same pod by mac
	info, err = kubeData.GetInfo([]cni.SecondaryNetKey{{NetworkName: "custom-network", Key: "~~AA:BB:CC:DD:EE:FF"}}, "", "")
	require.NoError(t, err)
	pod1.NetworkName = "custom-network"
	require.Equal(t, pod1, *info)

	// Test get pod owned
	info, err = kubeData.GetInfo(nil, "1.2.3.5", "")
	require.NoError(t, err)

	require.Equal(t, Info{
		Type: "Pod",
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod2",
			Namespace: "podNamespace",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "ReplicaSet",
				Name: "rs1",
			}},
		},
		HostName:         "node1",
		HostIP:           "10.0.0.1",
		Owner:            Owner{Name: "dep1", Type: "Deployment"},
		NetworkName:      "primary",
		ips:              []string{"1.2.3.5"},
		secondaryNetKeys: []string{},
	}, *info)

	// Test get node
	info, err = kubeData.GetInfo(nil, "10.0.0.1", "")
	require.NoError(t, err)

	require.Equal(t, Info{
		Type: "Node",
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
		},
		Owner:       Owner{Name: "node1", Type: "Node"},
		NetworkName: "primary",
		ips:         []string{"10.0.0.1"},
	}, *info)

	// Test get service
	info, err = kubeData.GetInfo(nil, "1.2.3.100", "")
	require.NoError(t, err)

	require.Equal(t, Info{
		Type: "Service",
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc1",
			Namespace: "svcNamespace",
		},
		Owner:       Owner{Name: "svc1", Type: "Service"},
		NetworkName: "primary",
		ips:         []string{"1.2.3.100"},
	}, *info)

	// Test no match
	info, err = kubeData.GetInfo(nil, "1.2.3.200", "")
	require.NotNil(t, err)
	require.Nil(t, info)
}
