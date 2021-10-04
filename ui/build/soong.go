// Copyright 2017 Google Inc. All rights reserved.
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

package build

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"android/soong/ui/metrics"
	soong_metrics_proto "android/soong/ui/metrics/metrics_proto"
	"android/soong/ui/status"

	"android/soong/shared"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/deptools"
	"github.com/google/blueprint/microfactory"

	"google.golang.org/protobuf/proto"
)

const (
	availableEnvFile = "soong.environment.available"
	usedEnvFile      = "soong.environment.used"

	soongBuildTag      = "build"
	bp2buildTag        = "bp2build"
	jsonModuleGraphTag = "modulegraph"
	queryviewTag       = "queryview"
	soongDocsTag       = "soong_docs"
)

func writeEnvironmentFile(ctx Context, envFile string, envDeps map[string]string) error {
	data, err := shared.EnvFileContents(envDeps)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(envFile, data, 0644)
}

// This uses Android.bp files and various tools to generate <builddir>/build.ninja.
//
// However, the execution of <builddir>/build.ninja happens later in
// build/soong/ui/build/build.go#Build()
//
// We want to rely on as few prebuilts as possible, so we need to bootstrap
// Soong. The process is as follows:
//
// 1. We use "Microfactory", a simple tool to compile Go code, to build
//    first itself, then soong_ui from soong_ui.bash. This binary contains
//    parts of soong_build that are needed to build itself.
// 2. This simplified version of soong_build then reads the Blueprint files
//    that describe itself and emits .bootstrap/build.ninja that describes
//    how to build its full version and use that to produce the final Ninja
//    file Soong emits.
// 3. soong_ui executes .bootstrap/build.ninja
//
// (After this, Kati is executed to parse the Makefiles, but that's not part of
// bootstrapping Soong)

// A tiny struct used to tell Blueprint that it's in bootstrap mode. It would
// probably be nicer to use a flag in bootstrap.Args instead.
type BlueprintConfig struct {
	toolDir                   string
	soongOutDir               string
	outDir                    string
	runGoTests                bool
	useValidations            bool
	debugCompilation          bool
	subninjas                 []string
	primaryBuilderInvocations []bootstrap.PrimaryBuilderInvocation
}

func (c BlueprintConfig) HostToolDir() string {
	return c.toolDir
}

func (c BlueprintConfig) SoongOutDir() string {
	return c.soongOutDir
}

func (c BlueprintConfig) OutDir() string {
	return c.outDir
}

func (c BlueprintConfig) RunGoTests() bool {
	return c.runGoTests
}

func (c BlueprintConfig) UseValidationsForGoTests() bool {
	return c.useValidations
}

func (c BlueprintConfig) DebugCompilation() bool {
	return c.debugCompilation
}

func (c BlueprintConfig) Subninjas() []string {
	return c.subninjas
}

func (c BlueprintConfig) PrimaryBuilderInvocations() []bootstrap.PrimaryBuilderInvocation {
	return c.primaryBuilderInvocations
}

func environmentArgs(config Config, suffix string) []string {
	return []string{
		"--available_env", shared.JoinPath(config.SoongOutDir(), availableEnvFile),
		"--used_env", shared.JoinPath(config.SoongOutDir(), usedEnvFile+"."+suffix),
	}
}

