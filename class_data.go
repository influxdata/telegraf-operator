/*
Copyright (c) 2020 InfluxData

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/influxdata/toml"
)

type classDataHandler struct {
	Logger                   logr.Logger
	TelegrafClassesDirectory string
}

func (c *classDataHandler) validateClassData() error {
	classDataValid := true
	filesAvailable := false

	c.Logger.Info(fmt.Sprintf("validating class data from directory %s", c.TelegrafClassesDirectory))

	files, err := ioutil.ReadDir(c.TelegrafClassesDirectory)
	if err != nil {
		c.Logger.Info(fmt.Sprintf("unable to retrieve class data from directory: %v", err))
	}

	for _, file := range files {
		stat, err := os.Stat(filepath.Join(c.TelegrafClassesDirectory, file.Name()))
		if err != nil {
			c.Logger.Info(fmt.Sprintf("unable to stat %s: %v", file.Name(), err))
			continue
		}

		if stat.Mode().IsRegular() {
			data, err := ioutil.ReadFile(filepath.Join(c.TelegrafClassesDirectory, file.Name()))
			if err != nil {
				c.Logger.Info(fmt.Sprintf("unable to retrieve class data from file %s: %v", file.Name(), err))
			} else {
				filesAvailable = true
				if _, err := toml.Parse(data); err != nil {
					c.Logger.Info(fmt.Sprintf("unable to parse class data %s: %v", file.Name(), err))
					classDataValid = false
				}
			}
		}
	}

	if !classDataValid {
		return fmt.Errorf("class data contains errors ; unable to continue")
	}

	if !filesAvailable {
		return fmt.Errorf("no class data found ; unable to continue")
	}

	return nil
}

func (c *classDataHandler) getData(className string) (string, error) {
	data, err := ioutil.ReadFile(filepath.Join(c.TelegrafClassesDirectory, className))

	if err != nil {
		c.Logger.Info("unable to class data for %s: %v", className, err)
		return "", err
	}

	return string(data), nil
}
