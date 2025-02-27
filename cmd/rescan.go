package cmd

import (
	"os"
	"time"
	log "github.com/inconshreveable/log15"
	"github.com/ncsa/ssh-auditor/sshauditor"
	"github.com/spf13/cobra"
)

var timeoutRescanMs int

var rescanCmd = &cobra.Command{
	Use:   "rescan",
	Short: "Rescan hosts with credentials that have previously worked",
	Run: func(cmd *cobra.Command, args []string) {
		timeoutDuration := time.Duration(timeoutRescanMs) * time.Millisecond
		
		scanConfig := sshauditor.ScanConfiguration{
			Concurrency: concurrency,
			Timeout: timeoutDuration,
		}
		auditor := sshauditor.New(store)
		_, err := auditor.Rescan(scanConfig)
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	rescanCmd.Flags().IntVar(&timeoutRescanMs, "timeout", 4000, "SSH connection timeout in milliseconds")
	RootCmd.AddCommand(rescanCmd)
}
