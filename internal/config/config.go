package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var saveMu sync.Mutex

type Config struct {
	NodeID     string            `json:"node_id"`
	KnownPeers map[string]string `json:"known_peers"`
}

func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg.KnownPeers = map[string]string{}
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		backup := fmt.Sprintf("%s.bad-%d", path, time.Now().Unix())
		_ = os.Rename(path, backup)
		cfg.KnownPeers = map[string]string{}
		return cfg, nil
	}
	if cfg.KnownPeers == nil {
		cfg.KnownPeers = map[string]string{}
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	saveMu.Lock()
	defer saveMu.Unlock()

	if cfg.KnownPeers == nil {
		cfg.KnownPeers = map[string]string{}
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, append(data, '\n'), 0644); err != nil {
		return err
	}
	if err := replaceFile(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func replaceFile(tmp, path string) error {
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}
