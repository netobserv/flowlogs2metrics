package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEncodeDecode(t *testing.T) {
	msg := KafkaCacheMessage{
		Operation: OperationAdd,
		Resource: &ResourceMetaData{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
				Labels: map[string]string{
					"label-1": "value-1",
				},
			},
			Kind: KindNode,
			IPs:  []string{"1.2.3.4", "4.5.6.7"},
		},
	}

	b, err := msg.ToBytes()
	require.NoError(t, err)

	decoded, err := MessageFromBytes(b)
	require.NoError(t, err)

	require.Equal(t, msg, *decoded)
}
