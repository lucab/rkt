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

package stage0

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/appc/spec/schema/types"
	"github.com/coreos/rkt/common"
	"github.com/hashicorp/errwrap"
)

func Attach(cdir string, podPID int, appName types.ACName, stage1Path string, uuid string, args []string) error {
	if err := runCrossingEntrypoint(cdir, podPID, appName.String(), attachEntrypoint, args); err != nil {
		return err
	}

	return nil
}

func runCrossingEntrypoint(dir string, podPID int, appName string, entrypoint string, entrypointArgs []string) error {
	enterCmd, err := getStage1Entrypoint(dir, enterEntrypoint)
	if err != nil {
		return errwrap.Wrap(errors.New("error determining 'enter' entrypoint"), err)
	}

	previousDir, err := os.Getwd()
	if err != nil {
		return err
	}

	debug("Pivoting to filesystem %s", dir)
	if err := os.Chdir(dir); err != nil {
		return errwrap.Wrap(errors.New("failed changing to dir"), err)
	}

	ep, err := getStage1Entrypoint(dir, entrypoint)
	if err != nil {
		return fmt.Errorf("%q not implemented for pod's stage1: %v", entrypoint, err)
	}
	execArgs := []string{filepath.Join(common.Stage1RootfsPath(dir), ep)}
	execArgs = append(execArgs, entrypointArgs...)

	c := exec.Cmd{
		Path:   execArgs[0],
		Args:   execArgs,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Env: []string{
			fmt.Sprintf("RKT_STAGE1_ENTERCMD=%s", filepath.Join(common.Stage1RootfsPath(dir), enterCmd)),
			fmt.Sprintf("RKT_STAGE1_ENTERPID=%d", podPID),
			fmt.Sprintf("RKT_STAGE1_ENTERAPPNAME=%s", appName),
		},
	}

	if err := c.Run(); err != nil {
		return fmt.Errorf("error executing stage1 entrypoint: %v", err)
	}

	if err := os.Chdir(previousDir); err != nil {
		return errwrap.Wrap(errors.New("failed changing to dir"), err)
	}

	return nil
}
