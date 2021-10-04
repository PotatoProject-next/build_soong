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

package bp2build

import (
	"android/soong/android"
	"android/soong/apex"
	"android/soong/cc"
	"android/soong/java"

	"testing"
)

func runApexTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, registerApexModuleTypes, tc)
}

func registerApexModuleTypes(ctx android.RegistrationContext) {
	// CC module types needed as they can be APEX dependencies
	cc.RegisterCCBuildComponents(ctx)

	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	ctx.RegisterModuleType("apex_key", apex.ApexKeyFactory)
	ctx.RegisterModuleType("android_app_certificate", java.AndroidAppCertificateFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
}

func TestApexBundleSimple(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                        "apex - simple example",
		moduleTypeUnderTest:                "apex",
		moduleTypeUnderTestFactory:         apex.BundleFactory,
		moduleTypeUnderTestBp2BuildMutator: apex.ApexBundleBp2Build,
		filesystem:                         map[string]string{},
		blueprint: `
apex_key {
        name: "com.android.apogee.key",
        public_key: "com.android.apogee.avbpubkey",
        private_key: "com.android.apogee.pem",
	bazel_module: { bp2build_available: false },
}

android_app_certificate {
        name: "com.android.apogee.certificate",
        certificate: "com.android.apogee",
        bazel_module: { bp2build_available: false },
}

cc_library {
        name: "native_shared_lib_1",
	bazel_module: { bp2build_available: false },
}

cc_library {
        name: "native_shared_lib_2",
	bazel_module: { bp2build_available: false },
}

// TODO(b/194878861): Add bp2build support for prebuilt_etc
cc_library {
        name: "pretend_prebuilt_1",
        bazel_module: { bp2build_available: false },
}

// TODO(b/194878861): Add bp2build support for prebuilt_etc
cc_library {
        name: "pretend_prebuilt_2",
        bazel_module: { bp2build_available: false },
}

filegroup {
	name: "com.android.apogee-file_contexts",
        srcs: [
                "com.android.apogee-file_contexts",
        ],
        bazel_module: { bp2build_available: false },
}

apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
	androidManifest: "ApogeeAndroidManifest.xml",
        file_contexts: "com.android.apogee-file_contexts",
	min_sdk_version: "29",
	key: "com.android.apogee.key",
	certificate: "com.android.apogee.certificate",
	updatable: false,
	installable: false,
	native_shared_libs: [
	    "native_shared_lib_1",
	    "native_shared_lib_2",
	],
	binaries: [
            "binary_1",
	    "binary_2",
	],
	prebuilts: [
	    "pretend_prebuilt_1",
	    "pretend_prebuilt_2",
	],
}
`,
		expectedBazelTargets: []string{`apex(
    name = "com.android.apogee",
    android_manifest = "ApogeeAndroidManifest.xml",
    binaries = [
        "binary_1",
        "binary_2",
    ],
    certificate = ":com.android.apogee.certificate",
    file_contexts = ":com.android.apogee-file_contexts",
    installable = False,
    key = ":com.android.apogee.key",
    manifest = "apogee_manifest.json",
    min_sdk_version = "29",
    native_shared_libs = [
        ":native_shared_lib_1",
        ":native_shared_lib_2",
    ],
    prebuilts = [
        ":pretend_prebuilt_1",
        ":pretend_prebuilt_2",
    ],
    updatable = False,
)`}})
}

func TestApexBundleDefaultPropertyValues(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                        "apex - default property values",
		moduleTypeUnderTest:                "apex",
		moduleTypeUnderTestFactory:         apex.BundleFactory,
		moduleTypeUnderTestBp2BuildMutator: apex.ApexBundleBp2Build,
		filesystem:                         map[string]string{},
		blueprint: `
apex {
	name: "com.android.apogee",
	manifest: "apogee_manifest.json",
}
`,
		expectedBazelTargets: []string{`apex(
    name = "com.android.apogee",
    manifest = "apogee_manifest.json",
)`}})
}

func TestApexBundleHasBazelModuleProps(t *testing.T) {
	runApexTestCase(t, bp2buildTestCase{
		description:                        "apex - has bazel module props",
		moduleTypeUnderTest:                "apex",
		moduleTypeUnderTestFactory:         apex.BundleFactory,
		moduleTypeUnderTestBp2BuildMutator: apex.ApexBundleBp2Build,
		filesystem:                         map[string]string{},
		blueprint: `
apex {
	name: "apogee",
	manifest: "manifest.json",
	bazel_module: { bp2build_available: true },
}
`,
		expectedBazelTargets: []string{`apex(
    name = "apogee",
    manifest = "manifest.json",
)`}})
}
