package viper

/*
 * Copyright 2020-2026 Aldelo, LP
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
 */

import (
	"errors"
	"strings"
	"sync"
	"time"

	util "github.com/aldelo/common"
	"github.com/spf13/viper"
)

// ViperConf struct info,
// ConfigName or SpecificConfigFileFullPath = One of the field is required
// UseYAML = true uses YAML; false uses JSON
// UseAutomaticEnvVar = true will auto load environment variables that matches to key
// AppName = used by config path options, identifies the name of the app
// UseConfig... = indicates config file search pattern
type ViperConf struct {
	// define config properties
	ConfigName                 string
	SpecificConfigFileFullPath string

	UseYAML            bool
	UseAutomaticEnvVar bool

	AppName                  string
	UseConfigPathEtcAppName  bool
	UseConfigPathHomeAppName bool
	CustomConfigPath         string

	// cache viper config object
	viperClient *viper.Viper

	muInitOnce sync.Once
	mu         *sync.RWMutex
}

func (v *ViperConf) ensureMu() {
	if v == nil {
		return
	}
	if v.mu != nil {
		return
	}

	v.muInitOnce.Do(func() {
		if v.mu == nil {
			v.mu = &sync.RWMutex{}
		}
	})
}

func (v *ViperConf) getClient() *viper.Viper {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.viperClient
}

func (v *ViperConf) setClient(c *viper.Viper) {
	if v == nil {
		return
	}
	v.ensureMu()
	v.mu.Lock()
	defer v.mu.Unlock()
	v.viperClient = c
}

// Init will initialize config and readInConfig
// if config file does not exist, false is returned
func (v *ViperConf) Init() (bool, error) {
	if v == nil {
		return false, errors.New("ViperConf Init Failed: ViperConf receiver is nil")
	}
	v.ensureMu()
	v.mu.Lock()
	defer v.mu.Unlock()

	// validate
	if util.LenTrim(v.ConfigName) <= 0 && util.LenTrim(v.SpecificConfigFileFullPath) <= 0 {
		return false, errors.New("Init Config Failed: Either Config Name or Config Full Path is Required")
	}

	// create new viper client object if needed
	if v.viperClient == nil {
		v.viperClient = viper.New()
	}

	// set viper properties
	if util.LenTrim(v.SpecificConfigFileFullPath) <= 0 {
		v.viperClient.SetConfigName(v.ConfigName)

		if util.LenTrim(v.AppName) > 0 {
			if v.UseConfigPathEtcAppName {
				v.viperClient.AddConfigPath("/etc/" + v.AppName + "/")
			}

			if v.UseConfigPathHomeAppName {
				v.viperClient.AddConfigPath("$HOME/." + v.AppName)
			}
		}

		if util.LenTrim(v.CustomConfigPath) > 0 && v.CustomConfigPath != "." {
			v.viperClient.AddConfigPath(v.CustomConfigPath)
		}

		v.viperClient.AddConfigPath(".")
	} else {
		v.viperClient.SetConfigFile(v.SpecificConfigFileFullPath)
	}

	if v.UseAutomaticEnvVar {
		v.viperClient.AutomaticEnv()
	}

	if v.UseYAML {
		v.viperClient.SetConfigType("yaml")
	} else {
		v.viperClient.SetConfigType("json")
	}

	v.viperClient.SetTypeByDefaultValue(true)

	// read in config data
	if err := v.viperClient.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			// config file not found, ignore error
			return false, nil
		}
		return false, errors.New("Init Config Failed: (ReadInConfig Action) " + err.Error())
	}

	return true, nil
}

