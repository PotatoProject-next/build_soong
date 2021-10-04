// Copyright 2021 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bazel

import (
	"fmt"
	"strings"
)

const (
	// ArchType names in arch.go
	archArm    = "arm"
	archArm64  = "arm64"
	archX86    = "x86"
	archX86_64 = "x86_64"

	// OsType names in arch.go
	osAndroid     = "android"
	osDarwin      = "darwin"
	osLinux       = "linux_glibc"
	osLinuxMusl   = "linux_musl"
	osLinuxBionic = "linux_bionic"
	osWindows     = "windows"

	// Targets in arch.go
	osArchAndroidArm        = "android_arm"
	osArchAndroidArm64      = "android_arm64"
	osArchAndroidX86        = "android_x86"
	osArchAndroidX86_64     = "android_x86_64"
	osArchDarwinX86_64      = "darwin_x86_64"
	osArchLinuxX86          = "linux_glibc_x86"
	osArchLinuxX86_64       = "linux_glibc_x86_64"
	osArchLinuxMuslX86      = "linux_musl_x86"
	osArchLinuxMuslX86_64   = "linux_musl_x86_64"
	osArchLinuxBionicArm64  = "linux_bionic_arm64"
	osArchLinuxBionicX86_64 = "linux_bionic_x86_64"
	osArchWindowsX86        = "windows_x86"
	osArchWindowsX86_64     = "windows_x86_64"

	// This is the string representation of the default condition wherever a
	// configurable attribute is used in a select statement, i.e.
	// //conditions:default for Bazel.
	//
	// This is consistently named "conditions_default" to mirror the Soong
	// config variable default key in an Android.bp file, although there's no
	// integration with Soong config variables (yet).
	ConditionsDefaultConfigKey = "conditions_default"

	ConditionsDefaultSelectKey = "//conditions:default"

	productVariableBazelPackage = "//build/bazel/product_variables"
)

var (
	// These are the list of OSes and architectures with a Bazel config_setting
	// and constraint value equivalent. These exist in arch.go, but the android
	// package depends on the bazel package, so a cyclic dependency prevents
	// using those variables here.

	// A map of architectures to the Bazel label of the constraint_value
	// for the @platforms//cpu:cpu constraint_setting
	platformArchMap = map[string]string{
		archArm:                    "//build/bazel/platforms/arch:arm",
		archArm64:                  "//build/bazel/platforms/arch:arm64",
		archX86:                    "//build/bazel/platforms/arch:x86",
		archX86_64:                 "//build/bazel/platforms/arch:x86_64",
		ConditionsDefaultConfigKey: ConditionsDefaultSelectKey, // The default condition of as arch select map.
	}

	// A map of target operating systems to the Bazel label of the
	// constraint_value for the @platforms//os:os constraint_setting
	platformOsMap = map[string]string{
		osAndroid:                  "//build/bazel/platforms/os:android",
		osDarwin:                   "//build/bazel/platforms/os:darwin",
		osLinux:                    "//build/bazel/platforms/os:linux",
		osLinuxMusl:                "//build/bazel/platforms/os:linux_musl",
		osLinuxBionic:              "//build/bazel/platforms/os:linux_bionic",
		osWindows:                  "//build/bazel/platforms/os:windows",
		ConditionsDefaultConfigKey: ConditionsDefaultSelectKey, // The default condition of an os select map.
	}

	platformBionicMap = map[string]string{
		"bionic":                   "//build/bazel/platforms/os:bionic",
		ConditionsDefaultConfigKey: ConditionsDefaultSelectKey, // The default condition of an os select map.
	}

	platformOsArchMap = map[string]string{
		osArchAndroidArm:           "//build/bazel/platforms/os_arch:android_arm",
		osArchAndroidArm64:         "//build/bazel/platforms/os_arch:android_arm64",
		osArchAndroidX86:           "//build/bazel/platforms/os_arch:android_x86",
		osArchAndroidX86_64:        "//build/bazel/platforms/os_arch:android_x86_64",
		osArchDarwinX86_64:         "//build/bazel/platforms/os_arch:darwin_x86_64",
		osArchLinuxX86:             "//build/bazel/platforms/os_arch:linux_glibc_x86",
		osArchLinuxX86_64:          "//build/bazel/platforms/os_arch:linux_glibc_x86_64",
		osArchLinuxMuslX86:         "//build/bazel/platforms/os_arch:linux_musl_x86",
		osArchLinuxMuslX86_64:      "//build/bazel/platforms/os_arch:linux_musl_x86_64",
		osArchLinuxBionicArm64:     "//build/bazel/platforms/os_arch:linux_bionic_arm64",
		osArchLinuxBionicX86_64:    "//build/bazel/platforms/os_arch:linux_bionic_x86_64",
		osArchWindowsX86:           "//build/bazel/platforms/os_arch:windows_x86",
		osArchWindowsX86_64:        "//build/bazel/platforms/os_arch:windows_x86_64",
		ConditionsDefaultConfigKey: ConditionsDefaultSelectKey, // The default condition of an os select map.
	}
)

