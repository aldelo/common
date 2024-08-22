package viper

/*
 * Copyright 2020-2023 Aldelo, LP
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
	util "github.com/aldelo/common"
	"github.com/spf13/viper"
	"strings"
	"time"
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
}

// Init will initialize config and readInConfig
// if config file does not exist, false is returned
func (v *ViperConf) Init() (bool, error) {
	// validate
	if util.LenTrim(v.ConfigName) <= 0 && util.LenTrim(v.SpecificConfigFileFullPath) <= 0 {
		return false, errors.New("Init Config Failed: " + "Either Config Name or Config Full Path is Required")
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
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// config file not found, ignore error
			return false, nil
		} else {
			// config file found, but other error occurred
			return false, errors.New("Init Config Failed: (ReadInConfig Action) " + err.Error())
		}
	} else {
		// read success
		// v.viperClient.WatchConfig()
		return true, nil
	}
}

// WatchConfig watches if config file does not exist, this call will panic
func (v *ViperConf) WatchConfig() {
	if v.viperClient != nil {
		v.viperClient.WatchConfig()
	}
}

// ConfigFileUsed returns the current config file full path in use
func (v *ViperConf) ConfigFileUsed() string {
	if v.viperClient != nil {
		return v.viperClient.ConfigFileUsed()
	} else {
		return ""
	}
}

// Default will set default key value pairs, allows method chaining
func (v *ViperConf) Default(key string, value interface{}) *ViperConf {
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
	if v.viperClient == nil {
		return errors.New("Unmarshal Config To Struct Failed: " + "Config Object Needs Initialized")
	}

	if outputStructPtr == nil {
		return errors.New("Unmarshal Config To Struct Failed: " + "Output Struct Ptr is Required")
	}

	var err error

	if len(key) <= 0 {
		err = v.viperClient.Unmarshal(outputStructPtr)
	} else {
		err = v.viperClient.UnmarshalKey(key[0], outputStructPtr)
	}

	if err != nil {
		return errors.New("Unmarshal Config To Struct Failed: (Unmarshal Error) " + err.Error())
	} else {
		return nil
	}
}

// SubConf returns a sub config object based on the given key
func (v *ViperConf) SubConf(key string) *ViperConf {
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
			}
		} else {
			return nil
		}
	} else {
		return nil
	}
}

// Set will set key value pair into config object
func (v *ViperConf) Set(key string, value interface{}) *ViperConf {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		v.viperClient.Set(key, value)
	}

	return v
}

// Save will save the current config object into target config disk file
func (v *ViperConf) Save() error {
	if v.viperClient == nil {
		return errors.New("Save Config Failed: " + "Config Client Not Initialized")
	}

	fileName := v.SpecificConfigFileFullPath

	if util.LenTrim(fileName) <= 0 {
		fileName = v.ConfigFileUsed()

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
			return errors.New("Save Config Failed: (WriteConfig Action) " + "Config File Name Must Not Be Same as Folder Name")
		} else {
			return errors.New("Save Config Failed: (WriteConfig Action) " + err.Error())
		}
	} else {
		return nil
	}
}

// Alias will create an alias for the related key,
// this allows the alias name and key name both refer to the same stored config data
func (v *ViperConf) Alias(key string, alias string) *ViperConf {
	if v.viperClient != nil && util.LenTrim(key) > 0 && util.LenTrim(alias) > 0 {
		v.viperClient.RegisterAlias(alias, key)
	}

	return v
}

// IsDefined indicates if a key is defined within the config file
func (v *ViperConf) IsDefined(key string) bool {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.InConfig(key)
	} else {
		return false
	}
}

// IsSet indicates if a key's value was set within the config file
func (v *ViperConf) IsSet(key string) bool {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.IsSet(key)
	} else {
		return false
	}
}

// Size returns the given key's value in bytes
func (v *ViperConf) Size(key string) int64 {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return int64(v.viperClient.GetSizeInBytes(key))
	} else {
		return 0
	}
}

// Get returns value by key
func (v *ViperConf) Get(key string) interface{} {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.Get(key)
	} else {
		return nil
	}
}

// GetInt returns value by key
func (v *ViperConf) GetInt(key string) int {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetInt(key)
	} else {
		return 0
	}
}

// GetIntSlice returns value by key
func (v *ViperConf) GetIntSlice(key string) []int {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetIntSlice(key)
	} else {
		return nil
	}
}

// GetInt64 returns value by key
func (v *ViperConf) GetInt64(key string) int64 {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetInt64(key)
	} else {
		return 0
	}
}

// GetFloat64 returns value by key
func (v *ViperConf) GetFloat64(key string) float64 {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetFloat64(key)
	} else {
		return 0.00
	}
}

// GetBool returns value by key
func (v *ViperConf) GetBool(key string) bool {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetBool(key)
	} else {
		return false
	}
}

// GetTime returns value by key
func (v *ViperConf) GetTime(key string) time.Time {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetTime(key)
	} else {
		return time.Time{}
	}
}

// GetDuration returns value by key
func (v *ViperConf) GetDuration(key string) time.Duration {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetDuration(key)
	} else {
		return 0
	}
}

// GetString returns value by key
func (v *ViperConf) GetString(key string) string {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetString(key)
	} else {
		return ""
	}
}

// GetStringSlice returns value by key
func (v *ViperConf) GetStringSlice(key string) []string {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetStringSlice(key)
	} else {
		return nil
	}
}

// GetStringMapInterface returns value by key
func (v *ViperConf) GetStringMapInterface(key string) map[string]interface{} {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetStringMap(key)
	} else {
		return nil
	}
}

// GetStringMapString returns value by key
func (v *ViperConf) GetStringMapString(key string) map[string]string {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetStringMapString(key)
	} else {
		return nil
	}
}

// GetStringMapStringSlice returns value by key
func (v *ViperConf) GetStringMapStringSlice(key string) map[string][]string {
	if v.viperClient != nil && util.LenTrim(key) > 0 {
		return v.viperClient.GetStringMapStringSlice(key)
	} else {
		return nil
	}
}

// AllKeys returns all keys in config file
func (v *ViperConf) AllKeys() []string {
	if v.viperClient != nil {
		return v.viperClient.AllKeys()
	} else {
		return nil
	}
}

// AllSettings returns map of all settings in config file
func (v *ViperConf) AllSettings() map[string]interface{} {
	if v.viperClient != nil {
		return v.viperClient.AllSettings()
	} else {
		return nil
	}
}
