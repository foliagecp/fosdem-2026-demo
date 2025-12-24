package main

import (
	"context"
	"os"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m3/common/apps"
	statefun "github.com/foliagecp/sdk/statefun"
	lg "github.com/foliagecp/sdk/statefun/logger"
	"github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

var (
	cn_json_file_updateIntervalSec int    = system.GetEnvMustProceed("CN_FILE_UPDATE_INTERVAL_SEC", 10)
	cn_json_file_filename          string = system.GetEnvMustProceed("CN_FILE_FILENAME", "")
)

func kickerController(ctx context.Context, runtime *statefun.Runtime) {
	go kickJsonFileConnector(ctx, runtime)
}

func kickJsonFileConnector(ctx context.Context, runtime *statefun.Runtime) {
	// Define the polling interval. Ensure it is positive.
	interval := time.Duration(cn_json_file_updateIntervalSec) * time.Second
	if interval <= 0 {
		interval = 1 * time.Second
	}

	// Use a ticker so we can stop waiting immediately when ctx is canceled.
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		cmdUpdateStatus(runtime)

		// Wait for either:
		// 1) context cancellation/deadline, or
		// 2) the next tick.
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// If no filename is configured, skip this iteration to avoid nil dereference.
		if len(cn_json_file_filename) == 0 {
			continue
		}

		// Optional early-exit check (useful if extra work is added above later).
		if ctx.Err() != nil {
			return
		}

		// Read the file contents.
		fileBytes, err := os.ReadFile(cn_json_file_filename)
		if err != nil {
			lg.Logf(lg.ErrorLevel, "%s file read error: %s", cn_json_file_filename, err.Error())
			return
		}

		// Parse JSON from bytes.
		j, ok := easyjson.JSONFromBytes(fileBytes)
		if !ok {
			lg.Logf(lg.ErrorLevel, "%s does not contain a json data", cn_json_file_filename)
			return
		}

		// Use a stable key derived from the filename.
		uuid := system.GetHashStr(cn_json_file_filename)

		payload := easyjson.NewJSONObject()
		payload.SetByPath("json.uuid", easyjson.NewJSON(uuid))
		payload.SetByPath("json.data", j)

		// Send the signal to the runtime.
		runtime.Signal(
			plugins.AutoSignalSelect,
			"function.connector.jsonfile.push_update",
			apps.APP_CN_JSON_FILE,
			&payload,
			nil,
		)
	}
}
