/*
Copyright 2022 The KubeOne Authors.

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
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"text/template"

	"github.com/MakeNowJust/heredoc/v2"

	"k8c.io/kubeone/testv2/e2e"

	"sigs.k8s.io/yaml"
)

type Infrastructure struct {
	Name      string `json:"name"`
	AlwaysRun bool   `json:"alwaysRun"`
	Optional  bool   `json:"optional"`
}

type KubeoneTest struct {
	Scenario        string           `json:"scenario"`
	InitVersion     string           `json:"initVersion"`
	UpgradedVersion string           `json:"upgradedVersion"`
	Infrastructures []Infrastructure `json:"infrastructures"`
}

var (
	filePathFlag    string
	packageNameFlag string
	outputFileFlag  string
	outputType      string
)

const fileHeader = `// Code generated by e2e/generator, DO NOT EDIT.

package {{.PackageName}}

import (
	"testing"
)

func TestStub(t *testing.T) {
	t.Skip("stub is skipped")
}`

func main() {
	flag.StringVar(&filePathFlag, "file", "", "path to the YAML file with tests definitions to generate")
	flag.StringVar(&packageNameFlag, "package", "e2e", "the name of the generated Go package")
	flag.StringVar(&outputType, "type", "", "the type of the generator output (yaml|go)")
	flag.StringVar(&outputFileFlag, "output", "-", "the name of the file to write to, - for stdout")
	flag.Parse()

	if filePathFlag == "" {
		log.Fatal("-file argument in required")
	}

	var generatorType e2e.GeneratorType

	switch outputType {
	case "":
		log.Fatal("-type argument is required")
	case "go":
		generatorType = e2e.GeneratorTypeGo
	case "yaml":
		generatorType = e2e.GeneratorTypeYAML
	default:
		log.Fatalf("-type=%s argument is invalid", outputType)
	}

	var outputBuf io.ReadWriter = &bytes.Buffer{}

	if outputFileFlag == "-" {
		outputBuf = os.Stdout
	}

	buf, err := os.ReadFile(filePathFlag)
	if err != nil {
		log.Fatal(err)
	}

	var getTests []KubeoneTest

	if err = yaml.UnmarshalStrict(buf, &getTests); err != nil {
		log.Fatal(err)
	}

	switch generatorType {
	case e2e.GeneratorTypeGo:
		err = template.Must(template.New("").Parse(fileHeader)).Execute(outputBuf, struct {
			PackageName string
		}{
			PackageName: packageNameFlag,
		})
		if err != nil {
			log.Fatal(err)
		}
	case e2e.GeneratorTypeYAML:
		fmt.Fprintf(outputBuf, heredoc.Doc(`
		# Code generated by e2e/generator, DO NOT EDIT.
		presubmits:
		`))
	}

	for _, genTest := range getTests {
		scenario, ok := e2e.Scenarios[genTest.Scenario]
		if !ok {
			log.Fatalf("%q scenario is not defined", genTest.Scenario)
		}

		for _, genInfra := range genTest.Infrastructures {
			infra, ok := e2e.Infrastructures[genInfra.Name]
			if !ok {
				log.Fatalf("%q infra is not defined", genInfra.Name)
			}

			scenario.SetInfra(infra)
			versions := []string{genTest.InitVersion}
			if genTest.UpgradedVersion != "" {
				versions = append(versions, genTest.UpgradedVersion)
			}
			scenario.SetVersions(versions...)

			cfg := e2e.ProwConfig{
				AlwaysRun: genInfra.AlwaysRun,
				Optional:  genInfra.Optional,
			}

			if err = scenario.GenerateTests(outputBuf, generatorType, cfg); err != nil {
				log.Fatal(err)
			}
		}
	}

	if outputFileFlag != "-" {
		data, _ := io.ReadAll(outputBuf)
		os.WriteFile(outputFileFlag, data, 0644)
	}
}
