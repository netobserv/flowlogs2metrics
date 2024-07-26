/*
 * Copyright (C) 2021 IBM, Inc.
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

package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/api"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/netobserv/flowlogs-pipeline/pkg/operational"
	"github.com/netobserv/flowlogs-pipeline/pkg/test"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testConfig1 = `---
log-level: debug
pipeline:
  - name: ingest1
parameters:
  - name: ingest1
    ingest:
      type: kafka
      kafka:
        brokers: ["1.1.1.1:9092"]
        topic: topic1
        groupid: group1
        startOffset: FirstOffset
        groupBalancers: ["range", "roundRobin"]
        batchReadTimeout: 300
        decoder:
          type: json
`

const testConfig2 = `---
log-level: debug
pipeline:
  - name: ingest1
parameters:
  - name: ingest1
    ingest:
      type: kafka
      kafka:
        brokers: ["1.1.1.2:9092"]
        topic: topic2
        groupid: group2
        startOffset: LastOffset
        groupBalancers: ["rackAffinity"]
        decoder:
          type: json
        batchMaxLen: 1000
        commitInterval: 1000
`

func initNewIngestKafka(t *testing.T, configTemplate string) Ingester {
	test.ResetPromRegistry()
	v, cfg := test.InitConfig(t, configTemplate)
	require.NotNil(t, v)

	newIngest, err := NewIngestKafka(operational.NewMetrics(&config.MetricsSettings{}), cfg.Parameters[0])
	require.NoError(t, err)
	return newIngest
}

func Test_NewIngestKafka1(t *testing.T) {
	newIngest := initNewIngestKafka(t, testConfig1)
	ingestKafka := newIngest.(*ingestKafka)
	require.Equal(t, "topic1", ingestKafka.kafkaReader.Config().Topic)
	require.Equal(t, "group1", ingestKafka.kafkaReader.Config().GroupID)
	expectedBrokers := []string{"1.1.1.1:9092"}
	require.Equal(t, expectedBrokers, ingestKafka.kafkaReader.Config().Brokers)
	require.Equal(t, int64(-2), ingestKafka.kafkaReader.Config().StartOffset)
	require.Equal(t, 2, len(ingestKafka.kafkaReader.Config().GroupBalancers))
	require.Equal(t, time.Duration(500)*time.Millisecond, ingestKafka.kafkaReader.Config().CommitInterval)
}

func Test_NewIngestKafka2(t *testing.T) {
	newIngest := initNewIngestKafka(t, testConfig2)
	ingestKafka := newIngest.(*ingestKafka)
	require.Equal(t, "topic2", ingestKafka.kafkaReader.Config().Topic)
	require.Equal(t, "group2", ingestKafka.kafkaReader.Config().GroupID)
	expectedBrokers := []string{"1.1.1.2:9092"}
	require.Equal(t, expectedBrokers, ingestKafka.kafkaReader.Config().Brokers)
	require.Equal(t, int64(-1), ingestKafka.kafkaReader.Config().StartOffset)
	require.Equal(t, 1, len(ingestKafka.kafkaReader.Config().GroupBalancers))
	require.Equal(t, time.Duration(1000)*time.Millisecond, ingestKafka.kafkaReader.Config().CommitInterval)
}

func removeTimestamp(receivedEntry config.GenericMap) {
	delete(receivedEntry, "TimeReceived")
}

func Test_IngestKafka(t *testing.T) {
	newIngest := initNewIngestKafka(t, testConfig1)
	ingestKafka := newIngest.(*ingestKafka)
	ingestOutput := make(chan config.GenericMap)

	// run Ingest in a separate thread
	go func() {
		ingestKafka.Ingest(ingestOutput)
	}()
	// wait a second for the ingest pipeline to come up
	time.Sleep(time.Second)

	// feed some data into the pipeline
	record1 := "{\"Bytes\":20801,\"DstAddr\":\"10.130.2.1\",\"DstPort\":36936,\"Packets\":401,\"SrcAddr\":\"10.130.2.13\",\"SrcPort\":3100}"
	record2 := "{\"Bytes\":20802,\"DstAddr\":\"10.130.2.2\",\"DstPort\":36936,\"Packets\":402,\"SrcAddr\":\"10.130.2.13\",\"SrcPort\":3100}"
	record3 := "{\"Bytes\":20803,\"DstAddr\":\"10.130.2.3\",\"DstPort\":36936,\"Packets\":403,\"SrcAddr\":\"10.130.2.13\",\"SrcPort\":3100}"

	inChan := ingestKafka.in
	inChan <- []byte(record1)
	inChan <- []byte(record2)
	inChan <- []byte(record3)

	// wait for the data to have been processed
	receivedEntry, err := test.WaitFromChannel(ingestOutput, timeout)
	require.NoError(t, err)
	// we remove timestamp for test stability
	// Timereceived field is tested in the decodeJson tests
	removeTimestamp(receivedEntry)
	require.Equal(t, test.DeserializeJSONToMap(t, record1), receivedEntry)

	receivedEntry, err = test.WaitFromChannel(ingestOutput, timeout)
	require.NoError(t, err)
	removeTimestamp(receivedEntry)
	require.Equal(t, test.DeserializeJSONToMap(t, record2), receivedEntry)

	receivedEntry, err = test.WaitFromChannel(ingestOutput, timeout)
	require.NoError(t, err)
	removeTimestamp(receivedEntry)
	require.Equal(t, test.DeserializeJSONToMap(t, record3), receivedEntry)
}

type fakeKafkaReader struct {
	readToDo int
	mock.Mock
}

var fakeRecord = []byte(`{"Bytes":20801,"DstAddr":"10.130.2.1","DstPort":36936,"Packets":401,"SrcAddr":"10.130.2.13","SrcPort":3100}`)

// ReadMessage runs in the kafka client thread, which blocks until data is available.
// If data is always available, we have an infinite loop. So we return data only a specified number of time.
func (f *fakeKafkaReader) ReadMessage(_ context.Context) (kafkago.Message, error) {
	if f.readToDo == 0 {
		// block indefinitely
		c := make(chan struct{})
		<-c
	}
	message := kafkago.Message{
		Topic: "topic1",
		Value: fakeRecord,
	}
	f.readToDo--
	return message, nil
}

func (f *fakeKafkaReader) Config() kafkago.ReaderConfig {
	return kafkago.ReaderConfig{}
}

func (f *fakeKafkaReader) Stats() kafkago.ReaderStats {
	return kafkago.ReaderStats{}
}

func Test_KafkaListener(t *testing.T) {
	ingestOutput := make(chan config.GenericMap)
	newIngest := initNewIngestKafka(t, testConfig1)
	ingestKafka := newIngest.(*ingestKafka)

	// change the ReadMessage function to the mock-up
	fr := fakeKafkaReader{readToDo: 1}
	ingestKafka.kafkaReader = &fr

	// run Ingest in a separate thread
	go func() {
		ingestKafka.Ingest(ingestOutput)
	}()

	// wait for the data to have been processed
	receivedEntry, err := test.WaitFromChannel(ingestOutput, 2*time.Second)
	require.NoError(t, err)

	// we remove timestamp for test stability
	// Timereceived field is tested in the decodeJson tests
	removeTimestamp(receivedEntry)

	require.Equal(t, test.DeserializeJSONToMap(t, string(fakeRecord)), receivedEntry)
}

func Test_TLSConfigEmpty(t *testing.T) {
	test.ResetPromRegistry()
	stage := config.NewKafkaPipeline("ingest-kafka", api.IngestKafka{
		Brokers: []string{"any"},
		Topic:   "topic",
		Decoder: api.Decoder{Type: "json"},
	})
	newIngest, err := NewIngestKafka(operational.NewMetrics(&config.MetricsSettings{}), stage.GetStageParams()[0])
	require.NoError(t, err)
	tlsConfig := newIngest.(*ingestKafka).kafkaReader.Config().Dialer.TLS
	require.Nil(t, tlsConfig)
}

func Test_TLSConfigCA(t *testing.T) {
	test.ResetPromRegistry()
	ca, cleanup := test.CreateCACert(t)
	defer cleanup()
	stage := config.NewKafkaPipeline("ingest-kafka", api.IngestKafka{
		Brokers: []string{"any"},
		Topic:   "topic",
		Decoder: api.Decoder{Type: "json"},
		TLS: &api.ClientTLS{
			CACertPath: ca,
		},
	})
	newIngest, err := NewIngestKafka(operational.NewMetrics(&config.MetricsSettings{}), stage.GetStageParams()[0])
	require.NoError(t, err)

	tlsConfig := newIngest.(*ingestKafka).kafkaReader.Config().Dialer.TLS

	require.Empty(t, tlsConfig.Certificates)
	require.NotNil(t, tlsConfig.RootCAs)
	require.Len(t, tlsConfig.RootCAs.Subjects(), 1) //nolint:staticcheck
}

func Test_MutualTLSConfig(t *testing.T) {
	test.ResetPromRegistry()
	ca, user, userKey, cleanup := test.CreateAllCerts(t)
	defer cleanup()
	stage := config.NewKafkaPipeline("ingest-kafka", api.IngestKafka{
		Brokers: []string{"any"},
		Topic:   "topic",
		Decoder: api.Decoder{Type: "json"},
		TLS: &api.ClientTLS{
			CACertPath:   ca,
			UserCertPath: user,
			UserKeyPath:  userKey,
		},
	})
	newIngest, err := NewIngestKafka(operational.NewMetrics(&config.MetricsSettings{}), stage.GetStageParams()[0])
	require.NoError(t, err)

	tlsConfig := newIngest.(*ingestKafka).kafkaReader.Config().Dialer.TLS

	require.Len(t, tlsConfig.Certificates, 1)
	require.NotEmpty(t, tlsConfig.Certificates[0].Certificate)
	require.NotNil(t, tlsConfig.Certificates[0].PrivateKey)
	require.NotNil(t, tlsConfig.RootCAs)
	require.Len(t, tlsConfig.RootCAs.Subjects(), 1) //nolint:staticcheck
}
