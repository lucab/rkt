// Copyright 2014 The rkt Authors
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

//+build linux

package main

import (
	"fmt"
	"strings"

	//	"github.com/appc/spec/schema/types"
	pkgPod "github.com/coreos/rkt/pkg/pod"
	"github.com/coreos/rkt/stage0"
	"github.com/coreos/rkt/store/imagestore"
	"github.com/coreos/rkt/store/treestore"
	//	"github.com/hashicorp/errwrap"
	"github.com/spf13/cobra"
)

var (
	cmdAttach = &cobra.Command{
		Use:   "attach [--app=APPNAME] [--mode=MODE] UUID",
		Short: "Attach to an app running within a rkt pod",

		Long: `UUID should be the UUID of a running pod.`,
		Run:  ensureSuperuser(runWrapper(runAttach)),
	}
	flagAttachMode string
)

func init() {
	cmdRkt.AddCommand(cmdAttach)
	cmdAttach.Flags().StringVar(&flagAppName, "app", "", "name of the app to enter within the specified pod")
	cmdAttach.Flags().StringVar(&flagAttachMode, "mode", "list", "attach mode")

	// Disable interspersed flags to stop parsing after the first non flag
	// argument. This is need to permit to correctly handle
	// multiple "IMAGE -- imageargs ---"  options
	cmdAttach.Flags().SetInterspersed(false)
}

func runAttach(cmd *cobra.Command, args []string) (exit int) {
	if len(args) < 1 {
		cmd.Usage()
		return 254
	}

	uuid := args[0]
	p, err := pkgPod.PodFromUUIDString(getDataDir(), uuid)
	if err != nil {
		stderr.PrintE("problem retrieving pod", err)
		return 254
	}
	defer p.Close()

	if p.State() != pkgPod.Running {
		stderr.Printf("pod %q isn't currently running", p.UUID)
		return 254
	}

	podPID, err := p.ContainerPid1()
	if err != nil {
		stderr.PrintE(fmt.Sprintf("unable to determine the pid for pod %q", p.UUID), err)
		return 254
	}

	appName, err := getAppName(p)
	if err != nil {
		stderr.PrintE("unable to determine app name", err)
		return 254
	}

	s, err := imagestore.NewStore(storeDir())
	if err != nil {
		stderr.PrintE("cannot open store", err)
		return 254
	}

	ts, err := treestore.NewStore(treeStoreDir(), s)
	if err != nil {
		stderr.PrintE("cannot open store", err)
		return 254
	}

	stage1TreeStoreID, err := p.GetStage1TreeStoreID()
	if err != nil {
		stderr.PrintE("error getting stage1 treeStoreID", err)
		return 254
	}

	stage1RootFS := ts.GetRootFS(stage1TreeStoreID)

	attachTTY := false
	attachStdin := false
	attachStdout := false
	attachStderr := false
	muxAction := "list"
	if flagAttachMode != "list" {
		muxAction = "attach"
	} else {
		if flagAttachMode == "auto" {
		}
		if strings.Contains(flagAttachMode, "stdin") {
			attachStdin = true
		}
		if strings.Contains(flagAttachMode, "stdout") {
			attachStdout = true
		}
		if strings.Contains(flagAttachMode, "stderr") {
			attachStderr = true
		}
		if strings.Contains(flagAttachMode, "tty") {
			attachTTY = true
			attachStdin = false
			attachStdout = false
			attachStderr = false
		}
	}

	argv := []string{
		fmt.Sprintf("--action=%s", muxAction),
		fmt.Sprintf("--app=%s", appName.String()),
		fmt.Sprintf("--tty=%t", attachTTY),
		fmt.Sprintf("--stdin=%t", attachStdin),
		fmt.Sprintf("--stdout=%t", attachStdout),
		fmt.Sprintf("--stderr=%t", attachStderr),
	}

	if err = stage0.Attach(p.Path(), podPID, *appName, stage1RootFS, uuid, argv); err != nil {
		stderr.PrintE("enter failed", err)
		return 254
	}
	// not reached when stage0.Attach execs /enter
	return 0
}
