// Copyright 2015 ISRG.  All rights reserved
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"crypto/x509"
	"database/sql"
	"fmt"
	"time"

	"github.com/letsencrypt/boulder/Godeps/_workspace/src/github.com/cactus/go-statsd-client/statsd"
	"github.com/letsencrypt/boulder/Godeps/_workspace/src/github.com/jmhodges/clock"
	gorp "github.com/letsencrypt/boulder/Godeps/_workspace/src/gopkg.in/gorp.v1"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/core"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/rpc"
	"github.com/letsencrypt/boulder/sa"
)

// OCSPUpdater contains the useful objects for the Updater
type OCSPUpdater struct {
	stats statsd.Statter
	log   *blog.AuditLogger
	clk   clock.Clock

	dbMap *gorp.DbMap

	cac  core.CertificateAuthority
	pubc core.Publisher
	sac  core.StorageAuthority

	// Used  to calculate how far back stale OCSP responses should be looked for
	ocspMinTimeToExpiry time.Duration
	// Used to calculate how far back missing SCT receipts should be looked for
	oldestIssuedSCT time.Duration
	// Number of CT logs we expect to have receipts from
	numLogs int

	loops []*looper
}

// This is somewhat gross but can be pared down a bit once the publisher and this
// are fully smooshed together
func newUpdater(
	stats statsd.Statter,
	clk clock.Clock,
	dbMap *gorp.DbMap,
	ca core.CertificateAuthority,
	pub core.Publisher,
	sac core.StorageAuthority,
	config cmd.OCSPUpdaterConfig,
	numLogs int,
) (*OCSPUpdater, error) {
	if config.NewCertificateBatchSize == 0 ||
		config.OldOCSPBatchSize == 0 ||
		config.MissingSCTBatchSize == 0 {
		return nil, fmt.Errorf("Loop batch sizes must be non-zero")
	}
	if config.NewCertificateWindow.Duration == 0 ||
		config.OldOCSPWindow.Duration == 0 ||
		config.MissingSCTWindow.Duration == 0 {
		return nil, fmt.Errorf("Loop window sizes must be non-zero")
	}

	updater := OCSPUpdater{
		stats:               stats,
		clk:                 clk,
		dbMap:               dbMap,
		cac:                 ca,
		sac:                 sac,
		pubc:                pub,
		log:                 blog.GetAuditLogger(),
		numLogs:             numLogs,
		ocspMinTimeToExpiry: config.OCSPMinTimeToExpiry.Duration,
		oldestIssuedSCT:     config.OldestIssuedSCT.Duration,
	}

	// Setup loops
	updater.loops = []*looper{
		&looper{
			clk:                  clk,
			stats:                stats,
			batchSize:            config.NewCertificateBatchSize,
			tickDur:              config.NewCertificateWindow.Duration,
			tickFunc:             updater.newCertificateTick,
			name:                 "NewCertificates",
			failureBackoffFactor: config.SignFailureBackoffFactor,
			failureBackoffMax:    config.SignFailureBackoffMax.Duration,
		},
		&looper{
			clk:                  clk,
			stats:                stats,
			batchSize:            config.OldOCSPBatchSize,
			tickDur:              config.OldOCSPWindow.Duration,
			tickFunc:             updater.oldOCSPResponsesTick,
			name:                 "OldOCSPResponses",
			failureBackoffFactor: config.SignFailureBackoffFactor,
			failureBackoffMax:    config.SignFailureBackoffMax.Duration,
		},
		// The missing SCT loop doesn't need to know about failureBackoffFactor or
		// failureBackoffMax as it doesn't make any calls to the CA
		&looper{
			clk:       clk,
			stats:     stats,
			batchSize: config.MissingSCTBatchSize,
			tickDur:   config.MissingSCTWindow.Duration,
			tickFunc:  updater.missingReceiptsTick,
			name:      "MissingSCTReceipts",
		},
	}
	if config.RevokedCertificateBatchSize != 0 &&
		config.RevokedCertificateWindow.Duration != 0 {
		updater.loops = append(updater.loops, &looper{
			clk:                  clk,
			stats:                stats,
			batchSize:            config.RevokedCertificateBatchSize,
			tickDur:              config.RevokedCertificateWindow.Duration,
			tickFunc:             updater.revokedCertificatesTick,
			name:                 "RevokedCertificates",
			failureBackoffFactor: config.SignFailureBackoffFactor,
			failureBackoffMax:    config.SignFailureBackoffMax.Duration,
		})
	}

	updater.ocspMinTimeToExpiry = config.OCSPMinTimeToExpiry.Duration

	return &updater, nil
}

