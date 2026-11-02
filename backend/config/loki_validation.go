package config

import (
	"fmt"
	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"gopkg.in/yaml.v3"
)

func validateLokiYAML(raw []byte) error {
	var root struct {
		Log struct {
			Loki logcfg.LokiConfig `yaml:"loki"`
		} `yaml:"log"`
	}
	if yaml.Unmarshal(raw, &root) != nil {
		return fmt.Errorf("log.loki fields invalid")
	}
	c := root.Log.Loki
	for _, v := range []struct{ value, maximum int64 }{{int64(c.QueueCapacity), 10000}, {c.MaxQueueBytes, 64 << 20}, {int64(c.MaxEntryBytes), 1 << 20}, {int64(c.BatchSize), 1000}, {int64(c.BatchWait), 10}, {int64(c.RequestTimeoutMS), 10000}, {int64(c.FlushTimeoutMS), 10000}} {
		if v.value < 0 || v.value > v.maximum {
			return fmt.Errorf("log.loki delivery bound invalid")
		}
	}
	entry := c.MaxEntryBytes
	if entry == 0 {
		entry = 64 << 10
	}
	queue := c.MaxQueueBytes
	if queue == 0 {
		queue = 8 << 20
	}
	if int64(entry) > queue {
		return fmt.Errorf("log.loki entry exceeds queue byte budget")
	}
	return nil
}
