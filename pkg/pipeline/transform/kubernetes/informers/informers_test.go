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

	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/transform/kubernetes/model"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetInfo(t *testing.T) {
	kubeData := Informers{}
	pidx, hidx, sidx, ridx := SetupIndexerMocks(&kubeData)
	ridx.MockReplicaSet("rs1", "podNamespace", "dep1", "Deployment")
	ridx.FallbackNotFound()
	pidx.MockPod("1.2.3.4", "pod1", "podNamespace", "10.0.0.1", "pod1", "Pod")
	pidx.MockPod("1.2.3.5", "pod2", "podNamespace", "10.0.0.1", "rs1", "ReplicaSet")
	pidx.FallbackNotFound()
	sidx.MockService("1.2.3.100", "svc1", "svcNamespace")
	sidx.FallbackNotFound()
	hidx.MockNode("10.0.0.1", "node1")
	hidx.FallbackNotFound()

	// Test get orphan pod
	info := kubeData.GetByIP("1.2.3.4")
	require.NotNil(t, info)

	require.Equal(t, model.ResourceMetaData{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod1",
			Namespace: "podNamespace",
		},
		Kind:      "Pod",
		HostName:  "node1",
		HostIP:    "10.0.0.1",
		OwnerName: "pod1",
		OwnerKind: "Pod",
	}, *info)

	// Test get pod owned
	info = kubeData.GetByIP("1.2.3.5")
	require.NotNil(t, info)

	require.Equal(t, model.ResourceMetaData{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod2",
			Namespace: "podNamespace",
		},
		Kind:      "Pod",
		HostName:  "node1",
		HostIP:    "10.0.0.1",
		OwnerName: "dep1",
		OwnerKind: "Deployment",
	}, *info)

	// Test get node
	info = kubeData.GetByIP("10.0.0.1")
	require.NotNil(t, info)

	require.Equal(t, model.ResourceMetaData{
		ObjectMeta: v1.ObjectMeta{
			Name: "node1",
		},
		Kind:      "Node",
		OwnerName: "node1",
		OwnerKind: "Node",
	}, *info)

	// Test get service
	info = kubeData.GetByIP("1.2.3.100")
	require.NotNil(t, info)

	require.Equal(t, model.ResourceMetaData{
		ObjectMeta: v1.ObjectMeta{
			Name:      "svc1",
			Namespace: "svcNamespace",
		},
		Kind:      "Service",
		OwnerName: "svc1",
		OwnerKind: "Service",
	}, *info)

	// Test no match
	info = kubeData.GetByIP("1.2.3.200")
	require.Nil(t, info)
}