func (updater *OCSPUpdater) findStaleOCSPResponses(oldestLastUpdatedTime time.Time, batchSize int) ([]core.CertificateStatus, error) {
	var statuses []core.CertificateStatus
	_, err := updater.dbMap.Select(
		&statuses,
		`SELECT cs.*
			 FROM certificateStatus AS cs
			 JOIN certificates AS cert
			 ON cs.serial = cert.serial
			 WHERE cs.ocspLastUpdated < :lastUpdate
			 AND cert.expires > now()
			 ORDER BY cs.ocspLastUpdated ASC
			 LIMIT :limit`,
		map[string]interface{}{
			"lastUpdate": oldestLastUpdatedTime,
			"limit":      batchSize,
		},
	)
	if err == sql.ErrNoRows {
		return statuses, nil
	}
	return statuses, err
}

func (updater *OCSPUpdater) getCertificatesWithMissingResponses(batchSize int) ([]core.CertificateStatus, error) {
	var statuses []core.CertificateStatus
	_, err := updater.dbMap.Select(
		&statuses,
		`SELECT * FROM certificateStatus
			 WHERE ocspLastUpdated = 0
			 LIMIT :limit`,
		map[string]interface{}{
			"limit": batchSize,
		},
	)
	if err == sql.ErrNoRows {
		return statuses, nil
	}
	return statuses, err
}

type responseMeta struct {
	*core.OCSPResponse
	*core.CertificateStatus
}

func (updater *OCSPUpdater) generateResponse(status core.CertificateStatus) (*core.CertificateStatus, error) {
	var cert core.Certificate
	err := updater.dbMap.SelectOne(
		&cert,
		"SELECT * FROM certificates WHERE serial = :serial",
		map[string]interface{}{"serial": status.Serial},
	)
	if err != nil {
		return nil, err
	}

	_, err = x509.ParseCertificate(cert.DER)
	if err != nil {
		return nil, err
	}

	signRequest := core.OCSPSigningRequest{
		CertDER:   cert.DER,
		Reason:    status.RevokedReason,
		Status:    string(status.Status),
		RevokedAt: status.RevokedDate,
	}

	ocspResponse, err := updater.cac.GenerateOCSP(signRequest)
	if err != nil {
		return nil, err
	}

	status.OCSPLastUpdated = updater.clk.Now()
	status.OCSPResponse = ocspResponse
	return &status, nil
}

func (updater *OCSPUpdater) generateRevokedResponse(status core.CertificateStatus) (*core.CertificateStatus, error) {
	cert, err := updater.sac.GetCertificate(status.Serial)
	if err != nil {
		return nil, err
	}

	signRequest := core.OCSPSigningRequest{
		CertDER:   cert.DER,
		Status:    string(core.OCSPStatusRevoked),
		Reason:    status.RevokedReason,
		RevokedAt: status.RevokedDate,
	}

	ocspResponse, err := updater.cac.GenerateOCSP(signRequest)
	if err != nil {
		return nil, err
	}

	now := updater.clk.Now()
	status.OCSPLastUpdated = now
	status.OCSPResponse = ocspResponse
	return &status, nil
}

func (updater *OCSPUpdater) storeResponse(status *core.CertificateStatus) error {
	// Update the certificateStatus table with the new OCSP response, the status
	// WHERE is used make sure we don't overwrite a revoked response with a one
	// containing a 'good' status and that we don't do the inverse when the OCSP
	// status should be 'good'.
	_, err := updater.dbMap.Exec(
		`UPDATE certificateStatus
		 SET ocspResponse=?,ocspLastUpdated=?
		 WHERE serial=?
		 AND status=?`,
		status.OCSPResponse,
		status.OCSPLastUpdated,
		status.Serial,
		string(status.Status),
	)
	return err
}

// newCertificateTick checks for certificates issued since the last tick and
// generates and stores OCSP responses for these certs
func (updater *OCSPUpdater) newCertificateTick(batchSize int) error {
	// Check for anything issued between now and previous tick and generate first
	// OCSP responses
	statuses, err := updater.getCertificatesWithMissingResponses(batchSize)
	if err != nil {
		updater.stats.Inc("OCSP.Errors.FindMissingResponses", 1, 1.0)
		updater.log.AuditErr(fmt.Errorf("Failed to find certificates with missing OCSP responses: %s", err))
		return err
	}

	return updater.generateOCSPResponses(statuses)
}

func (updater *OCSPUpdater) findRevokedCertificatesToUpdate(batchSize int) ([]core.CertificateStatus, error) {
	var statuses []core.CertificateStatus
	_, err := updater.dbMap.Select(
		&statuses,
		`SELECT * FROM certificateStatus
		 WHERE status = :revoked
		 AND ocspLastUpdated <= revokedDate
		 LIMIT :limit`,
		map[string]interface{}{
			"revoked": string(core.OCSPStatusRevoked),
			"limit":   batchSize,
		},
	)
	return statuses, err
}

