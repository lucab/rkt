// Copyright 2016 The rkt Authors
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

// +build host coreos src kvm

package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/coreos/rkt/tests/testutils"
)

func TestMountNSApp(t *testing.T) {
	image := patchTestACI("rkt-test-mount-ns-app.aci", "--exec=/inspect --check-mountns", "--capability=CAP_SYS_PTRACE")
	defer os.Remove(image)

	ctx := testutils.NewRktRunCtx()
	defer ctx.Cleanup()

	rktCmd := fmt.Sprintf("%s --insecure-options=image run %s", ctx.Cmd(), image)

	expectedLine := "check-mountns: DIFFERENT"
	runRktAndCheckOutput(t, rktCmd, expectedLine, false)
}

func TestSharedSlave(t *testing.T) {
}

// TestProcFSRestrictions checks that access to sensitive paths under
// /proc and /sys is correctly restricted:
// https://github.com/coreos/rkt/issues/2484
func TestProcFSRestrictions(t *testing.T) {
	// check access to read-only paths
	roEntry := "/proc/sysrq-trigger"
	testContent := "h"
	roImage := patchTestACI("rkt-inspect-write-procfs.aci", fmt.Sprintf("--exec=/inspect --write-file --file-name %s --content %s", roEntry, testContent))
	defer os.Remove(roImage)

	roCtx := testutils.NewRktRunCtx()
	defer roCtx.Cleanup()

	roCmd := fmt.Sprintf("%s --debug --insecure-options=image run %s", roCtx.Cmd(), roImage)

	roExpectedLine := fmt.Sprintf("Cannot write to file \"%s\"", roEntry)
	runRktAndCheckOutput(t, roCmd, roExpectedLine, true)

	// check access to inaccessible paths
	hiddenEntry := "/sys/firmware/"
	hiddenImage := patchTestACI("rkt-inspect-stat-procfs.aci", fmt.Sprintf("--exec=/inspect --stat-file --file-name %s", hiddenEntry))
	defer os.Remove(hiddenImage)

	hiddenCtx := testutils.NewRktRunCtx()
	defer hiddenCtx.Cleanup()

	hiddenCmd := fmt.Sprintf("%s --insecure-options=image run %s", hiddenCtx.Cmd(), hiddenImage)

	hiddenExpectedLine := fmt.Sprintf("%s: mode: d---------", hiddenEntry)
	runRktAndCheckOutput(t, hiddenCmd, hiddenExpectedLine, false)
}