// WatchConfig watches if config file does not exist, this call will panic
func (v *ViperConf) WatchConfig() {
	if v == nil {
		return
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient == nil {
		return
	}

	if len(v.viperClient.ConfigFileUsed()) == 0 {
		return
	}

	v.viperClient.WatchConfig()
}

// ConfigFileUsed returns the current config file full path in use
func (v *ViperConf) ConfigFileUsed() string {
	if v == nil {
		return ""
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil {
		return v.viperClient.ConfigFileUsed()
	}

	return ""
}

// Default will set default key value pairs, allows method chaining
func (v *ViperConf) Default(key string, value interface{}) *ViperConf {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.viperClient == nil {
		v.viperClient = viper.New()
	}

	if util.LenTrim(key) > 0 {
		v.viperClient.SetDefault(key, value)
	}

	return v
}

// Unmarshal serializes config content into output struct, optionally serializes a portion of config identified by key into a struct
//
// struct tag use `mapstructure:""`
// if struct tag refers to sub element of yaml, define struct to contain the sub elements
//
//	 	for example:
//				parentkey:
//					childkey: abc
//					childkey_2: xyz
//			type Child struct { ChildKey string `mapstructure:"childkey"`  ChildKey2 string `mapstructure:"childkey_2"` }
//			type Parent struct { ChildData Child `mapstructure:"parentkey"`}
func (v *ViperConf) Unmarshal(outputStructPtr interface{}, key ...string) error {
	if v == nil {
		return errors.New("ViperConf Unmarshal Failed: ViperConf receiver is nil")
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient == nil {
		return errors.New("Unmarshal Config To Struct Failed: Config Object Needs Initialized")
	}

	if outputStructPtr == nil {
		return errors.New("Unmarshal Config To Struct Failed: Output Struct Ptr is Required")
	}

	var err error

	if len(key) <= 0 || util.LenTrim(key[0]) == 0 {
		err = v.viperClient.Unmarshal(outputStructPtr)
	} else {
		err = v.viperClient.UnmarshalKey(key[0], outputStructPtr)
	}

	if err != nil {
		return errors.New("Unmarshal Config To Struct Failed: (Unmarshal Error) " + err.Error())
	}

	return nil
}

// SubConf returns a sub config object based on the given key
func (v *ViperConf) SubConf(key string) *ViperConf {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil {
		subViper := v.viperClient.Sub(key)

		if subViper != nil {
			return &ViperConf{
				ConfigName:                 v.ConfigName,
				SpecificConfigFileFullPath: v.SpecificConfigFileFullPath,
				UseYAML:                    v.UseYAML,
				UseAutomaticEnvVar:         v.UseAutomaticEnvVar,
				AppName:                    v.AppName,
				UseConfigPathEtcAppName:    v.UseConfigPathEtcAppName,
				UseConfigPathHomeAppName:   v.UseConfigPathHomeAppName,
				CustomConfigPath:           v.CustomConfigPath,
				viperClient:                subViper,
				mu:                         v.mu,
			}
		}
	}

	return nil
}

// Set will set key value pair into config object
func (v *ViperConf) Set(key string, value interface{}) *ViperConf {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		v.viperClient.Set(key, value)
	}

	return v
}

// Save will save the current config object into target config disk file
func (v *ViperConf) Save() error {
	if v == nil {
		return errors.New("ViperConf Save Failed: ViperConf receiver is nil")
	}
	v.ensureMu()
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.viperClient == nil {
		return errors.New("Save Config Failed: Config Client Not Initialized")
	}

	fileName := v.SpecificConfigFileFullPath

	if util.LenTrim(fileName) <= 0 {
		fileName = v.viperClient.ConfigFileUsed()

		if util.LenTrim(fileName) <= 0 {
			fileName = "./" + v.ConfigName

			if v.UseYAML {
				fileName += ".yaml"
			} else {
				fileName += ".json"
			}
		}
	}

	var err error

	if util.FileExists(fileName) {
		err = v.viperClient.WriteConfig()
	} else {
		err = v.viperClient.WriteConfigAs(fileName)
	}

	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "invalid trailing UTF-8 octet") {
			return errors.New("Save Config Failed: (WriteConfig Action) Config File Name Must Not Be Same as Folder Name")
		}

		return errors.New("Save Config Failed: (WriteConfig Action) " + err.Error())
	}

	return nil
}

// Alias will create an alias for the related key,
// this allows the alias name and key name both refer to the same stored config data
func (v *ViperConf) Alias(key string, alias string) *ViperConf {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 && util.LenTrim(alias) > 0 {
		v.viperClient.RegisterAlias(alias, key)
	}

	return v
}

// IsDefined indicates if a key is defined within the config file
func (v *ViperConf) IsDefined(key string) bool {
	if v == nil {
		return false
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.InConfig(key)
	}

	return false
}

// IsSet indicates if a key's value was set within the config file
func (v *ViperConf) IsSet(key string) bool {
	if v == nil {
		return false
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.IsSet(key)
	}

	return false
}

// Size returns the given key's value in bytes
func (v *ViperConf) Size(key string) int64 {
	if v == nil {
		return 0
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return int64(v.viperClient.GetSizeInBytes(key))
	}

	return 0
}

// Get returns value by key
func (v *ViperConf) Get(key string) interface{} {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.Get(key)
	}

	return nil
}

// GetInt returns value by key
func (v *ViperConf) GetInt(key string) int {
	if v == nil {
		return 0
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetInt(key)
	}

	return 0
}

// GetIntSlice returns value by key
func (v *ViperConf) GetIntSlice(key string) []int {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetIntSlice(key)
	}

	return nil
}

// GetInt64 returns value by key
func (v *ViperConf) GetInt64(key string) int64 {
	if v == nil {
		return 0
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetInt64(key)
	}

	return 0
}

// GetFloat64 returns value by key
func (v *ViperConf) GetFloat64(key string) float64 {
	if v == nil {
		return 0.00
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetFloat64(key)
	}

	return 0.00
}

// GetBool returns value by key
func (v *ViperConf) GetBool(key string) bool {
	if v == nil {
		return false
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetBool(key)
	}

	return false
}

// GetTime returns value by key
func (v *ViperConf) GetTime(key string) time.Time {
	if v == nil {
		return time.Time{}
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetTime(key)
	}

	return time.Time{}
}

// GetDuration returns value by key
func (v *ViperConf) GetDuration(key string) time.Duration {
	if v == nil {
		return 0
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetDuration(key)
	}

	return 0
}

// GetString returns value by key
func (v *ViperConf) GetString(key string) string {
	if v == nil {
		return ""
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetString(key)
	}

	return ""
}

// GetStringSlice returns value by key
func (v *ViperConf) GetStringSlice(key string) []string {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetStringSlice(key)
	}

	return nil
}

// GetStringMapInterface returns value by key
func (v *ViperConf) GetStringMapInterface(key string) map[string]interface{} {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetStringMap(key)
	}

	return nil
}

// GetStringMapString returns value by key
func (v *ViperConf) GetStringMapString(key string) map[string]string {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetStringMapString(key)
	}

	return nil
}

// GetStringMapStringSlice returns value by key
func (v *ViperConf) GetStringMapStringSlice(key string) map[string][]string {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetStringMapStringSlice(key)
	}

	return nil
}

// AllKeys returns all keys in config file
func (v *ViperConf) AllKeys() []string {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil {
		return v.viperClient.AllKeys()
	}

	return nil
}

// AllSettings returns map of all settings in config file
func (v *ViperConf) AllSettings() map[string]interface{} {
	if v == nil {
		return nil
	}
	v.ensureMu()
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.viperClient != nil {
		return v.viperClient.AllSettings()
	}

	return nil
}
