package agent

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/flags"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

const agentDisabledMessage = `trace-agent not enabled.
Set env var DD_APM_ENABLED=true or add
apm_enabled: true
to your datadog.conf file.
Exiting.`

// Run is the entrypoint of our code, which starts the agent.
func Run(ctx context.Context) {
	// Global Agent configuration
	err := common.SetupConfig(flags.ConfigPath)
	if err != nil {
		log.Errorf("Failed to setup config %v", err)
	}

	// configure a default logger before anything so we can observe initialization
	syslogURI := coreconfig.GetSyslogURI()
	logFile := coreconfig.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}

	if coreconfig.Datadog.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	err = coreconfig.SetupLogger(
		coreconfig.Datadog.GetString("log_level"),
		logFile,
		syslogURI,
		coreconfig.Datadog.GetBool("syslog_rfc"),
		coreconfig.Datadog.GetBool("log_to_console"),
		coreconfig.Datadog.GetBool("log_format_json"),
	)
	defer watchdog.LogOnPanic()

	// start CPU profiling
	if flags.CPUProfile != "" {
		f, err := os.Create(flags.CPUProfile)
		if err != nil {
			log.Critical(err)
		}
		pprof.StartCPUProfile(f)
		log.Info("CPU profiling started...")
		defer pprof.StopCPUProfile()
	}

	if flags.Version {
		fmt.Print(info.VersionString())
		return
	}

	if !flags.Info && flags.PIDFilePath != "" {
		err := pidfile.WritePID(flags.PIDFilePath)
		if err != nil {
			log.Errorf("Error while writing PID file, exiting: %v", err)
			os.Exit(1)
		}

		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), flags.PIDFilePath)
		defer func() {
			// remove pidfile if set
			os.Remove(flags.PIDFilePath)
		}()
	}

	cfg, err := config.Load(flags.ConfigPath)
	if err != nil {
		osutil.Exitf("%v", err)
	}
	err = info.InitInfo(cfg) // for expvar & -info option
	if err != nil {
		panic(err)
	}

	if flags.Info {
		if err := info.Info(os.Stdout, cfg); err != nil {
			os.Stdout.WriteString(fmt.Sprintf("failed to print info: %s\n", err))
			os.Exit(1)
		}
		return
	}

	// Exit if tracing is not enabled
	if !cfg.Enabled {
		log.Info(agentDisabledMessage)

		// a sleep is necessary to ensure that supervisor registers this process as "STARTED"
		// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
		// http://supervisord.org/subprocess.html#process-states
		time.Sleep(5 * time.Second)
		return
	}

	// Initialize dogstatsd client
	err = metrics.Configure(cfg, []string{"version:" + info.Version})
	if err != nil {
		osutil.Exitf("cannot configure dogstatsd: %v", err)
	}

	// count the number of times the agent started
	metrics.Count("datadog.trace_agent.started", 1, nil, 1)

	// Seed rand
	rand.Seed(time.Now().UTC().UnixNano())

	ta := NewAgent(ctx, cfg)

	log.Infof("trace-agent running on host %s", cfg.Hostname)
	ta.Run()

	// collect memory profile
	if flags.MemProfile != "" {
		f, err := os.Create(flags.MemProfile)
		if err != nil {
			log.Critical("could not create memory profile: ", err)
		}

		// get up-to-date statistics
		runtime.GC()
		// Not using WriteHeapProfile but instead calling WriteTo to
		// make sure we pass debug=1 and resolve pointers to names.
		if err := pprof.Lookup("heap").WriteTo(f, 1); err != nil {
			log.Critical("could not write memory profile: ", err)
		}
		f.Close()
	}
}
