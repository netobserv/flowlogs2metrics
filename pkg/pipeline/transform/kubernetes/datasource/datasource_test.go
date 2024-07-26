package datasource

import (
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/transform/kubernetes/model"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	node1 = model.ResourceMetaData{
		ObjectMeta: v1.ObjectMeta{
			Name: "node-1",
		},
		Kind: model.KindNode,
		IPs:  []string{"1.2.3.4", "5.6.7.8"},
	}
	pod1 = model.ResourceMetaData{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "ns-1",
		},
		Kind: model.KindPod,
		IPs:  []string{"10.0.0.1", "10.0.0.2"},
	}
	svc1 = model.ResourceMetaData{
		ObjectMeta: v1.ObjectMeta{
			Name:      "svc-1",
			Namespace: "ns-1",
		},
		Kind: model.KindService,
		IPs:  []string{"192.168.0.1"},
	}
)

func TestCacheUpdate(t *testing.T) {
	ds := Datasource{
		kafkaIPCache:       make(map[string]model.ResourceMetaData),
		kafkaNodeNameCache: make(map[string]model.ResourceMetaData),
	}
	var err error

	res := ds.GetByIP("1.2.3.4")
	require.Nil(t, res)
	res, err = ds.GetNodeByName("node-1")
	require.NoError(t, err)
	require.Nil(t, res)

	ds.updateCache(&model.KafkaCacheMessage{
		Operation: model.OperationAdd,
		Resource:  &node1,
	})

	ds.updateCache(&model.KafkaCacheMessage{
		Operation: model.OperationAdd,
		Resource:  &pod1,
	})

	ds.updateCache(&model.KafkaCacheMessage{
		Operation: model.OperationAdd,
		Resource:  &svc1,
	})

	res = ds.GetByIP("1.2.3.4")
	require.Equal(t, node1, *res)
	res, err = ds.GetNodeByName("node-1")
	require.NoError(t, err)
	require.Equal(t, node1, *res)

	res = ds.GetByIP("5.6.7.8")
	require.Equal(t, node1, *res)

	res = ds.GetByIP("10.0.0.1")
	require.Equal(t, pod1, *res)

	res = ds.GetByIP("192.168.0.1")
	require.Equal(t, svc1, *res)

	svc2 := svc1
	svc2.Labels = map[string]string{"label": "value"}
	ds.updateCache(&model.KafkaCacheMessage{
		Operation: model.OperationUpdate,
		Resource:  &svc2,
	})
	ds.updateCache(&model.KafkaCacheMessage{
		Operation: model.OperationDelete,
		Resource:  &node1,
	})

	res = ds.GetByIP("192.168.0.1")
	require.Equal(t, map[string]string{"label": "value"}, res.Labels)

	res = ds.GetByIP("1.2.3.4")
	require.Nil(t, res)
	res, err = ds.GetNodeByName("node-1")
	require.NoError(t, err)
	require.Nil(t, res)
}

func BenchmarkPromEncode(b *testing.B) {
	ds := Datasource{
		kafkaIPCache:       make(map[string]model.ResourceMetaData),
		kafkaNodeNameCache: make(map[string]model.ResourceMetaData),
	}

	for i := 0; i < b.N; i++ {
		ds.updateCache(&model.KafkaCacheMessage{
			Operation: model.OperationAdd,
			Resource:  &node1,
		})

		ds.updateCache(&model.KafkaCacheMessage{
			Operation: model.OperationAdd,
			Resource:  &pod1,
		})

		ds.updateCache(&model.KafkaCacheMessage{
			Operation: model.OperationAdd,
			Resource:  &svc1,
		})
	}
}