func (updater *OCSPUpdater) revokedCertificatesTick(batchSize int) error {
	statuses, err := updater.findRevokedCertificatesToUpdate(batchSize)
	if err != nil {
		updater.stats.Inc("OCSP.Errors.FindRevokedCertificates", 1, 1.0)
		updater.log.AuditErr(fmt.Errorf("Failed to find revoked certificates: %s", err))
		return err
	}

	for _, status := range statuses {
		meta, err := updater.generateRevokedResponse(status)
		if err != nil {
			updater.log.AuditErr(fmt.Errorf("Failed to generate revoked OCSP response: %s", err))
			updater.stats.Inc("OCSP.Errors.RevokedResponseGeneration", 1, 1.0)
			return err
		}
		err = updater.storeResponse(meta)
		if err != nil {
			updater.stats.Inc("OCSP.Errors.StoreRevokedResponse", 1, 1.0)
			updater.log.AuditErr(fmt.Errorf("Failed to store OCSP response: %s", err))
			continue
		}
	}
	return nil
}

func (updater *OCSPUpdater) generateOCSPResponses(statuses []core.CertificateStatus) error {
	for _, status := range statuses {
		meta, err := updater.generateResponse(status)
		if err != nil {
			updater.log.AuditErr(fmt.Errorf("Failed to generate OCSP response: %s", err))
			updater.stats.Inc("OCSP.Errors.ResponseGeneration", 1, 1.0)
			return err
		}
		updater.stats.Inc("OCSP.GeneratedResponses", 1, 1.0)
		err = updater.storeResponse(meta)
		if err != nil {
			updater.log.AuditErr(fmt.Errorf("Failed to store OCSP response: %s", err))
			updater.stats.Inc("OCSP.Errors.StoreResponse", 1, 1.0)
			continue
		}
		updater.stats.Inc("OCSP.StoredResponses", 1, 1.0)
	}
	return nil
}

// oldOCSPResponsesTick looks for certificates with stale OCSP responses and
// generates/stores new ones
func (updater *OCSPUpdater) oldOCSPResponsesTick(batchSize int) error {
	now := time.Now()
	statuses, err := updater.findStaleOCSPResponses(now.Add(-updater.ocspMinTimeToExpiry), batchSize)
	if err != nil {
		updater.stats.Inc("OCSP.Errors.FindStaleResponses", 1, 1.0)
		updater.log.AuditErr(fmt.Errorf("Failed to find stale OCSP responses: %s", err))
		return err
	}

	return updater.generateOCSPResponses(statuses)
}

func (updater *OCSPUpdater) getSerialsIssuedSince(since time.Time, batchSize int) ([]string, error) {
	var serials []string
	_, err := updater.dbMap.Select(
		&serials,
		`SELECT serial FROM certificates
			 WHERE issued > :since
			 ORDER BY issued ASC
			 LIMIT :limit`,
		map[string]interface{}{
			"since": since,
			"limit": batchSize,
		},
	)
	if err == sql.ErrNoRows {
		return serials, nil
	}
	return serials, err
}

func (updater *OCSPUpdater) getNumberOfReceipts(serial string) (int, error) {
	var count int
	err := updater.dbMap.SelectOne(
		&count,
		"SELECT COUNT(id) FROM sctReceipts WHERE certificateSerial = :serial",
		map[string]interface{}{"serial": serial},
	)
	return count, err
}

// missingReceiptsTick looks for certificates without the correct number of SCT
// receipts and retrieves them
func (updater *OCSPUpdater) missingReceiptsTick(batchSize int) error {
	now := updater.clk.Now()
	since := now.Add(-updater.oldestIssuedSCT)
	serials, err := updater.getSerialsIssuedSince(since, batchSize)
	if err != nil {
		updater.log.AuditErr(fmt.Errorf("Failed to get certificate serials: %s", err))
		return err
	}

	for _, serial := range serials {
		count, err := updater.getNumberOfReceipts(serial)
		if err != nil {
			updater.log.AuditErr(fmt.Errorf("Failed to get number of SCT receipts for certificate: %s", err))
			continue
		}
		if count == updater.numLogs {
			continue
		}
		cert, err := updater.sac.GetCertificate(serial)
		if err != nil {
			updater.log.AuditErr(fmt.Errorf("Failed to get certificate: %s", err))
			continue
		}

		err = updater.pubc.SubmitToCT(cert.DER)
		if err != nil {
			updater.log.AuditErr(fmt.Errorf("Failed to submit certificate to CT log: %s", err))
			continue
		}
	}
	return nil
}

type looper struct {
	clk                  clock.Clock
	stats                statsd.Statter
	batchSize            int
	tickDur              time.Duration
	tickFunc             func(int) error
	name                 string
	failureBackoffFactor float64
	failureBackoffMax    time.Duration
	failures             int
}