func writeEmptyGlobFile(ctx Context, path string) {
	err := os.MkdirAll(filepath.Dir(path), 0777)
	if err != nil {
		ctx.Fatalf("Failed to create parent directories of empty ninja glob file '%s': %s", path, err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		err = ioutil.WriteFile(path, nil, 0666)
		if err != nil {
			ctx.Fatalf("Failed to create empty ninja glob file '%s': %s", path, err)
		}
	}
}

func primaryBuilderInvocation(config Config, name string, output string, specificArgs []string) bootstrap.PrimaryBuilderInvocation {
	commonArgs := make([]string, 0, 0)

	if !config.skipSoongTests {
		commonArgs = append(commonArgs, "-t")
	}

	commonArgs = append(commonArgs, "-l", filepath.Join(config.FileListDir(), "Android.bp.list"))

	if os.Getenv("SOONG_DELVE") != "" {
		commonArgs = append(commonArgs, "--delve_listen", os.Getenv("SOONG_DELVE"))
		commonArgs = append(commonArgs, "--delve_path", shared.ResolveDelveBinary())
	}

	allArgs := make([]string, 0, 0)
	allArgs = append(allArgs, specificArgs...)
	allArgs = append(allArgs,
		"--globListDir", name,
		"--globFile", config.NamedGlobFile(name))

	allArgs = append(allArgs, commonArgs...)
	allArgs = append(allArgs, environmentArgs(config, name)...)
	allArgs = append(allArgs, "Android.bp")

	return bootstrap.PrimaryBuilderInvocation{
		Inputs:  []string{"Android.bp"},
		Outputs: []string{output},
		Args:    allArgs,
	}
}

func bootstrapBlueprint(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunSoong, "blueprint bootstrap")
	defer ctx.EndTrace()

	mainSoongBuildExtraArgs := []string{"-o", config.MainNinjaFile()}
	if config.EmptyNinjaFile() {
		mainSoongBuildExtraArgs = append(mainSoongBuildExtraArgs, "--empty-ninja-file")
	}

	mainSoongBuildInvocation := primaryBuilderInvocation(
		config,
		soongBuildTag,
		config.MainNinjaFile(),
		mainSoongBuildExtraArgs)

	if config.bazelBuildMode() == mixedBuild {
		// Mixed builds call Bazel from soong_build and they therefore need the
		// Bazel workspace to be available. Make that so by adding a dependency on
		// the bp2build marker file to the action that invokes soong_build .
		mainSoongBuildInvocation.Inputs = append(mainSoongBuildInvocation.Inputs,
			config.Bp2BuildMarkerFile())
	}

	bp2buildInvocation := primaryBuilderInvocation(
		config,
		bp2buildTag,
		config.Bp2BuildMarkerFile(),
		[]string{
			"--bp2build_marker", config.Bp2BuildMarkerFile(),
		})

	jsonModuleGraphInvocation := primaryBuilderInvocation(
		config,
		jsonModuleGraphTag,
		config.ModuleGraphFile(),
		[]string{
			"--module_graph_file", config.ModuleGraphFile(),
		})

	queryviewInvocation := primaryBuilderInvocation(
		config,
		queryviewTag,
		config.QueryviewMarkerFile(),
		[]string{
			"--bazel_queryview_dir", filepath.Join(config.SoongOutDir(), "queryview"),
		})

	soongDocsInvocation := primaryBuilderInvocation(
		config,
		soongDocsTag,
		config.SoongDocsHtml(),
		[]string{
			"--soong_docs", config.SoongDocsHtml(),
		})

	globFiles := []string{
		config.NamedGlobFile(soongBuildTag),
		config.NamedGlobFile(bp2buildTag),
		config.NamedGlobFile(jsonModuleGraphTag),
		config.NamedGlobFile(queryviewTag),
		config.NamedGlobFile(soongDocsTag),
	}

	// The glob .ninja files are subninja'd. However, they are generated during
	// the build itself so we write an empty file if the file does not exist yet
	// so that the subninja doesn't fail on clean builds
	for _, globFile := range globFiles {
		writeEmptyGlobFile(ctx, globFile)
	}

	var blueprintArgs bootstrap.Args

	blueprintArgs.ModuleListFile = filepath.Join(config.FileListDir(), "Android.bp.list")
	blueprintArgs.OutFile = shared.JoinPath(config.SoongOutDir(), ".bootstrap/build.ninja")
	blueprintArgs.EmptyNinjaFile = false

	blueprintCtx := blueprint.NewContext()
	blueprintCtx.SetIgnoreUnknownModuleTypes(true)
	blueprintConfig := BlueprintConfig{
		soongOutDir:    config.SoongOutDir(),
		toolDir:        config.HostToolDir(),
		outDir:         config.OutDir(),
		runGoTests:     !config.skipSoongTests,
		useValidations: !config.skipSoongTests,
		// If we want to debug soong_build, we need to compile it for debugging
		debugCompilation: os.Getenv("SOONG_DELVE") != "",
		subninjas:        globFiles,
		primaryBuilderInvocations: []bootstrap.PrimaryBuilderInvocation{
			mainSoongBuildInvocation,
			bp2buildInvocation,
			jsonModuleGraphInvocation,
			queryviewInvocation,
			soongDocsInvocation},
	}

	bootstrapDeps := bootstrap.RunBlueprint(blueprintArgs, bootstrap.DoEverything, blueprintCtx, blueprintConfig)
	bootstrapDepFile := shared.JoinPath(config.SoongOutDir(), ".bootstrap/build.ninja.d")
	err := deptools.WriteDepFile(bootstrapDepFile, blueprintArgs.OutFile, bootstrapDeps)
	if err != nil {
		ctx.Fatalf("Error writing depfile '%s': %s", bootstrapDepFile, err)
	}
}

