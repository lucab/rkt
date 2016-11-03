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

package main

import (
	"fmt"
	"strings"

	"github.com/coreos/rkt/common"
	pkgPod "github.com/coreos/rkt/pkg/pod"
	"github.com/coreos/rkt/stage0"
	"github.com/coreos/rkt/store/imagestore"
	"github.com/coreos/rkt/store/treestore"
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
	if common.IsExperimentEnabled("attach") {
		cmdRkt.AddCommand(cmdAttach)
		cmdAttach.Flags().StringVar(&flagAppName, "app", "", "name of the app to enter within the specified pod")
		cmdAttach.Flags().StringVar(&flagAttachMode, "mode", "list", "attach mode")
	}
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
	attachArgs, err := parseAttachMode(appName.String(), globalFlags.Debug, flagAttachMode)
	if err != nil {
		stderr.PrintE("invalid attach mode", err)
		return 254
	}

	if err = stage0.Attach(p.Path(), podPID, *appName, stage1RootFS, uuid, attachArgs); err != nil {
		stderr.PrintE("enter failed", err)
		return 254
	}
	// not reached when stage0.Attach execs /enter
	return 0
}

// parseAttachMode parses a stage0 CLI "--mode" and returns options suited
// for stage1/attach entrypoint invocation
func parseAttachMode(appName string, debug bool, attachMode string) ([]string, error) {
	attachArgs := []string{
		fmt.Sprintf("--app=%s", appName),
		fmt.Sprintf("--debug=%t", debug),
	}

	// list mode: just print endpoints
	if attachMode == "list" || attachMode == "" {
		attachArgs = append(attachArgs, "--action=list")
		return attachArgs, nil
	}

	// auto-attach mode: stage1-attach will figure out endpoints
	if attachMode == "auto" {
		attachArgs = append(attachArgs, "--action=auto-attach")
		return attachArgs, nil
	}

	// custom-attach mode: user specified channels
	attachArgs = append(attachArgs, "--action=custom-attach")

	// check for tty-attaching modes
	attachTTYIn := false
	attachTTYOut := false
	if attachMode == "tty" {
		attachTTYIn = true
		attachTTYOut = true
	}
	if strings.Contains(attachMode, "tty-in") {
		attachTTYIn = true
	}
	if strings.Contains(attachMode, "tty-out") {
		attachTTYOut = true
	}

	// check for stream-attaching modes
	attachStdin := false
	attachStdout := false
	attachStderr := false
	if strings.Contains(attachMode, "stdin") {
		attachStdin = true
	}
	if strings.Contains(attachMode, "stdout") {
		attachStdout = true
	}
	if strings.Contains(attachMode, "stderr") {
		attachStderr = true
	}

	// check that the resulting attach mode is sane
	if !(attachTTYIn || attachTTYOut || attachStdin || attachStdout || attachStderr) {
		return nil, fmt.Errorf("mode must specify at least one endpoint to attach")
	}
	if (attachTTYIn || attachTTYOut) && (attachStdin || attachStdout || attachStderr) {
		return nil, fmt.Errorf("incompatibles endpoints %q", attachMode)
	}

	attachArgs = append(attachArgs, []string{
		fmt.Sprintf("--tty-in=%t", attachTTYIn),
		fmt.Sprintf("--tty-out=%t", attachTTYOut),
		fmt.Sprintf("--stdin=%t", attachStdin),
		fmt.Sprintf("--stdout=%t", attachStdout),
		fmt.Sprintf("--stderr=%t", attachStderr),
	}...)
	return attachArgs, nil

}
