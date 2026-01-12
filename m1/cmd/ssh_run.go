package main

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/util"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
	"golang.org/x/crypto/ssh"
)

const (
	sshRunFoliageFunctionName = "function.cmd.ssh.run"
)

// Payload schema (minimal, extensible):
//
//	{
//	  "target": {"ip": "192.168.157.12", "port": 22},
//	  "auth": {"user": "demo", "password": "demo"},
//	  "exec": {"path": "/opt/scripts/fix_kvm_libvirtd.sh", "args": []},
//	  "result": {"uuid": "optional-fixed-uuid"}
//	}
func sshRun(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "ssh.run cannot create db client")
		return
	}

	ip, ok := ctx.Payload.GetByPath("target.ip").AsString()
	if !ok || strings.TrimSpace(ip) == "" {
		lg.Logln(lg.ErrorLevel, "ssh.run: missing target.ip")
		return
	}
	port := int(ctx.Payload.GetByPath("target.port").AsNumericDefault(22))
	if port <= 0 {
		port = 22
	}

	user := ctx.Payload.GetByPath("auth.user").AsStringDefault("demo")
	password := ctx.Payload.GetByPath("auth.password").AsStringDefault("demo")
	privateKeyStr, _ := ctx.Payload.GetByPath("auth.private_key").AsString()

	path, ok := ctx.Payload.GetByPath("exec.path").AsString()
	if !ok || strings.TrimSpace(path) == "" {
		lg.Logln(lg.ErrorLevel, "ssh.run: missing exec.path")
		return
	}

	args := []string{}
	if arr, ok := ctx.Payload.GetByPath("exec.args").AsArray(); ok {
		for _, v := range arr {
			j := easyjson.NewJSON(v)
			if s, ok := j.AsString(); ok {
				args = append(args, s)
			}
		}
	}

	resultUUID, _ := ctx.Payload.GetByPath("result.uuid").AsString()
	if strings.TrimSpace(resultUUID) == "" {
		resultUUID = system.GetHashStr(fmt.Sprintf("%s:%d:%s", ip, time.Now().UnixNano(), path))
	}

	timeoutSec := int(ctx.Payload.GetByPath("exec.timeout_sec").AsNumericDefault(10))
	if timeoutSec <= 0 {
		timeoutSec = 10
	}

	cmd := buildShellCmd(path, args)
	stdout, stderr, runErr := sshExec(ip, port, user, password, privateKeyStr, cmd, time.Duration(timeoutSec)*time.Second)

	res := easyjson.NewJSONObject()
	res.SetByPath("target.ip", easyjson.NewJSON(ip))
	res.SetByPath("target.port", easyjson.NewJSON(port))
	res.SetByPath("auth.user", easyjson.NewJSON(user))
	res.SetByPath("exec.path", easyjson.NewJSON(path))
	res.SetByPath("exec.args", easyjson.NewJSON(args))
	res.SetByPath("exec.cmd", easyjson.NewJSON(cmd))
	res.SetByPath("result.stdout", easyjson.NewJSON(string(stdout)))
	res.SetByPath("result.stderr", easyjson.NewJSON(string(stderr)))
	if runErr != nil {
		res.SetByPath("result.ok", easyjson.NewJSON(false))
		res.SetByPath("result.error", easyjson.NewJSON(runErr.Error()))
	} else {
		res.SetByPath("result.ok", easyjson.NewJSON(true))
	}
	res.SetByPath("result.timestamp_unix_nano", easyjson.NewJSON(time.Now().UnixNano()))

	// Persist result for the UI/graph inspection.
	if err := dbc.CMDB.ObjectUpdate(resultUUID, res, true, types.TYPE_FOLIAGE_CMD_ACTION_RESULT); err != nil {
		lg.Logf(lg.ErrorLevel, "ssh.run: cannot store result %s: %v", resultUUID, err)
	}

	// Best-effort link to corresponding server object if it exists.
	hostID := util.HostIDFromIP(ip)
	_ = dbc.CMDB.ObjectsLinkUpdate(hostID, resultUUID, nil, easyjson.NewJSONObject(), false, "ssh_action")
}

func buildShellCmd(path string, args []string) string {
	parts := append([]string{path}, args...)
	for i := range parts {
		parts[i] = shellEscape(parts[i])
	}
	return strings.Join(parts, " ")
}

func shellEscape(s string) string {
	// POSIX-ish single-quote escaping.
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\r\"'\\$") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\"+"'"+"'") + "'"
}

func sshExec(ip string, port int, user, password, privateKeyStr, cmd string, timeout time.Duration) ([]byte, []byte, error) {
	addr := fmt.Sprintf("%s:%d", ip, port)
	auth := []ssh.AuthMethod{}
	if strings.TrimSpace(privateKeyStr) != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKeyStr))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid private_key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	} else {
		auth = append(auth, ssh.Password(password))
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, nil, err
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return nil, nil, err
	}
	defer sess.Close()

	var outb, errb bytes.Buffer
	sess.Stdout = &outb
	sess.Stderr = &errb

	err = sess.Run(cmd)
	return outb.Bytes(), errb.Bytes(), err
}
