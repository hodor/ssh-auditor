package sshauditor

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	log "github.com/inconshreveable/log15"
	"github.com/pkg/errors"
)

type ScanConfiguration struct {
	Include     []string
	Exclude     []string
	Ports       []int
	Concurrency int
	Timeout     time.Duration
}
type AuditResult struct {
	totalCount int
	negCount   int
	posCount   int
	errCount   int
}
type AuditReport struct {
	ActiveHosts      []Host
	ActiveHostsCount int

	DuplicateKeys      map[string][]Host
	DuplicateKeysCount int

	Vulnerabilities      []Vulnerability
	VulnerabilitiesCount int
}

func joinInts(ints []int, sep string) string {
	var foo []string
	for _, i := range ints {
		foo = append(foo, strconv.Itoa(i))
	}
	return strings.Join(foo, sep)
}

//expandScanConfiguration takes a ScanConfiguration and returns a channel
//of all hostports that match the scan configuration.
func expandScanConfiguration(cfg ScanConfiguration) (chan string, error) {
	hostChan := make(chan string, 1024)
	hosts, err := EnumerateHosts(cfg.Include, cfg.Exclude)
	if err != nil {
		return hostChan, err
	}
	log.Info("discovering hosts",
		"include", strings.Join(cfg.Include, ","),
		"exclude", strings.Join(cfg.Exclude, ","),
		"total", len(hosts),
		"ports", joinInts(cfg.Ports, ","),
	)
	go func() {
		// Iterate over ports first, so for a large scan there's a
		// delay between attempts per host
		for _, port := range cfg.Ports {
			portString := strconv.Itoa(port)
			for _, h := range hosts {
				hostChan <- net.JoinHostPort(h, portString)
			}
		}
		close(hostChan)
	}()
	return hostChan, err
}

type SSHAuditor struct {
	//TODO: should be interface
	store *SQLiteStore
}

func New(store *SQLiteStore) *SSHAuditor {
	return &SSHAuditor{
		store: store,
	}
}

func (a *SSHAuditor) updateStoreFromDiscovery(hosts chan SSHHost) error {
	knownHosts, err := a.store.getKnownHosts()
	if err != nil {
		return err
	}
	log.Info("current known hosts", "count", len(knownHosts))
	var totalCount, updatedCount, newCount int

	hostsWrapped := make(chan interface{})
	go func() {
		for v := range hosts {
			hostsWrapped <- v
		}
		close(hostsWrapped)
	}()
	for hostBatch := range batch(context.TODO(), hostsWrapped, 50, 2*time.Second) {
		_, err := a.store.Begin()
		if err != nil {
			return errors.Wrap(err, "updateStoreFromDiscovery")
		}
		for _, host := range hostBatch {
			host := host.(SSHHost)
			var needUpdate bool
			rec, existing := knownHosts[host.hostport]
			if existing {
				if host.keyfp == "" {
					host.keyfp = rec.Fingerprint
				}
				if host.version == "" {
					host.version = rec.Version
				}
				needUpdate = (host.keyfp != rec.Fingerprint || host.version != rec.Version)
				err := a.store.addHostChanges(host, rec)
				if err != nil {
					return errors.Wrap(err, "updateStoreFromDiscovery")
				}
			}
			l := log.New("host", host.hostport, "version", host.version, "fp", host.keyfp)
			if !existing || needUpdate {
				err = a.store.addOrUpdateHost(host)
				if err != nil {
					return errors.Wrap(err, "updateStoreFromDiscovery")
				}
			}
			//If it already existed and we didn't otherwise update it, mark that it was seen
			if existing {
				err = a.store.setLastSeen(host)
				if err != nil {
					return errors.Wrap(err, "updateStoreFromDiscovery")
				}
			}
			totalCount++
			if !existing {
				l.Info("discovered new host")
				newCount++
			} else if needUpdate {
				l.Info("discovered changed host")
				updatedCount++
			}
		}
		err = a.store.Commit()
		if err != nil {
			return errors.Wrap(err, "updateStoreFromDiscovery")
		}
	}
	log.Info("discovery report", "total", totalCount, "new", newCount, "updated", updatedCount)
	return nil
}

func (a *SSHAuditor) updateQueues() error {
	queued, err := a.store.initHostCreds()
	if err != nil {
		return err
	}
	queuesize, err := a.store.getScanQueueSize()
	if err != nil {
		return err
	}
	log.Info("brute force queue size", "new", queued, "total", queuesize)
	return nil
}

func (a *SSHAuditor) Discover(cfg ScanConfiguration) error {
	//Push all candidate hosts into the banner fetcher queue
	hostChan, err := expandScanConfiguration(cfg)
	if err != nil {
		return err
	}

	portResults := bannerFetcher(cfg.Concurrency*2, hostChan)
	keyResults := fingerPrintFetcher(cfg.Concurrency, portResults)

	err = a.updateStoreFromDiscovery(keyResults)
	if err != nil {
		return err
	}

	err = a.updateQueues()
	return err
}

