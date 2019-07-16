# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.9.0]
- Updated documentation: README and other md files and the yaml
- Refactored main.go
- Added device ids to `status` endpoint 
- Bumped codex
- Expect the record's times to be in UnixNano



## [v0.7.0]
- bumped codex to v0.5.0



## [v0.6.0]
 - Stopped building other services for integration tests
 - Bumped codex and bascule
 - Added wrp-go
 - Now we expect the event to be encoded as Msgpack


## [v0.5.1]
- fixed cipher yaml loading



## [v0.5.0]
 - Extended wrp.Message to include birthdate
 - If an event can't be unmarshaled or decrypted, an Unknown event is returned instead for `events` endpoint
 - Removed blacklist
 - bumped codex to v0.4.0 for cipher upgrades


## [v0.4.1]
- Bumped codex-common to v0.3.3
- Bumped bascule to v0.2.3



## [v0.4.0]
- Added Blacklist
- Added sat and basic auth verification using `bascule`
- bumped codex to v0.3.2
- bumped webpa-common to v1.0.0


## [v0.3.0]
 - Modified health metric to reflect unhealthy when pinging the database fails
 - Changed expected "reason-for-close" key in payload to "reason-for-closure" for `status` endpoint
 - Device id will always be converted to lowercase to query the database
 - Added sat verification for endpoints
 - Added basic level of decryption
 - Expect the data of a record to be a wrp.Message



## [v0.2.7]
 - bumped codex



## [v0.2.6]
- replace dep with Modules
- bumped codex
- Added a configurable limit to gets



## [v0.2.5]
 - Bumped codex common
 - Converted times to Unix for the `db` package



## [v0.2.4]
 - Bumped codex version



## [v0.2.3]
 - Bumped codex version



## [v0.2.2]
 - Bumped codex common to v0.2.3



## [v0.2.1]
- enabled pprof
- increased file limit
- bumped codex version


## [v0.2.0]
- modified events endpoints
- updated swagger docs
- bumped codex common version to v0.2.0
- added metrics


- updated swagger comments
- Centralized where http status responses come from
- Added unit tests

## [v0.1.1]
- added health endpoint
- updated bookkeeping logger
- working on automation

## [v0.1.0]
- Initial creation
- Bumped codex version, modified code to match changes

[Unreleased]: https://github.com/xmidt-org/codex-gungnir/compare/v0.9.0...HEAD
[v0.9.0]: https://github.com/xmidt-org/codex-gungnir/compare/v0.7.0...v0.9.0
[v0.7.0]: https://github.com/xmidt-org/codex-gungnir/compare/v0.6.0...v0.7.0
[v0.6.0]: https://github.com/xmidt-org/codex-gungnir/compare/v0.5.1...v0.6.0
[v0.5.1]: https://github.com/xmidt-org/codex-gungnir/compare/v0.5.0...v0.5.1
[v0.5.0]: https://github.com/xmidt-org/codex-gungnir/compare/v0.4.1...v0.5.0
[v0.4.1]: https://github.com/xmidt-org/codex-gungnir/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/xmidt-org/codex-gungnir/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/xmidt-org/codex-gungnir/compare/v0.2.7...v0.3.0
[v0.2.7]: https://github.com/xmidt-org/codex-gungnir/compare/v0.2.6...v0.2.7
[v0.2.6]: https://github.com/xmidt-org/codex-gungnir/compare/v0.2.5...v0.2.6
[v0.2.5]: https://github.com/xmidt-org/codex-gungnir/compare/v0.2.4...v0.2.5
[v0.2.4]: https://github.com/xmidt-org/codex-gungnir/compare/v0.2.3...v0.2.4
[v0.2.3]: https://github.com/xmidt-org/codex-gungnir/compare/v0.2.2...v0.2.3
[v0.2.2]: https://github.com/xmidt-org/codex-gungnir/compare/v0.2.1...v0.2.2
[v0.2.1]: https://github.com/xmidt-org/codex-gungnir/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/xmidt-org/codex-gungnir/compare/v0.1.1...v0.2.0
[v0.1.1]: https://github.com/xmidt-org/codex-gungnir/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/xmidt-org/codex-gungnir/compare/0.0.0...v0.1.0
