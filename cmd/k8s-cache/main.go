package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/netobserv/flowlogs-pipeline/pkg/api"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/transform/kubernetes"
	"github.com/netobserv/flowlogs-pipeline/pkg/pipeline/utils"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

var (
	buildVersion = "unknown"
	buildDate    = "unknown"
	app          = "flp-cache"
	configPath   = flag.String("config", "", "path to a config file")
	versionFlag  = flag.Bool("v", false, "print version")
	log          = logrus.WithField("module", "main")
)

type Config struct {
	KubeConfigPath string          `yaml:"kubeConfigPath"`
	KafkaConfig    api.EncodeKafka `yaml:"kafkaConfig"`
	PProfPort      int32           `yaml:"pprofPort"` // TODO: manage pprof
	LogLevel       string          `yaml:"logLevel"`
}

func main() {
	flag.Parse()

	appVersion := fmt.Sprintf("%s [build version: %s, build date: %s]", app, buildVersion, buildDate)
	if *versionFlag {
		fmt.Println(appVersion)
		os.Exit(0)
	}

	cfg, err := readConfig(*configPath)
	if err != nil {
		log.WithError(err).Fatal("error reading config file")
	}

	lvl, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Errorf("Log level %s not recognized, using info", cfg.LogLevel)
		lvl = logrus.InfoLevel
	}
	logrus.SetLevel(lvl)
	log.Infof("Starting %s at log level %s", appVersion, lvl)
	log.Infof("Configuration: %#v", cfg)

	err = kubernetes.InitInformerDatasource(cfg.KubeConfigPath, &cfg.KafkaConfig)
	if err != nil {
		log.WithError(err).Fatal("error initializing Kubernetes & informers")
	}

	stopCh := utils.SetupElegantExit()
	<-stopCh
}

func readConfig(path string) (*Config, error) {
	var cfg Config
	if len(path) == 0 {
		return &cfg, nil
	}
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, err
}
