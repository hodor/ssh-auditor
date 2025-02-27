package main

import (
	"os"

	log "github.com/inconshreveable/log15"
	"github.com/hodor/ssh-auditor/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}
