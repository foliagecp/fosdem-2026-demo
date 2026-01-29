package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2/common/apps"
	"github.com/foliagecp/fosdem-2026-demo/m2/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	graphCRUD "github.com/foliagecp/sdk/embedded/graph/crud"
	graphDebug "github.com/foliagecp/sdk/embedded/graph/debug"
	"github.com/foliagecp/sdk/embedded/graph/fpl"
	"github.com/foliagecp/sdk/embedded/graph/jpgql"
	"github.com/foliagecp/sdk/embedded/graph/search"
	"github.com/foliagecp/sdk/statefun"
	"github.com/foliagecp/sdk/statefun/cache"
	lg "github.com/foliagecp/sdk/statefun/logger"
	"github.com/foliagecp/sdk/statefun/system"
	uilib "github.com/foliagecp/ui-app-lib"
)

var (
	// natsURL - nats server url
	natsURL = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
)

func healthyState(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "main is healthy")
	})

	srv := &http.Server{
		Addr:    ":9000",
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		// http.ErrServerClosed - normal error during Shutdown
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		// give some time for active requests to complete correctly
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = srv.Shutdown(shutdownCtx) // you can handle the error if it's important
		return ctx.Err()

	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func cmdUpdateStatus(runtime *statefun.Runtime) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		lg.Logf(lg.ErrorLevel, "cannot update cmd status: %v", err)
	}

	t := time.Now()

	data := easyjson.NewJSONObject()
	data.SetByPath("updated_at.datetime", easyjson.NewJSON(t.Format("2006-01-02 15:04:05 MST")))
	data.SetByPath("updated_at.nano", easyjson.NewJSON(t.UnixNano()))

	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_CMD, data, false, types.TYPE_FOLIAGE_APP_CMD))
}

func onAfterStart(ctx context.Context, runtime *statefun.Runtime) error {
	runtime.Domain.SetWeakClusterDomains([]string{"m1", "m3", "m4"})

	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_APP_CMD, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_CMD, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_APP_CMD))

	go healthyState(ctx)

	return nil
}

func registerFunctionTypes(runtime *statefun.Runtime) {
	graphCRUD.RegisterAllFunctionTypes(runtime)
	graphDebug.RegisterAllFunctionTypes(runtime)
	jpgql.RegisterAllFunctionTypes(runtime)
	fpl.RegisterAllFunctionTypes(runtime)
	search.RegisterAllFunctionTypes(runtime)
	uilib.RegisterAllFunctions(runtime)
}

func start() {
	system.GlobalPrometrics = system.NewPrometrics("", ":9901")
	if runtime, err := statefun.NewRuntime(*statefun.NewRuntimeConfigSimple(natsURL, apps.APP_CMD).UseJSDomainAsHubDomainName().SetDomainRoutersHandling(false)); err == nil {
		registerFunctionTypes(runtime)
		runtime.RegisterOnAfterStartFunction(onAfterStart, false)
		if err := runtime.Start(context.Background(), cache.NewCacheConfig("graph_cache")); err != nil {
			lg.Logf(lg.ErrorLevel, "Cannot start due to an error: %s", err)
		}
	} else {
		lg.Logf(lg.ErrorLevel, "Cannot create statefun runtime due to an error: %s", err)
	}
}

func main() {
	helpFlag := flag.Bool("h", false, "Show help message")
	helpFlagAlias := flag.Bool("help", false, "Show help message (alias)")
	logLevelFlag := flag.Int("ll", int(lg.InfoLevel), "Log level (0-6): panic, fatal, error, warn, info, debug, trace")
	logReportCallerFlag := flag.Bool("lrp", false, "Log report caller shows file name and line number where log originates from")

	flag.Parse()

	if *helpFlag || *helpFlagAlias {
		fmt.Println("usage: foliage [option]")
		fmt.Println("Options:")
		flag.PrintDefaults()
		return
	}

	lg.SetDefaultOptions(
		os.Stdout,
		// subtract and multiply, because each level has a factor of 4: -8, -4, 0, 4, 8, 12, 16
		lg.LogLevel((4-*logLevelFlag)*4),
		*logReportCallerFlag,
	)

	start()
}
