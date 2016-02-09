--
-- Copyright 2015 ISRG.  All rights reserved
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.
--
-- This file defines the default users for the primary database, used by
-- all the parts of Boulder except the Certificate Authority module, which
-- utilizes its own database.
--

-- Create users for each component with the appropriate permissions. We want to
-- drop each user and recreate them, but if the user doesn't already exist, the
-- drop command will fail. So we grant the dummy `USAGE` privilege to make sure
-- the user exists and then drop the user.

-- Storage Authority
GRANT DELETE,SELECT,INSERT,UPDATE ON authz TO 'sa'@'localhost';
GRANT DELETE,SELECT,INSERT,UPDATE,DELETE ON pendingAuthorizations TO 'sa'@'localhost';
GRANT DELETE,SELECT(id,Lockcol) ON pendingAuthorizations TO 'sa'@'localhost';
GRANT DELETE,SELECT,INSERT ON certificates TO 'sa'@'localhost';
GRANT DELETE,SELECT,INSERT,UPDATE ON certificateStatus TO 'sa'@'localhost';
GRANT DELETE,SELECT,INSERT ON issuedNames TO 'sa'@'localhost';
GRANT DELETE,SELECT,INSERT ON sctReceipts TO 'sa'@'localhost';
GRANT DELETE,SELECT,INSERT ON deniedCSRs TO 'sa'@'localhost';
GRANT DELETE,INSERT ON ocspResponses TO 'sa'@'localhost';
GRANT DELETE,SELECT,INSERT,UPDATE ON registrations TO 'sa'@'localhost';
GRANT DELETE,SELECT,INSERT,UPDATE ON challenges TO 'sa'@'localhost';

-- OCSP Responder
GRANT DELETE,SELECT ON certificateStatus TO 'ocsp_resp'@'localhost';
GRANT DELETE,SELECT ON ocspResponses TO 'ocsp_resp'@'localhost';

-- OCSP Generator Tool (Updater)
GRANT DELETE,INSERT ON ocspResponses TO 'ocsp_update'@'localhost';
GRANT DELETE,SELECT ON certificates TO 'ocsp_update'@'localhost';
GRANT DELETE,SELECT,UPDATE ON certificateStatus TO 'ocsp_update'@'localhost';
GRANT DELETE,SELECT ON sctReceipts TO 'ocsp_update'@'localhost';

-- Revoker Tool
GRANT DELETE,SELECT ON registrations TO 'revoker'@'localhost';
GRANT DELETE,SELECT ON certificates TO 'revoker'@'localhost';
GRANT DELETE,SELECT,INSERT ON deniedCSRs TO 'revoker'@'localhost';

-- External Cert Importer
GRANT DELETE,SELECT,INSERT,UPDATE,DELETE ON identifierData TO 'importer'@'localhost';
GRANT DELETE,SELECT,INSERT,UPDATE,DELETE ON externalCerts TO 'importer'@'localhost';

-- Expiration mailer
GRANT DELETE,SELECT ON certificates TO 'mailer'@'localhost';
GRANT DELETE,SELECT,UPDATE ON certificateStatus TO 'mailer'@'localhost';

-- Cert checker
GRANT DELETE,SELECT ON certificates TO 'cert_checker'@'localhost';

-- Test setup and teardown
GRANT ALL PRIVILEGES ON * to 'test_setup'@'localhost';
GRANT ALL PRIVILEGES ON * to 'sa'@'localhost';
GRANT ALL PRIVILEGES ON * to 'ocsp_resp'@'localhost';
GRANT ALL PRIVILEGES ON * to 'ocsp_update'@'localhost';
GRANT ALL PRIVILEGES ON * to 'revoker'@'localhost';
GRANT ALL PRIVILEGES ON * to 'importer'@'localhost';
GRANT ALL PRIVILEGES ON * to 'mailer'@'localhost';
GRANT ALL PRIVILEGES ON * to 'cert_checker'@'localhost';