// basic configuration types
type configurationType int

const (
	noConfig configurationType = iota
	arch
	os
	osArch
	bionic
	productVariables
)

func (ct configurationType) String() string {
	return map[configurationType]string{
		noConfig:         "no_config",
		arch:             "arch",
		os:               "os",
		osArch:           "arch_os",
		bionic:           "bionic",
		productVariables: "product_variables",
	}[ct]
}

func (ct configurationType) validateConfig(config string) {
	switch ct {
	case noConfig:
		if config != "" {
			panic(fmt.Errorf("Cannot specify config with %s, but got %s", ct, config))
		}
	case arch:
		if _, ok := platformArchMap[config]; !ok {
			panic(fmt.Errorf("Unknown arch: %s", config))
		}
	case os:
		if _, ok := platformOsMap[config]; !ok {
			panic(fmt.Errorf("Unknown os: %s", config))
		}
	case osArch:
		if _, ok := platformOsArchMap[config]; !ok {
			panic(fmt.Errorf("Unknown os+arch: %s", config))
		}
	case bionic:
		if _, ok := platformBionicMap[config]; !ok {
			panic(fmt.Errorf("Unknown for %s: %s", ct.String(), config))
		}
	case productVariables:
		// do nothing
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationType %d", ct))
	}
}

// SelectKey returns the Bazel select key for a given configurationType and config string.
func (ct configurationType) SelectKey(config string) string {
	ct.validateConfig(config)
	switch ct {
	case noConfig:
		panic(fmt.Errorf("SelectKey is unnecessary for noConfig ConfigurationType "))
	case arch:
		return platformArchMap[config]
	case os:
		return platformOsMap[config]
	case osArch:
		return platformOsArchMap[config]
	case bionic:
		return platformBionicMap[config]
	case productVariables:
		if config == ConditionsDefaultConfigKey {
			return ConditionsDefaultSelectKey
		}
		return fmt.Sprintf("%s:%s", productVariableBazelPackage, strings.ToLower(config))
	default:
		panic(fmt.Errorf("Unrecognized ConfigurationType %d", ct))
	}
}

var (
	// Indicating there is no configuration axis
	NoConfigAxis = ConfigurationAxis{configurationType: noConfig}
	// An axis for architecture-specific configurations
	ArchConfigurationAxis = ConfigurationAxis{configurationType: arch}
	// An axis for os-specific configurations
	OsConfigurationAxis = ConfigurationAxis{configurationType: os}
	// An axis for arch+os-specific configurations
	OsArchConfigurationAxis = ConfigurationAxis{configurationType: osArch}
	// An axis for bionic os-specific configurations
	BionicConfigurationAxis = ConfigurationAxis{configurationType: bionic}
)

// ProductVariableConfigurationAxis returns an axis for the given product variable
func ProductVariableConfigurationAxis(variable string) ConfigurationAxis {
	return ConfigurationAxis{
		configurationType: productVariables,
		subType:           variable,
	}
}

// ConfigurationAxis is an independent axis for configuration, there should be no overlap between
// elements within an axis.
type ConfigurationAxis struct {
	configurationType
	// some configuration types (e.g. productVariables) have multiple independent axes, subType helps
	// distinguish between them without needing to list all 17 product variables.
	subType string
}

func (ca *ConfigurationAxis) less(other ConfigurationAxis) bool {
	if ca.configurationType < other.configurationType {
		return true
	}
	return ca.subType < other.subType
}
