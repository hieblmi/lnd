# Release Notes
- [Bug Fixes](#bug-fixes)
- [New Features](#new-features)
    - [Functional Enhancements](#functional-enhancements)
    - [RPC Additions](#rpc-additions)
    - [lncli Additions](#lncli-additions)
- [Improvements](#improvements)
    - [Functional Updates](#functional-updates)
    - [RPC Updates](#rpc-updates)
    - [lncli Updates](#lncli-updates)
    - [Breaking Changes](#breaking-changes)
    - [Performance Improvements](#performance-improvements)
- [Technical and Architectural Updates](#technical-and-architectural-updates)
    - [BOLT Spec Updates](#bolt-spec-updates)
    - [Testing](#testing)
    - [Database](#database)
    - [Code Health](#code-health)
    - [Tooling and Documentation](#tooling-and-documentation)

# Bug Fixes

* [Fixed a bug](https://github.com/lightningnetwork/lnd/pull/8887) in error
  matching from publishing already confirmed transactions that can cause lnd
  fail to startup if `btcd` with an older version (pre-`v0.24.2`) is used.

# New Features
## Functional Enhancements
## RPC Additions
## lncli Additions

# Improvements
## Functional Updates
## RPC Updates
## lncli Updates
## Code Health
## Breaking Changes
## Performance Improvements

# Technical and Architectural Updates
## BOLT Spec Updates
## Testing
## Database

* [Fixed](https://github.com/lightningnetwork/lnd/pull/8854) pagination issues 
  in SQL invoicedb queries.

## Code Health
## Tooling and Documentation

# Contributors (Alphabetical Order)

* Yong Yu
