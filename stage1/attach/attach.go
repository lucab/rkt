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

//+build linux

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	//	"path/filepath"

	//	"github.com/coreos/rkt/common"
	rktlog "github.com/coreos/rkt/pkg/log"
	stage1initcommon "github.com/coreos/rkt/stage1/init/common"

	"github.com/appc/spec/schema/types"
)

var (
	app          string
	action       string
	attachTTY    string
	attachStdin  string
	attachStdout string
	attachStderr string
	debug        bool
	log          *rktlog.Logger
	diag         *rktlog.Logger
)

func init() {
	flag.BoolVar(&debug, "debug", false, "Run in debug mode")
	flag.StringVar(&action, "action", "list", "Action")
	flag.StringVar(&app, "app", "", "Application name")
	flag.StringVar(&attachTTY, "tty", "false", "attach tty")
	flag.StringVar(&attachStdin, "stdin", "true", "attach stdin")
	flag.StringVar(&attachStdout, "stdout", "true", "attach stdin")
	flag.StringVar(&attachStderr, "stderr", "false", "attach tty")
}

func main() {
	flag.Parse()

	stage1initcommon.InitDebug(debug)

	log, diag, _ = rktlog.NewLogSet("stage1-attach", debug)
	if !debug {
		diag.SetOutput(ioutil.Discard)
	}

	appName, err := types.NewACName(app)
	if err != nil {
		log.PrintE("invalid app name", err)
		os.Exit(254)
	}

	var args []string
	enterCmd := os.Getenv("RKT_STAGE1_ENTERCMD")
	enterPID := os.Getenv("RKT_STAGE1_ENTERPID")
	if enterCmd != "" {
		args = append(args, []string{enterCmd, fmt.Sprintf("--pid=%s", enterPID), "--"}...)
	}

	args = append(args, "/iottymux")
	args = append(args, fmt.Sprintf("--action=%s", action))

	cmd := exec.Cmd{
		Path:   args[0],
		Args:   args,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Env: []string{
			fmt.Sprintf("STAGE2_APPNAME=%s", appName),
			fmt.Sprintf("STAGE2_TTY=%s", attachTTY),
			fmt.Sprintf("STAGE2_STDIN=%s", attachStdin),
			fmt.Sprintf("STAGE2_STDOUT=%s", attachStdout),
			fmt.Sprintf("STAGE2_STDERR=%s", attachStderr),
		},
	}

	if err := cmd.Run(); err != nil {
		log.PrintE(`error executing "iottymux"`, err)
		os.Exit(254)
	}

	os.Exit(0)
}
