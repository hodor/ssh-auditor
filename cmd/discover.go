package cmd

import (
	"bufio"
	"os"

	log "github.com/inconshreveable/log15"
	"github.com/ncsa/ssh-auditor/sshauditor"
	"github.com/spf13/cobra"
)

var ports []int
var exclude []string
var timeoutMs int

var discoverCmd = &cobra.Command{
	Use:     "discover",
	Aliases: []string{"d"},
	Example: "discover -p 22 -p 2222 192.168.1.0/24 10.1.1.0/24 --exclude 192.168.1.100/32",
	Short:   "discover new hosts",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			cmd.Usage()
			return
		}
		timeoutDuration := time.Duration(timeoutMs) * time.Millisecond
		scanConfig := sshauditor.ScanConfiguration{
			Concurrency: concurrency,
			Include:     args,
			Exclude:     exclude,
			Ports:       ports,
			Timeout: timeoutDuration,
		}
		auditor := sshauditor.New(store)
		err := auditor.Discover(scanConfig)
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
	},
}

var discoverFromFileCmd = &cobra.Command{
	Use:     "fromfile",
	Example: "fromfile -p 22 hosts.txt",
	Short:   "discover new hosts using a list of hosts from stdin",
	Run: func(cmd *cobra.Command, args []string) {
		scanner := bufio.NewScanner(os.Stdin)
		timeoutDuration := time.Duration(timeoutMs) * time.Millisecond
		scanConfig := sshauditor.ScanConfiguration{
			Concurrency: concurrency,
			Include:     []string{},
			Ports:       ports,
			Timeout: timeoutDuration,
		}
		for scanner.Scan() {
			host := scanner.Text()
			scanConfig.Include = append(scanConfig.Include, host)
		}
		auditor := sshauditor.New(store)
		err := auditor.Discover(scanConfig)
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	discoverCmd.Flags().IntSliceVarP(&ports, "ports", "p", []int{22}, "ports to check during initial discovery")
	discoverCmd.Flags().StringSliceVarP(&exclude, "exclude", "x", []string{}, "subnets to exclude from discovery")
	discoverCmd.Flags().IntVar(&timeoutMs, "timeout", 4000, "SSH connection timeout in milliseconds")

	discoverFromFileCmd.Flags().IntSliceVarP(&ports, "ports", "p", []int{22}, "ports to check during initial discovery")
	discoverFromFileCmd.Flags().IntVar(&timeoutMs, "timeout", 4000, "SSH connection timeout in milliseconds")
	RootCmd.AddCommand(discoverCmd)
	discoverCmd.AddCommand(discoverFromFileCmd)
}
