// Copyright (c) 2017 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	vc "github.com/containers/virtcontainers"
	"github.com/containers/virtcontainers/pkg/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

// Semantic version for the output of the "cc-env" command.
//
// XXX: Increment for every change to the output format
// (meaning any change to the EnvInfo type).
const formatVersion = "1.0.0"

// MetaInfo stores information on the format of the output itself
type MetaInfo struct {
	// output format version
	Version string
}

// PathInfo stores a path in its original, and fully resolved forms
type PathInfo struct {
	Path     string
	Resolved string
}

// CPUInfo stores host CPU details
type CPUInfo struct {
	Vendor string
	Model  string
}

// RuntimeConfigInfo stores runtime config details.
type RuntimeConfigInfo struct {
	Location PathInfo
	// Note a PathInfo as it may not exist (validly)
	GlobalLogPath string
}

// RuntimeInfo stores runtime details.
type RuntimeInfo struct {
	Version RuntimeVersionInfo
	Config  RuntimeConfigInfo
}

// RuntimeVersionInfo stores details of the runtime version
type RuntimeVersionInfo struct {
	Semver string
	Commit string
	OCI    string
}

// ProxyInfo stores proxy details
type ProxyInfo struct {
	Type    string
	Version string
	URL     string
}

// ShimInfo stores shim details
type ShimInfo struct {
	Type     string
	Version  string
	Location PathInfo
}

// AgentInfo stores agent details
type AgentInfo struct {
	Type     string
	Version  string
	PauseBin PathInfo
}

// DistroInfo stores host operating system distribution details.
type DistroInfo struct {
	Name    string
	Version string
}

// HostInfo stores host details
type HostInfo struct {
	Kernel    string
	Distro    DistroInfo
	CPU       CPUInfo
	CCCapable bool
}

// EnvInfo collects all information that will be displayed by the
// "cc-env" command.
//
// XXX: Any changes must be coupled with a change to formatVersion.
type EnvInfo struct {
	Meta       MetaInfo
	Runtime    RuntimeInfo
	Hypervisor PathInfo
	Image      PathInfo
	Kernel     PathInfo
	Proxy      ProxyInfo
	Shim       ShimInfo
	Agent      AgentInfo
	Host       HostInfo
}

func getMetaInfo() MetaInfo {
	return MetaInfo{
		Version: formatVersion,
	}
}

func getRuntimeInfo(configFile, logFile string, config oci.RuntimeConfig) (RuntimeInfo, error) {
	configFileResolved, err := filepath.EvalSymlinks(configFile)
	if err != nil {
		return RuntimeInfo{}, err
	}

	runtimeVersion := RuntimeVersionInfo{
		Semver: version,
		Commit: commit,
		OCI:    specs.Version,
	}

	runtimeConfig := RuntimeConfigInfo{
		GlobalLogPath: logFile,
		Location: PathInfo{
			Path:     configFile,
			Resolved: configFileResolved,
		},
	}

	ccRuntime := RuntimeInfo{
		Version: runtimeVersion,
		Config:  runtimeConfig,
	}

	return ccRuntime, nil
}

func getHostInfo() (HostInfo, error) {
	hostKernelVersion, err := getKernelVersion()
	if err != nil {
		return HostInfo{}, err
	}

	hostDistroName, hostDistroVersion, err := getDistroDetails()
	if err != nil {
		return HostInfo{}, err
	}

	cpuVendor, cpuModel, err := getCPUDetails()
	if err != nil {
		return HostInfo{}, err
	}

	hostCCCapable := true
	err = hostIsClearContainersCapable(procCPUInfo)
	if err != nil {
		hostCCCapable = false
	}

	hostDistro := DistroInfo{
		Name:    hostDistroName,
		Version: hostDistroVersion,
	}

	hostCPU := CPUInfo{
		Vendor: cpuVendor,
		Model:  cpuModel,
	}

	ccHost := HostInfo{
		Kernel:    hostKernelVersion,
		Distro:    hostDistro,
		CPU:       hostCPU,
		CCCapable: hostCCCapable,
	}

	return ccHost, nil
}

func getProxyInfo(config oci.RuntimeConfig) (ProxyInfo, error) {
	proxyConfig, ok := config.ProxyConfig.(vc.CCProxyConfig)
	if !ok {
		return ProxyInfo{}, errors.New("cannot determine proxy config")
	}

	proxyURL := proxyConfig.URL

	ccProxy := ProxyInfo{
		Type:    string(config.ProxyType),
		Version: unknown,
		URL:     proxyURL,
	}

	return ccProxy, nil
}

