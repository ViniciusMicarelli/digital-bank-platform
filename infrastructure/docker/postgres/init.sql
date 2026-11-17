-- One database per service (ADR-0003). Services never read each other's databases.
CREATE DATABASE ledger;
CREATE DATABASE account;
CREATE DATABASE transaction;
CREATE DATABASE pix;
CREATE DATABASE statement;
