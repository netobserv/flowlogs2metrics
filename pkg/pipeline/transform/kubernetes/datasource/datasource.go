package datasource

import (
	"context"
	"sync"

	"github.com/netobserv/flowlogs-pipeline/pkg/api"
	"github.com/netobserv/flowlogs-pipeline/pkg/kafka"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/transform/kubernetes/informers"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/transform/kubernetes/model"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/utils"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("component", "transform.Network.Kubernetes")

type Datasource struct {
	Informers informers.InformersInterface
	// We use map+mutex rather than sync.Map for better performance on writes, since the lock is acquired once to perform several writes.
	kafkaIPCacheMut       sync.RWMutex
	kafkaIPCache          map[string]model.ResourceMetaData
	kafkaNodeNameCacheMut sync.RWMutex
	kafkaNodeNameCache    map[string]model.ResourceMetaData
}

func NewInformerDatasource(kubeConfigPath string, kafkaConfig *api.EncodeKafka) (*Datasource, error) {
	inf := &informers.Informers{}
	if err := inf.InitFromConfig(kubeConfigPath, kafkaConfig); err != nil {
		return nil, err
	}
	return &Datasource{Informers: inf}, nil
}

func NewKafkaCacheDatasource(kafkaConfig *api.IngestKafka) (*Datasource, error) {
	// Init Kafka reader
	log.Debug("Initializing Kafka reader datasource")
	kafkaReader, _, err := kafka.NewReader(kafkaConfig)
	if err != nil {
		return nil, err
	}

	d := Datasource{
		kafkaIPCache:       make(map[string]model.ResourceMetaData),
		kafkaNodeNameCache: make(map[string]model.ResourceMetaData),
	}
	exitChan := utils.ExitChannel()
	go func() {
		for {
			select {
			case <-exitChan:
				log.Info("gracefully exiting")
				return
			default:
			}
			// Blocking
			msg, err := kafkaReader.ReadMessage(context.Background())
			if err != nil {
				log.Errorln(err)
				continue
			}
			if len(msg.Value) > 0 {
				content, err := model.MessageFromBytes(msg.Value)
				if err != nil {
					log.Errorln(err)
					continue
				}
				log.Debugf("Kafka reader: got message %v", content)
				d.updateCache(content)
			} else {
				log.Debug("Kafka reader: empty message")
			}
		}
	}()

	return &d, nil
}

func (d *Datasource) updateCache(msg *model.KafkaCacheMessage) {
	switch msg.Operation {
	case model.OperationAdd, model.OperationUpdate:
		d.kafkaIPCacheMut.Lock()
		for _, ip := range msg.Resource.IPs {
			d.kafkaIPCache[ip] = *msg.Resource
		}
		d.kafkaIPCacheMut.Unlock()
		if msg.Resource.Kind == model.KindNode {
			d.kafkaNodeNameCacheMut.Lock()
			d.kafkaNodeNameCache[msg.Resource.Name] = *msg.Resource
			d.kafkaNodeNameCacheMut.Unlock()
		}
	case model.OperationDelete:
		d.kafkaIPCacheMut.Lock()
		for _, ip := range msg.Resource.IPs {
			delete(d.kafkaIPCache, ip)
		}
		d.kafkaIPCacheMut.Unlock()
		if msg.Resource.Kind == model.KindNode {
			d.kafkaNodeNameCacheMut.Lock()
			delete(d.kafkaNodeNameCache, msg.Resource.Name)
			d.kafkaNodeNameCacheMut.Unlock()
		}
	}
}

func (d *Datasource) GetByIP(ip string) *model.ResourceMetaData {
	if d.Informers != nil {
		return d.Informers.GetByIP(ip)
	}
	d.kafkaIPCacheMut.RLock()
	defer d.kafkaIPCacheMut.RUnlock()
	if obj, ok := d.kafkaIPCache[ip]; ok {
		return &obj
	}
	return nil
}

func (d *Datasource) GetNodeByName(name string) (*model.ResourceMetaData, error) {
	if d.Informers != nil {
		return d.Informers.GetNodeByName(name)
	}
	d.kafkaNodeNameCacheMut.RLock()
	defer d.kafkaNodeNameCacheMut.RUnlock()
	if obj, ok := d.kafkaNodeNameCache[name]; ok {
		return &obj, nil
	}
	return nil, nil
}