func (a *SSHAuditor) brute(scantype string, cfg ScanConfiguration) (AuditResult, error) {
	var res AuditResult
	a.updateQueues()
	var err error

	var sc []ScanRequest
	switch scantype {
	case "scan":
		sc, err = a.store.getScanQueue()
	case "rescan":
		sc, err = a.store.getRescanQueue()
	}
	if err != nil {
		return res, errors.Wrap(err, "Error getting scan queue")
	}
	bruteResults := bruteForcer(cfg.Concurrency, sc, cfg.Timeout)

	bruteResultsWrapped := make(chan interface{})
	go func() {
		for v := range bruteResults {
			bruteResultsWrapped <- v
		}
		close(bruteResultsWrapped)
	}()

	var totalCount, errCount, negCount, posCount int
	for bruteBatch := range batch(context.TODO(), bruteResultsWrapped, 50, 2*time.Second) {
		_, err = a.store.Begin()
		if err != nil {
			return res, errors.Wrap(err, "brute")
		}

		for _, br := range bruteBatch {
			br := br.(BruteForceResult)
			l := log.New(
				"host", br.hostport,
				"user", br.cred.User,
				"password", br.cred.Password,
				"result", br.result,
			)
			if br.err != nil {
				l.Error("brute force error", "err", br.err.Error())
				errCount++
			} else if br.result == "" {
				l.Debug("negative brute force result")
				negCount++
			} else {
				l.Info("positive brute force result")
				posCount++
			}
			err = a.store.updateBruteResult(br)
			if err != nil {
				return res, err
			}
			totalCount++
		}
		err = a.store.Commit()
		if err != nil {
			return res, errors.Wrap(err, "brute")
		}
	}
	log.Info("brute force scan report", "total", totalCount, "neg", negCount, "pos", posCount, "err", errCount)
	return AuditResult{
		totalCount: totalCount,
		negCount:   negCount,
		posCount:   posCount,
		errCount:   errCount,
	}, nil
}

func (a *SSHAuditor) Scan(cfg ScanConfiguration) (AuditResult, error) {
	return a.brute("scan", cfg)
}
func (a *SSHAuditor) Rescan(cfg ScanConfiguration) (AuditResult, error) {
	return a.brute("rescan", cfg)
}

func (a *SSHAuditor) Dupes() (map[string][]Host, error) {
	keyMap := make(map[string][]Host)

	hosts, err := a.store.GetActiveHosts(2)

	if err != nil {
		return keyMap, errors.Wrap(err, "Dupes")
	}

	for _, h := range hosts {
		keyMap[h.Fingerprint] = append(keyMap[h.Fingerprint], h)
	}

	for fp, hosts := range keyMap {
		if len(hosts) == 1 {
			delete(keyMap, fp)
		}
	}
	return keyMap, nil
}
func (a *SSHAuditor) getLogCheckScanQueue() ([]ScanRequest, error) {
	var requests []ScanRequest
	hostList, err := a.store.GetActiveHosts(14)
	if err != nil {
		return requests, errors.Wrap(err, "getLogCheckQueue")
	}

	for _, h := range hostList {
		host, _, err := net.SplitHostPort(h.Hostport)
		if err != nil {
			log.Warn("bad hostport? %s %s", h.Hostport, err)
			continue
		}
		user := fmt.Sprintf("logcheck-%s", host)

		sr := ScanRequest{
			hostport:    h.Hostport,
			credentials: []Credential{{User: user, Password: "logcheck"}},
		}
		requests = append(requests, sr)
	}

	return requests, nil
}

func (a *SSHAuditor) Logcheck(cfg ScanConfiguration) error {
	sc, err := a.getLogCheckScanQueue()
	if err != nil {
		return err
	}

	bruteResults := bruteForcer(cfg.Concurrency, sc, cfg.Timeout)

	for br := range bruteResults {
		l := log.New("host", br.hostport, "user", br.cred.User)
		if br.err != nil {
			l.Error("Failed to send logcheck auth request", "error", br.err)
			continue
		}
		l.Info("Sent logcheck auth request")
		//TODO Collect hostports and return them for syslog cross referencing
	}
	return nil
}

func (a *SSHAuditor) LogcheckReport(ls LogSearcher) error {
	activeHosts, err := a.store.GetActiveHosts(2)
	if err != nil {
		return errors.Wrap(err, "LogcheckReport GetActiveHosts failed")
	}

	foundIPs, err := ls.GetIPs()
	if err != nil {
		return errors.Wrap(err, "LogcheckReport GetIPs failed")
	}

	logPresent := make(map[string]bool)
	for _, host := range foundIPs {
		logPresent[host] = true
	}

	log.Info("found active hosts in store", "count", len(activeHosts))
	log.Info("found related hosts in logs", "count", len(foundIPs))

	for _, host := range activeHosts {
		ip, _, err := net.SplitHostPort(host.Hostport)
		if err != nil {
			log.Error("invalid hostport", "host", host.Hostport)
			continue
		}
		fmt.Printf("%s %v\n", host.Hostport, logPresent[ip])
	}
	return nil
}

func (a *SSHAuditor) Vulnerabilities() ([]Vulnerability, error) {
	return a.store.GetVulnerabilities()
}

func (a *SSHAuditor) GetReport() (AuditReport, error) {
	var rep AuditReport
	hosts, err := a.store.GetActiveHosts(2)
	rep.ActiveHosts = hosts
	rep.ActiveHostsCount = len(hosts)
	if err != nil {
		return rep, err
	}

	dupes, err := a.Dupes()
	if err != nil {
		return rep, err
	}
	rep.DuplicateKeys = dupes
	rep.DuplicateKeysCount = len(dupes)

	vulns, err := a.Vulnerabilities()
	if err != nil {
		return rep, err
	}
	rep.Vulnerabilities = vulns
	rep.VulnerabilitiesCount = len(vulns)

	return rep, nil
}