func checkEnvironmentFile(currentEnv *Environment, envFile string) {
	getenv := func(k string) string {
		v, _ := currentEnv.Get(k)
		return v
	}
	if stale, _ := shared.StaleEnvFile(envFile, getenv); stale {
		os.Remove(envFile)
	}
}

func runSoong(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunSoong, "soong")
	defer ctx.EndTrace()

	// We have two environment files: .available is the one with every variable,
	// .used with the ones that were actually used. The latter is used to
	// determine whether Soong needs to be re-run since why re-run it if only
	// unused variables were changed?
	envFile := filepath.Join(config.SoongOutDir(), availableEnvFile)

	dir := filepath.Join(config.SoongOutDir(), ".bootstrap")
	if err := os.MkdirAll(dir, 0755); err != nil {
		ctx.Fatalf("Cannot mkdir " + dir)
	}

	buildMode := config.bazelBuildMode()
	integratedBp2Build := buildMode == mixedBuild

	// This is done unconditionally, but does not take a measurable amount of time
	bootstrapBlueprint(ctx, config)

	soongBuildEnv := config.Environment().Copy()
	soongBuildEnv.Set("TOP", os.Getenv("TOP"))
	// For Bazel mixed builds.
	soongBuildEnv.Set("BAZEL_PATH", "./tools/bazel")
	soongBuildEnv.Set("BAZEL_HOME", filepath.Join(config.BazelOutDir(), "bazelhome"))
	soongBuildEnv.Set("BAZEL_OUTPUT_BASE", filepath.Join(config.BazelOutDir(), "output"))
	soongBuildEnv.Set("BAZEL_WORKSPACE", absPath(ctx, "."))
	soongBuildEnv.Set("BAZEL_METRICS_DIR", config.BazelMetricsDir())

	// For Soong bootstrapping tests
	if os.Getenv("ALLOW_MISSING_DEPENDENCIES") == "true" {
		soongBuildEnv.Set("ALLOW_MISSING_DEPENDENCIES", "true")
	}

	err := writeEnvironmentFile(ctx, envFile, soongBuildEnv.AsMap())
	if err != nil {
		ctx.Fatalf("failed to write environment file %s: %s", envFile, err)
	}

	func() {
		ctx.BeginTrace(metrics.RunSoong, "environment check")
		defer ctx.EndTrace()

		checkEnvironmentFile(soongBuildEnv, filepath.Join(config.SoongOutDir(), usedEnvFile+".build"))

		if integratedBp2Build || config.Bp2Build() {
			checkEnvironmentFile(soongBuildEnv, filepath.Join(config.SoongOutDir(), usedEnvFile+".bp2build"))
		}

		if config.JsonModuleGraph() {
			checkEnvironmentFile(soongBuildEnv, filepath.Join(config.SoongOutDir(), usedEnvFile+".modulegraph"))
		}

		if config.Queryview() {
			checkEnvironmentFile(soongBuildEnv, filepath.Join(config.SoongOutDir(), usedEnvFile+".queryview"))
		}

		if config.SoongDocs() {
			checkEnvironmentFile(soongBuildEnv, filepath.Join(config.SoongOutDir(), usedEnvFile+".soong_docs"))
		}
	}()

	runMicrofactory(ctx, config, filepath.Join(config.HostToolDir(), "bpglob"), "github.com/google/blueprint/bootstrap/bpglob",
		map[string]string{"github.com/google/blueprint": "build/blueprint"})

	ninja := func(name, ninjaFile string, targets ...string) {
		ctx.BeginTrace(metrics.RunSoong, name)
		defer ctx.EndTrace()

		fifo := filepath.Join(config.OutDir(), ".ninja_fifo")
		nr := status.NewNinjaReader(ctx, ctx.Status.StartTool(), fifo)
		defer nr.Close()

		ninjaArgs := []string{
			"-d", "keepdepfile",
			"-d", "stats",
			"-o", "usesphonyoutputs=yes",
			"-o", "preremoveoutputs=yes",
			"-w", "dupbuild=err",
			"-w", "outputdir=err",
			"-w", "missingoutfile=err",
			"-j", strconv.Itoa(config.Parallel()),
			"--frontend_file", fifo,
			"-f", filepath.Join(config.SoongOutDir(), ninjaFile),
		}

		ninjaArgs = append(ninjaArgs, targets...)
		cmd := Command(ctx, config, "soong "+name,
			config.PrebuiltBuildTool("ninja"), ninjaArgs...)

		var ninjaEnv Environment

		// This is currently how the command line to invoke soong_build finds the
		// root of the source tree and the output root
		ninjaEnv.Set("TOP", os.Getenv("TOP"))

		cmd.Environment = &ninjaEnv
		cmd.Sandbox = soongSandbox
		cmd.RunAndStreamOrFatal()
	}

	targets := make([]string, 0, 0)

	if config.JsonModuleGraph() {
		targets = append(targets, config.ModuleGraphFile())
	}

	if config.Bp2Build() {
		targets = append(targets, config.Bp2BuildMarkerFile())
	}

	if config.Queryview() {
		targets = append(targets, config.QueryviewMarkerFile())
	}

	if config.SoongDocs() {
		targets = append(targets, config.SoongDocsHtml())
	}

	if config.SoongBuildInvocationNeeded() {
		// This build generates <builddir>/build.ninja, which is used later by build/soong/ui/build/build.go#Build().
		targets = append(targets, config.MainNinjaFile())
	}

	ninja("bootstrap", ".bootstrap/build.ninja", targets...)

	var soongBuildMetrics *soong_metrics_proto.SoongBuildMetrics
	if shouldCollectBuildSoongMetrics(config) {
		soongBuildMetrics := loadSoongBuildMetrics(ctx, config)
		logSoongBuildMetrics(ctx, soongBuildMetrics)
	}

	distGzipFile(ctx, config, config.SoongNinjaFile(), "soong")

	if !config.SkipKati() {
		distGzipFile(ctx, config, config.SoongAndroidMk(), "soong")
		distGzipFile(ctx, config, config.SoongMakeVarsMk(), "soong")
	}

	if shouldCollectBuildSoongMetrics(config) && ctx.Metrics != nil {
		ctx.Metrics.SetSoongBuildMetrics(soongBuildMetrics)
	}
}