func (l *looper) tick() {
	tickStart := l.clk.Now()
	err := l.tickFunc(l.batchSize)
	l.stats.TimingDuration(fmt.Sprintf("OCSP.%s.TickDuration", l.name), time.Since(tickStart), 1.0)
	l.stats.Inc(fmt.Sprintf("OCSP.%s.Ticks", l.name), 1, 1.0)
	tickEnd := tickStart.Add(time.Since(tickStart))
	expectedTickEnd := tickStart.Add(l.tickDur)
	if tickEnd.After(expectedTickEnd) {
		l.stats.Inc(fmt.Sprintf("OCSP.%s.LongTicks", l.name), 1, 1.0)
	}

	// After we have all the stats stuff out of the way let's check if the tick
	// function failed, if the reason is the HSM is dead increase the length of
	// sleepDur using the exponentially increasing duration returned by core.RetryBackoff.
	sleepDur := expectedTickEnd.Sub(tickEnd)
	if err != nil {
		l.stats.Inc(fmt.Sprintf("OCSP.%s.FailedTicks", l.name), 1, 1.0)
		if _, ok := err.(core.ServiceUnavailableError); ok && (l.failureBackoffFactor > 0 && l.failureBackoffMax > 0) {
			l.failures++
			sleepDur = core.RetryBackoff(l.failures, l.tickDur, l.failureBackoffMax, l.failureBackoffFactor)
		}
	} else if l.failures > 0 {
		// If the tick was successful and previously there were failures reset
		// counter to 0
		l.failures = 0
	}

	// Sleep for the remaining tick period or for the backoff time
	l.clk.Sleep(sleepDur)
}

func (l *looper) loop() error {
	if l.batchSize == 0 || l.tickDur == 0 {
		return fmt.Errorf("Both batch size and tick duration are required, not running '%s' loop", l.name)
	}
	for {
		l.tick()
	}
}

func setupClients(c cmd.Config, stats statsd.Statter) (
	core.CertificateAuthority,
	core.Publisher,
	core.StorageAuthority,
) {
	caRPC, err := rpc.NewAmqpRPCClient("OCSP->CA", c.AMQP.CA.Server, c, clock.Default(), stats)
	cmd.FailOnError(err, "Unable to create RPC client")

	cac, err := rpc.NewCertificateAuthorityClient(caRPC)
	cmd.FailOnError(err, "Unable to create CA client")

	pubRPC, err := rpc.NewAmqpRPCClient("OCSP->Publisher", c.AMQP.Publisher.Server, c, clock.Default(), stats)
	cmd.FailOnError(err, "Unable to create RPC client")

	pubc, err := rpc.NewPublisherClient(pubRPC)
	cmd.FailOnError(err, "Unable to create Publisher client")

	saRPC, err := rpc.NewAmqpRPCClient("OCSP->SA", c.AMQP.SA.Server, c, clock.Default(), stats)
	cmd.FailOnError(err, "Unable to create RPC client")

	sac, err := rpc.NewStorageAuthorityClient(saRPC)
	cmd.FailOnError(err, "Unable to create Publisher client")

	return cac, pubc, sac
}

func main() {
	app := cmd.NewAppShell("ocsp-updater", "Generates and updates OCSP responses")

	app.Action = func(c cmd.Config) {
		// Set up logging
		stats, err := statsd.NewClient(c.Statsd.Server, c.Statsd.Prefix)
		cmd.FailOnError(err, "Couldn't connect to statsd")

		auditlogger, err := blog.Dial(c.Syslog.Network, c.Syslog.Server, c.Syslog.Tag, stats)
		cmd.FailOnError(err, "Could not connect to Syslog")
		auditlogger.Info(app.VersionString())

		blog.SetAuditLogger(auditlogger)

		// AUDIT[ Error Conditions ] 9cc4d537-8534-4970-8665-4b382abe82f3
		defer auditlogger.AuditPanic()

		go cmd.DebugServer(c.OCSPUpdater.DebugAddr)
		go cmd.ProfileCmd("OCSP-Updater", stats)

		// Configure DB
		dbMap, err := sa.NewDbMap(c.OCSPUpdater.DBConnect)
		cmd.FailOnError(err, "Could not connect to database")

		cac, pubc, sac := setupClients(c, stats)

		updater, err := newUpdater(
			stats,
			clock.Default(),
			dbMap,
			cac,
			pubc,
			sac,
			// Necessary evil for now
			c.OCSPUpdater,
			len(c.Common.CT.Logs),
		)

		cmd.FailOnError(err, "Failed to create updater")

		for _, l := range updater.loops {
			go func(loop *looper) {
				err = loop.loop()
				if err != nil {
					auditlogger.AuditErr(err)
				}
			}(l)
		}

		// Sleep forever (until signaled)
		select {}
	}

	app.Run()
}