func getShimInfo(config oci.RuntimeConfig) (ShimInfo, error) {
	shimConfig, ok := config.ShimConfig.(vc.CCShimConfig)
	if !ok {
		return ShimInfo{}, errors.New("cannot determine shim config")
	}

	shimPath := shimConfig.Path
	shimPathResolved, err := filepath.EvalSymlinks(shimPath)
	if err != nil {
		return ShimInfo{}, err
	}

	ccShim := ShimInfo{
		Type:    string(config.ShimType),
		Version: unknown,
		Location: PathInfo{
			Path:     shimPath,
			Resolved: shimPathResolved,
		},
	}

	return ccShim, nil
}

func getAgentInfo(config oci.RuntimeConfig) (AgentInfo, error) {
	agentConfig, ok := config.AgentConfig.(vc.HyperConfig)
	if !ok {
		return AgentInfo{}, errors.New("cannot determine agent config")
	}

	agentBinPath := agentConfig.PauseBinPath
	agentBinPathResolved, err := filepath.EvalSymlinks(agentBinPath)
	if err != nil {
		return AgentInfo{}, err
	}

	ccAgent := AgentInfo{
		Type:    string(config.AgentType),
		Version: unknown,
		PauseBin: PathInfo{
			Path:     agentBinPath,
			Resolved: agentBinPathResolved,
		},
	}

	return ccAgent, nil
}

func getEnvInfo(configFile, logfilePath string, config oci.RuntimeConfig) (env EnvInfo, err error) {
	meta := getMetaInfo()

	ccRuntime, err := getRuntimeInfo(configFile, logfilePath, config)
	if err != nil {
		return EnvInfo{}, err
	}

	resolvedHypervisor, err := getHypervisorDetails(config)
	if err != nil {
		return EnvInfo{}, err
	}

	ccHost, err := getHostInfo()
	if err != nil {
		return EnvInfo{}, err
	}

	ccProxy, err := getProxyInfo(config)
	if err != nil {
		return EnvInfo{}, err
	}

	ccShim, err := getShimInfo(config)
	if err != nil {
		return EnvInfo{}, err
	}

	ccAgent, err := getAgentInfo(config)
	if err != nil {
		return EnvInfo{}, err
	}

	hypervisor := PathInfo{
		Path:     config.HypervisorConfig.HypervisorPath,
		Resolved: resolvedHypervisor.HypervisorPath,
	}

	image := PathInfo{
		Path:     config.HypervisorConfig.ImagePath,
		Resolved: resolvedHypervisor.ImagePath,
	}

	kernel := PathInfo{
		Path:     config.HypervisorConfig.KernelPath,
		Resolved: resolvedHypervisor.KernelPath,
	}

	env = EnvInfo{
		Meta:       meta,
		Runtime:    ccRuntime,
		Hypervisor: hypervisor,
		Image:      image,
		Kernel:     kernel,
		Proxy:      ccProxy,
		Shim:       ccShim,
		Agent:      ccAgent,
		Host:       ccHost,
	}

	return env, nil
}

func showSettings(ccEnv EnvInfo, file *os.File) error {

	buf := new(bytes.Buffer)
	encoder := toml.NewEncoder(buf)

	err := encoder.Encode(ccEnv)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(file, "%v", buf.String())

	return err
}

func handleSettings(context *cli.Context) error {
	metadata := context.App.Metadata

	configFile, ok := metadata["configFile"].(string)
	if !ok {
		return errors.New("cannot determine config file")
	}

	runtimeConfig, ok := metadata["runtimeConfig"].(oci.RuntimeConfig)
	if !ok {
		return errors.New("cannot determine runtime config")
	}

	logfilePath, ok := metadata["logfilePath"].(string)
	if !ok {
		return errors.New("cannot determine logfile config")
	}

	ccEnv, err := getEnvInfo(configFile, logfilePath, runtimeConfig)
	if err != nil {
		return err
	}

	return showSettings(ccEnv, os.Stdout)
}

var ccEnvCommand = cli.Command{
	Name:  "cc-env",
	Usage: "display settings",
	Action: func(context *cli.Context) error {
		return handleSettings(context)
	},
}
