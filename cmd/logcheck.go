package cmd

import (
	"os"
	"time"
	log "github.com/inconshreveable/log15"
	"github.com/hodor/ssh-auditor/sshauditor"
	"github.com/spf13/cobra"
)

var logcheckCmd = &cobra.Command{
	Use:     "logcheck",
	Short:   "trigger and report on failed ssh authentication attempts",
	Aliases: []string{"lc"},
}

var timeoutLogCheckMs int

var logcheckRunCmd = &cobra.Command{
	Use:   "run",
	Short: "trigger failed ssh authentication attempts",
	Long: `trigger failed ssh authentication attempts in order to verify that
		local servers are properly shipping logs to a central collector`,
	Run: func(cmd *cobra.Command, args []string) {
		auditor := sshauditor.New(store)
		timeoutDuration := time.Duration(timeoutLogCheckMs) * time.Millisecond
		scanConfig := sshauditor.ScanConfiguration{
			Concurrency: concurrency,
			Timeout: timeoutDuration,
		}
		err := auditor.Logcheck(scanConfig)
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
	},
}

var splunkHost string

var logcheckReportCmd = &cobra.Command{
	Use:     "report",
	Aliases: []string{"lc"},
	Short:   "compare syslog data to the store",
	Long: `After running logcheck, search syslog for failed login attempts in
		order to determine which hosts are properly logging to syslog`,
	Run: func(cmd *cobra.Command, args []string) {
		var ls sshauditor.LogSearcher
		if splunkHost != "" {
			ls = sshauditor.NewSplunkLogSearcher(splunkHost)
		} else {
			log.Error("only --splunk supported for now")
			os.Exit(1)
		}
		auditor := sshauditor.New(store)
		err := auditor.LogcheckReport(ls)
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	RootCmd.AddCommand(logcheckCmd)

	logcheckCmd.AddCommand(logcheckRunCmd)

	logcheckReportCmd.Flags().StringVar(&splunkHost, "splunk", "", "base url to splunk API (https://host:port)")
	logcheckReportCmd.Flags().IntVar(&timeoutLogCheckMs, "timeout", 4000, "SSH connection timeout in milliseconds")
	logcheckCmd.AddCommand(logcheckReportCmd)
}
