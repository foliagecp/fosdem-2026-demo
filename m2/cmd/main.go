package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2"
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

const (
	runtimeName = "main"
)

var (
	// natsURL - nats server url
	natsURL = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
)

func startHealthyState(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "main is healthy")
	})

	srv := &http.Server{
		Addr:        ":9000",
		Handler:     mux,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			lg.GetLogger().Errorf(ctx, "Health server error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			lg.GetLogger().Errorf(ctx, "Health server shutdown failed: %v", err)
		}
	}()
}

func onAfterStart(ctx context.Context, runtime *statefun.Runtime) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.TYPE_FOLIAGE_APP_CMD, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(m2.APP_CMD, easyjson.NewJSONObject(), false, m2.TYPE_FOLIAGE_APP_CMD))

	startHealthyState(ctx)

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
	if runtime, err := statefun.NewRuntime(*statefun.NewRuntimeConfigSimple(natsURL, runtimeName).UseJSDomainAsHubDomainName()); err == nil {
		registerFunctionTypes(runtime)
		runtime.RegisterOnAfterStartFunction(onAfterStart, false)
		if err := runtime.Start(context.TODO(), cache.NewCacheConfig("main_cache")); err != nil {
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
