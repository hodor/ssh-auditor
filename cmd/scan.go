package cmd

import (
	"os"
	"time"
	log "github.com/inconshreveable/log15"
	"github.com/ncsa/ssh-auditor/sshauditor"
	"github.com/spf13/cobra"
)

var timeoutScanMs int

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan hosts using new or outdated credentials",
	Run: func(cmd *cobra.Command, args []string) {
		// Convert timeoutScanMs to a time.Duration
        	timeoutDuration := time.Duration(timeoutScanMs) * time.Millisecond
		
		scanConfig := sshauditor.ScanConfiguration{
			Concurrency: concurrency,
			Timeout: timeoutDuration,
		}
		auditor := sshauditor.New(store)
		_, err := auditor.Scan(scanConfig)
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
	},
}

var scanResetIntervalCmd = &cobra.Command{
	Use:     "reset",
	Aliases: []string{"r"},
	Short:   "reset interval",
	Run: func(cmd *cobra.Command, args []string) {
		err := store.ResetInterval()
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	scanCmd.Flags().IntVar(&timeoutScanMs, "timeout", 4000, "SSH connection timeout in milliseconds")
	RootCmd.AddCommand(scanCmd)
	scanCmd.AddCommand(scanResetIntervalCmd)
}
