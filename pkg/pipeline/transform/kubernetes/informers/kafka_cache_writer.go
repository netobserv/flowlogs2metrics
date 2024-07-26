package informers

import (
	"context"

	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/transform/kubernetes/model"
	kafkago "github.com/segmentio/kafka-go"
	"k8s.io/client-go/tools/cache"
)

func getKafkaEventHandlers(kafka *kafkago.Writer) cache.ResourceEventHandler {
	return &cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if res, ok := obj.(*model.ResourceMetaData); ok {
				publish(&model.KafkaCacheMessage{
					Operation: model.OperationAdd,
					Resource:  res,
				}, kafka)
			}
		},
		UpdateFunc: func(_, new any) {
			if res, ok := new.(*model.ResourceMetaData); ok {
				publish(&model.KafkaCacheMessage{
					Operation: model.OperationUpdate,
					Resource:  res,
				}, kafka)
			}
		},
		DeleteFunc: func(obj any) {
			if res, ok := obj.(*model.ResourceMetaData); ok {
				publish(&model.KafkaCacheMessage{
					Operation: model.OperationDelete,
					Resource:  res,
				}, kafka)
			}
		},
	}
}

func publish(content *model.KafkaCacheMessage, kafka *kafkago.Writer) {
	log.Debugf("Publishing to Kafka: %v", content.Resource)
	b, err := content.ToBytes()
	if err != nil {
		log.Errorf("kafka publish, encoding error: %v", err)
		return
	}
	msg := kafkago.Message{Value: b}
	err = kafka.WriteMessages(context.Background(), msg)
	if err != nil {
		log.Errorf("kafka publish, write error: %v", err)
	}
}
