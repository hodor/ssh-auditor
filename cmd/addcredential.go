package cmd

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"os"
	"strconv"

	log "github.com/inconshreveable/log15"

	"github.com/hodor/ssh-auditor/sshauditor"
	"github.com/spf13/cobra"
)

var credentialCmd = &cobra.Command{
	Use:     "credential",
	Short:   "manage credentials",
	Aliases: []string{"cred", "c"},
}

var scanIntervalDays int

var credentialAddCmd = &cobra.Command{
	Use:     "add",
	Aliases: []string{"addcredential", "ac", "add"},
	Short:   "add a new credential pair",
	Example: "add root root123",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 2 {
			cmd.Usage()
			return
		}
		cred := sshauditor.Credential{
			User:         args[0],
			Password:     args[1],
			ScanInterval: scanIntervalDays,
		}
		l := log.New("user", cred.User, "password", cred.Password, "interval", scanIntervalDays)
		added, err := store.AddCredential(cred)
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
		if added {
			l.Info("added credential")
		} else {
			l.Info("updated credential")
		}
	},
}
var credentialListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "list credentials",
	Run: func(cmd *cobra.Command, args []string) {
		creds, err := store.GetAllCreds()
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
		w := json.NewEncoder(os.Stdout)
		for _, c := range creds {
			if err := w.Encode(c); err != nil {
				panic(err)
			}
		}
	},
}

var credentialResetCmd = &cobra.Command{
	Use:     "reset",
	Aliases: []string{"c"},
	Short:   "reset credential list",
	Run: func(cmd *cobra.Command, args []string) {
		err := store.ResetCreds()
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
	},
}

var credentialImportCmd = &cobra.Command{
	Use:   "import",
	Short: "load credentials from TSV or JSON",
}

var credentialImportTSVCmd = &cobra.Command{
	Use:   "tsv",
	Short: "load credentials from TSV",
	Long: `Load credentials from stdin in the format of
user	password	scaninterval

or
user	password

example:

root	root	7
test	test
`,
	Run: func(cmd *cobra.Command, args []string) {
		reader := csv.NewReader(os.Stdin)
		reader.Comma = '\t'
		records, err := reader.ReadAll()
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
		store.Begin()
		defer store.Commit()

		for _, r := range records {
			var si int
			if len(r) != 2 && len(r) != 3 {
				log.Error("Invalid record", "rec", r)
				continue
			}
			if len(r) == 3 {
				si, err = strconv.Atoi(r[2])
				if err != nil {
					log.Error("Invalid record", "rec", r, "err", err)
					continue
				}
			} else {
				si = scanIntervalDays
			}

			cred := sshauditor.Credential{
				User:         r[0],
				Password:     r[1],
				ScanInterval: si,
			}
			l := log.New("user", cred.User, "password", cred.Password, "interval", si)
			added, err := store.AddCredential(cred)
			if err != nil {
				log.Error(err.Error())
				os.Exit(1)
			}
			if added {
				l.Info("added credential")
			} else {
				l.Info("updated credential")
			}
		}
	},
}
var credentialImportJSONCmd = &cobra.Command{
	Use:   "json",
	Short: "load credentials from JSON",
	Long: `Load credentials from stdin in the format of
{"User":"root","Password":"root","ScanInterval":7}
{"User":"test","Password":"test"}
`,
	Run: func(cmd *cobra.Command, args []string) {
		store.Begin()
		defer store.Commit()
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var cred sshauditor.Credential
			json.Unmarshal(scanner.Bytes(), &cred)
			if cred.ScanInterval == 0 {
				cred.ScanInterval = scanIntervalDays
			}
			l := log.New("user", cred.User, "password", cred.Password, "interval", cred.ScanInterval)
			added, err := store.AddCredential(cred)
			if err != nil {
				log.Error(err.Error())
				os.Exit(1)
			}
			if added {
				l.Info("added credential")
			} else {
				l.Info("updated credential")
			}
		}
		if err := scanner.Err(); err != nil {
			log.Error("error reading standard input", "err", err)
		}
	},
}

func init() {
	credentialAddCmd.Flags().IntVar(&scanIntervalDays, "scan-interval", 14, "How often to re-scan for this credential, in days")
	RootCmd.AddCommand(credentialAddCmd)
	RootCmd.AddCommand(credentialCmd)
	credentialCmd.AddCommand(credentialAddCmd)
	credentialCmd.AddCommand(credentialListCmd)
	credentialCmd.AddCommand(credentialResetCmd)
	credentialCmd.AddCommand(credentialImportCmd)
	credentialImportCmd.AddCommand(credentialImportTSVCmd)
	credentialImportCmd.AddCommand(credentialImportJSONCmd)
}