func runMicrofactory(ctx Context, config Config, relExePath string, pkg string, mapping map[string]string) {
	name := filepath.Base(relExePath)
	ctx.BeginTrace(metrics.RunSoong, name)
	defer ctx.EndTrace()
	cfg := microfactory.Config{TrimPath: absPath(ctx, ".")}
	for pkgPrefix, pathPrefix := range mapping {
		cfg.Map(pkgPrefix, pathPrefix)
	}

	exePath := filepath.Join(config.SoongOutDir(), relExePath)
	dir := filepath.Dir(exePath)
	if err := os.MkdirAll(dir, 0777); err != nil {
		ctx.Fatalf("cannot create %s: %s", dir, err)
	}
	if _, err := microfactory.Build(&cfg, exePath, pkg); err != nil {
		ctx.Fatalf("failed to build %s: %s", name, err)
	}
}

func shouldCollectBuildSoongMetrics(config Config) bool {
	// Do not collect metrics protobuf if the soong_build binary ran as the
	// bp2build converter or the JSON graph dump.
	return config.SoongBuildInvocationNeeded()
}

func loadSoongBuildMetrics(ctx Context, config Config) *soong_metrics_proto.SoongBuildMetrics {
	soongBuildMetricsFile := filepath.Join(config.OutDir(), "soong", "soong_build_metrics.pb")
	buf, err := ioutil.ReadFile(soongBuildMetricsFile)
	if err != nil {
		ctx.Fatalf("Failed to load %s: %s", soongBuildMetricsFile, err)
	}
	soongBuildMetrics := &soong_metrics_proto.SoongBuildMetrics{}
	err = proto.Unmarshal(buf, soongBuildMetrics)
	if err != nil {
		ctx.Fatalf("Failed to unmarshal %s: %s", soongBuildMetricsFile, err)
	}
	return soongBuildMetrics
}

func logSoongBuildMetrics(ctx Context, metrics *soong_metrics_proto.SoongBuildMetrics) {
	ctx.Verbosef("soong_build metrics:")
	ctx.Verbosef(" modules: %v", metrics.GetModules())
	ctx.Verbosef(" variants: %v", metrics.GetVariants())
	ctx.Verbosef(" max heap size: %v MB", metrics.GetMaxHeapSize()/1e6)
	ctx.Verbosef(" total allocation count: %v", metrics.GetTotalAllocCount())
	ctx.Verbosef(" total allocation size: %v MB", metrics.GetTotalAllocSize()/1e6)

}
