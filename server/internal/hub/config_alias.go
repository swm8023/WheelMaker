package hub

import sharedcfg "github.com/swm8023/wheelmaker/internal/shared/config"

type Config = sharedcfg.Config
type LogConfig = sharedcfg.LogConfig
type ProjectConfig = sharedcfg.ProjectConfig
type IMConfig = sharedcfg.IMConfig
type ClientConf = sharedcfg.ClientConf
type RegistryConfig = sharedcfg.RegistryConfig

func LoadConfig(path string) (*Config, error) {
	return sharedcfg.LoadConfig(path)
}
