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
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	rktlog "github.com/coreos/rkt/pkg/log"
	stage1initcommon "github.com/coreos/rkt/stage1/init/common"

	"github.com/appc/spec/schema/types"
	"github.com/coreos/go-systemd/daemon"
	"github.com/kr/pty"
)

var (
	log     *rktlog.Logger
	diag    *rktlog.Logger
	action  string
	appName string
	debug   bool

	stdinMode  string
	stdoutMode string
	stderrMode string
)

const (
	pathPrefix = "/rkt/iottymux"
)

func init() {
	flag.StringVar(&action, "action", "list", "Sub-action to perform")
	flag.StringVar(&appName, "app", "", "Target application name")
}

type Endpoint struct {
	Name     string `json:"name"`
	Family   string `json:"family"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     string `json:"port"`
	Flavor   string `json:"flavor"`
}

func main() {
	var err error
	// Parse flag and initialize logging
	flag.Parse()
	if os.Getenv("STAGE1_DEBUG") == "true" {
		debug = true
	}
	stage1initcommon.InitDebug(debug)
	log, diag, _ = rktlog.NewLogSet("iottymux", debug)
	if !debug {
		diag.SetOutput(ioutil.Discard)
	}

	// Validate app name
	_, err = types.NewACName(appName)
	if err != nil {
		log.Printf("invalid app name (%s): %v", appName, err)
		os.Exit(254)
	}

	var r error
	switch action {
	case "auto-attach":
		r = actionAttach(true)
	case "custom-attach":
		r = actionAttach(false)
	case "iomux":
		statusFile, e := os.OpenFile(filepath.Join(pathPrefix, appName, "endpoints"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
		if e != nil {
			err = e
			break
		}
		r = actionIOMux(statusFile)
	case "ttymux":
		statusFile, e := os.OpenFile(filepath.Join(pathPrefix, appName, "endpoints"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
		if e != nil {
			err = e
			break
		}
		r = actionTTYMux(statusFile)
	case "list":
		fallthrough
	default:
		statusFile, e := os.OpenFile(filepath.Join(pathPrefix, appName, "endpoints"), os.O_RDONLY, os.ModePerm)
		if e != nil {
			err = e
			break
		}
		r = actionPrint(statusFile, os.Stdout)

	}

	if r != nil {
		log.PrintE("runtime failure", r)
		os.Exit(254)
	}
	os.Exit(0)
}

func actionAttach(autoMode bool) error {
	// retrieve available endpoints
	statusFile, err := os.OpenFile(filepath.Join(pathPrefix, appName, "endpoints"), os.O_RDONLY, os.ModePerm)
	if err != nil {
		return err
	}
	eps, err := getEndpoints(statusFile)
	_ = statusFile.Close()
	if err != nil {
		return err
	}

	// retrieve custom attach mode
	var withTTYIn, withTTYOut string
	var withStdin, withStdout, withStderr string
	if !autoMode {
		withTTYIn = os.Getenv("STAGE2_ATTACH_TTYIN")
		withTTYOut = os.Getenv("STAGE2_ATTACH_TTYOUT")
		withStdin = os.Getenv("STAGE2_ATTACH_STDIN")
		withStdout = os.Getenv("STAGE2_ATTACH_STDOUT")
		withStderr = os.Getenv("STAGE2_ATTACH_STDERR")
	}

	ec := make(chan error)
	for s, p := range eps {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", p))
		if err != nil {
			return err
		}
		switch s {
		case "stdin", "tty-in":
			if autoMode || withTTYIn == "true" || withStdin == "true" {
				go io.Copy(conn, os.Stdin)
			}
		case "stdout", "tty-out":
			if autoMode || withTTYOut == "true" || withStdout == "true" {
				go io.Copy(os.Stdout, conn)
			}
		case "stderr":
			if autoMode || withStderr == "true" {
				go io.Copy(os.Stderr, conn)
			}
		case "tty":
			if autoMode || withTTYIn == "true" {
				go io.Copy(conn, os.Stdin)
			}
			if autoMode || withTTYOut == "true" {
				go io.Copy(os.Stdout, conn)
			}
		}
	}
	return <-ec
}

func getEndpoints(in io.Reader) (map[string]string, error) {
	eps := make(map[string]string)
	r := bufio.NewReader(in)
	for {
		line, err := r.ReadString('\n')
		if err == nil {
			fields := strings.Split(line, ",")
			if len(fields) == 4 {
				eps[fields[0]] = strings.Trim(fields[3], "\n")
			}
		}
		if err == io.EOF {
			break
		}
	}
	return eps, nil
}

func actionPrint(in io.Reader, out io.Writer) error {
	eps, err := getEndpoints(in)
	if err != nil {
		return err
	}

	for s, p := range eps {
		out.Write([]byte(fmt.Sprintf("%s available on port %s\n", s, p)))
	}
	return nil
}

func actionTTYMux(statusFile *os.File) error {
	// Create a PTY pair and bind-mount the pts to `/rkt/iottymux/<app>/tty`.
	// Once ready, it will be used by systemd unit.
	pty, tty, err := pty.Open()
	if err != nil {
		return err
	}
	diag.Printf("TTY created at %q", tty.Name())
	ttypath := filepath.Join(pathPrefix, appName, "tty")
	f, err := os.Create(ttypath)
	if err != nil {
		return err
	}
	defer f.Close()
	err = syscall.Mount(tty.Name(), ttypath, "", syscall.MS_BIND, "")
	if err != nil {
		return err
	}

	// signal to systemd that PTY is ready and application can start
	ok, err := daemon.SdNotify("READY=1")
	if !ok {
		return fmt.Errorf("failure during startup notification: %v", err)
	}
	diag.Print("TTY handler ready")

	ttyMode := ""
	if os.Getenv("STAGE2_STDIN") == "true" {
		ttyMode = "tty-in"
	}
	if os.Getenv("STAGE2_STDOUT") == "true" || os.Getenv("STAGE2_STDERR") == "true" {
		if ttyMode == "tty-in" {
			ttyMode = "tty"
		} else {
			ttyMode = "tty-out"
		}

	}
	// Open FIFOs
	var endpoints = make([]*net.Listener, 3)
	var fifos = make([]*os.File, 3)
	if strings.Contains(ttyMode, "tty") {
		fifos[0] = pty
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			log.PrintE("unable to open tty listener", err)
			os.Exit(254)
		}
		endpoints[0] = &l
		port := strconv.Itoa((*endpoints[0]).Addr().(*net.TCPAddr).Port)
		diag.Printf("Listening for TTY on %s", port)

		// TODO(lucab): switch to JSON
		ep := strings.Join([]string{ttyMode, "AF_INET4", "127.0.0.1", port}, ",") + "\n"
		_, _ = statusFile.Write([]byte(ep))
		statusFile.Sync()
		statusFile.Close()

	}
	c := make(chan error)
	if fifos[0] != nil && endpoints[0] != nil {
		clients := make(chan *net.Conn)
		go acceptConn(endpoints[0], clients, "tty")
		go proxyIO(clients, fifos[0])
	}
	return <-c
}

func actionIOMux(statusFile *os.File) error {
	var err error
	var endpoints = make([]*net.Listener, 3)
	var fifos = make([]*os.File, 3)

	logMode := os.Getenv("STAGE1_LOGMODE")
	stdinMode = os.Getenv("STAGE2_STDIN")
	stdoutMode = os.Getenv("STAGE2_STDOUT")
	stderrMode = os.Getenv("STAGE2_STDERR")
	// Open FIFOs
	if stdinMode == "true" {
		fifos[0], err = os.OpenFile(filepath.Join(pathPrefix, appName, "stage2-stdin"), os.O_WRONLY, os.ModeNamedPipe)
		if err != nil {
			log.PrintE("invalid stdin FIFO", err)
			os.Exit(254)
		}
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			log.PrintE("unable to open stdin listener", err)
			os.Exit(254)
		}
		endpoints[0] = &l

		port := strconv.Itoa((*endpoints[0]).Addr().(*net.TCPAddr).Port)
		ep := strings.Join([]string{"stdin", "AF_INET4", "127.0.0.1", port}, ",") + "\n"
		_, _ = statusFile.Write([]byte(ep))
		diag.Printf("Listening for stdin on %s", port)

	}
	if stdoutMode == "true" {
		fifos[1], err = os.OpenFile(filepath.Join(pathPrefix, appName, "stage2-stdout"), os.O_RDONLY, os.ModeNamedPipe)
		if err != nil {
			log.PrintE("invalid stdout FIFO", err)
			os.Exit(254)
		}
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			log.PrintE("unable to open stdout listener", err)
			os.Exit(254)
		}
		endpoints[1] = &l
		port := strconv.Itoa((*endpoints[1]).Addr().(*net.TCPAddr).Port)
		ep := strings.Join([]string{"stdout", "AF_INET4", "127.0.0.1", port}, ",") + "\n"
		_, _ = statusFile.Write([]byte(ep))
		diag.Printf("Listening for stdout on %s", port)

	}
	if stderrMode == "true" {
		fifos[2], err = os.OpenFile(filepath.Join(pathPrefix, appName, "stage2-stderr"), os.O_RDONLY, os.ModeNamedPipe)
		if err != nil {
			log.PrintE("invalid stderr FIFO", err)
			os.Exit(254)
		}
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			log.PrintE("unable to open stdout listener", err)
			os.Exit(254)
		}
		endpoints[2] = &l
		port := strconv.Itoa((*endpoints[2]).Addr().(*net.TCPAddr).Port)
		ep := strings.Join([]string{"stderr", "AF_INET4", "127.0.0.1", port}, ",") + "\n"
		_, _ = statusFile.Write([]byte(ep))
		diag.Printf("Listening for stderr on %s", port)

	}
	statusFile.Sync()
	statusFile.Close()
	c := make(chan error)

	var logFile *os.File
	if logMode == "k8s-plain" {
		var err error
		logFile, err = os.OpenFile(filepath.Join(pathPrefix, appName, "logfile"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
		if err != nil {
			return err
		}
	}
	if fifos[0] != nil && endpoints[0] != nil {
		clients := make(chan *net.Conn)
		go acceptConn(endpoints[0], clients, "stdin")
		go forwardInput(clients, fifos[0])
	}
	if fifos[1] != nil && endpoints[1] != nil {
		localTargets := make(chan io.WriteCloser)
		clients := make(chan *net.Conn)
		lines := make(chan []byte)
		go drainOutput(fifos[1], lines)
		go acceptConn(endpoints[1], clients, "stdout")
		go muxBytes("stdout", lines, clients, localTargets)
		if logFile != nil {
			localTargets <- logFile
		}
	}
	if fifos[2] != nil && endpoints[2] != nil {
		localTargets := make(chan io.WriteCloser)
		clients := make(chan *net.Conn)
		lines := make(chan []byte)
		go drainOutput(fifos[2], lines)
		go acceptConn(endpoints[2], clients, "stderr")
		go muxBytes("stderr", lines, clients, localTargets)
		if logFile != nil {
			localTargets <- logFile
		}
	}

	return <-c
}

func drainOutput(r io.Reader, c chan []byte) {
	rd := bufio.NewReader(r)
	for {
		lineOut, err := rd.ReadBytes('\n')
		if err == nil {
			c <- lineOut
		}
	}
}

func acceptConn(socket *net.Listener, c chan *net.Conn, stream string) {
	for {
		conn, err := (*socket).Accept()
		if err == nil {
			diag.Printf("Accepted new connection for %s", stream)
			c <- &conn
		}
	}
}

func proxyIO(clients chan *net.Conn, tty *os.File) {
	for {
		select {
		case c := <-clients:
			go io.Copy(*c, tty)
			go io.Copy(tty, *c)

		}
	}
}

func forwardInput(clients chan *net.Conn, stdin *os.File) {
	for {
		select {
		case c := <-clients:
			go muxInput(c, stdin)

		}
	}
}

func muxInput(conn *net.Conn, stdin *os.File) {
	rd := bufio.NewReader(*conn)
	for {
		lineIn, err := rd.ReadBytes('\n')
		if err != nil {
			break
		}
		_, err = stdin.Write(lineIn)
		if err != nil {
			break
		}
	}

}

func muxBytes(streamLabel string, lines chan []byte, clients chan *net.Conn, targets chan io.WriteCloser) {
	var logs []io.WriteCloser
	var alivel []bool
	var conns []io.WriteCloser
	var alivec []bool
	for {
		select {
		case l := <-lines:
			for i, f := range logs {
				if alivel[i] == true {
					out := fmt.Sprintf("%s %s %s", time.Now().Format(time.RFC3339Nano), streamLabel, l)
					_, err := f.Write([]byte(out))
					if err != nil {
						f.Close()
						alivel[i] = false
					}
				}
			}
			for i, s := range conns {
				if alivec[i] == true {
					_, err := s.Write(l)
					if err != nil {
						s.Close()
						alivec[i] = false
					}
				}
			}
		case c := <-clients:
			conns = append(conns, *c)
			alivec = append(alivec, true)
		case t := <-targets:
			logs = append(logs, t)
			alivel = append(alivel, true)

		}

	}
}
