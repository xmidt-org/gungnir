# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.14.2]
- Updated references to the main branch [#114](https://github.com/xmidt-org/gungnir/pull/114)
- Bumped bascule and webpa-common versions to have case sensitivity for jwt claims [#115](https://github.com/xmidt-org/gungnir/pull/115)

## [v0.14.1]
- add fix for empty sessionID [#112](https://github.com/xmidt-org/gungnir/pull/112)

## [v0.14.0]
- leverage sessionID for better judgement in offline vs online [#110](https://github.com/xmidt-org/gungnir/pull/110)
- changed Event struct tag from wrp to json [#110](https://github.com/xmidt-org/gungnir/pull/110)
- bumped codex-db to v0.6.0 [#110](https://github.com/xmidt-org/gungnir/pull/110)
- bumped wrp-go to v2.0.1 [#110](https://github.com/xmidt-org/gungnir/pull/110)
- bumped webpa-common to v1.9.0 to add configurable regexp for capability check metric labels [#111](https://github.com/xmidt-org/gungnir/pull/111)

## [v0.13.1]
 - fixed a capabilityCheck issue by correctly parsing out an additional `/` from the URL [#108](https://github.com/xmidt-org/gungnir/pull/108)

## [v0.13.0]
 - added configurable way to check capabilities and put results into metrics, without rejecting requests [#106](https://github.com/xmidt-org/gungnir/pull/106)

## [v0.12.3]
- fixed no state hash returned by updating codex-db to v0.5.2 [#105](https://github.com/xmidt-org/gungnir/pull/105)

## [v0.12.2]
- updated long-poll status codes [#103](https://github.com/xmidt-org/gungnir/pull/103)
- fixed long-poll first get [#104](https://github.com/xmidt-org/gungnir/pull/104)
- updated codex-db to v0.5.1 [#104](https://github.com/xmidt-org/gungnir/pull/104)

## [v0.12.1]
- Add timeouts to long-poll [#102](https://github.com/xmidt-org/gungnir/pull/102)

## [v0.12.0]
- Added long-poll feature [#97](https://github.com/xmidt-org/gungnir/pull/97)
- updated codex-db to v0.5.0

## [v0.11.2]
- fix json encoding of int for backward compatibility [#100](https://github.com/xmidt-org/gungnir/pull/100)

## [v0.11.1]
- fix json encoding of events payload [#98](https://github.com/xmidt-org/gungnir/pull/98)

## [v0.11.0]
- added getStatusLimit [#92](https://github.com/xmidt-org/gungnir/pull/92)
- Updated release pipeline to use travis [#93](https://github.com/xmidt-org/gungnir/pull/93)
- bumped codex-db to v0.4.0 and removed db retry logic [#94](https://github.com/xmidt-org/gungnir/pull/94)
- bumped webpa-common to v1.5.1 [#94](https://github.com/xmidt-org/gungnir/pull/94)
- bumped bascule to v0.7.0 and updated constructor setup to match [#94](https://github.com/xmidt-org/gungnir/pull/94)

## [v0.10.1]
- bumped db package to v0.3.1
- fixed go health package
- fix status endpoint

## [v0.10.1]
- bumped db package to v0.3.1
- fixed go health package
- fix status endpoint

## [v0.10.0]
- switched database configuration from postgres to cassandra
- bumped codex-db to v0.2.0

## [v0.9.2]

## [v0.9.1]
- Updated imports and versions for codex packages and webpa-common
- Updated bascule 
- Updated links to update new location for gungnir

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

[Unreleased]: https://github.com/xmidt-org/gungnir/compare/v0.14.2...HEAD
[v0.14.2]: https://github.com/xmidt-org/gungnir/compare/v0.14.1...v0.14.2
[v0.14.1]: https://github.com/xmidt-org/gungnir/compare/v0.14.0...v0.14.1
[v0.14.0]: https://github.com/xmidt-org/gungnir/compare/v0.13.1...v0.14.0
[v0.13.1]: https://github.com/xmidt-org/gungnir/compare/v0.13.0...v0.13.1
[v0.13.0]: https://github.com/xmidt-org/gungnir/compare/v0.12.3...v0.13.0
[v0.12.3]: https://github.com/xmidt-org/gungnir/compare/v0.12.2...v0.12.3
[v0.12.2]: https://github.com/xmidt-org/gungnir/compare/v0.12.1...v0.12.2
[v0.12.1]: https://github.com/xmidt-org/gungnir/compare/v0.12.0...v0.12.1
[v0.12.0]: https://github.com/xmidt-org/gungnir/compare/v0.11.2...v0.12.0
[v0.11.2]: https://github.com/xmidt-org/gungnir/compare/v0.11.1...v0.11.2
[v0.11.1]: https://github.com/xmidt-org/gungnir/compare/v0.11.0...v0.11.1
[v0.11.0]: https://github.com/xmidt-org/gungnir/compare/v0.10.1...v0.11.0
[v0.10.1]: https://github.com/xmidt-org/gungnir/compare/v0.10.0...v0.10.1
[v0.10.0]: https://github.com/xmidt-org/gungnir/compare/v0.9.2...v0.10.0
[v0.9.2]: https://github.com/xmidt-org/gungnir/compare/v0.9.1...v0.9.2
[v0.9.1]: https://github.com/xmidt-org/gungnir/compare/v0.9.0...v0.9.1
[v0.9.0]: https://github.com/xmidt-org/gungnir/compare/v0.7.0...v0.9.0
[v0.7.0]: https://github.com/xmidt-org/gungnir/compare/v0.6.0...v0.7.0
[v0.6.0]: https://github.com/xmidt-org/gungnir/compare/v0.5.1...v0.6.0
[v0.5.1]: https://github.com/xmidt-org/gungnir/compare/v0.5.0...v0.5.1
[v0.5.0]: https://github.com/xmidt-org/gungnir/compare/v0.4.1...v0.5.0
[v0.4.1]: https://github.com/xmidt-org/gungnir/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/xmidt-org/gungnir/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/xmidt-org/gungnir/compare/v0.2.7...v0.3.0
[v0.2.7]: https://github.com/xmidt-org/gungnir/compare/v0.2.6...v0.2.7
[v0.2.6]: https://github.com/xmidt-org/gungnir/compare/v0.2.5...v0.2.6
[v0.2.5]: https://github.com/xmidt-org/gungnir/compare/v0.2.4...v0.2.5
[v0.2.4]: https://github.com/xmidt-org/gungnir/compare/v0.2.3...v0.2.4
[v0.2.3]: https://github.com/xmidt-org/gungnir/compare/v0.2.2...v0.2.3
[v0.2.2]: https://github.com/xmidt-org/gungnir/compare/v0.2.1...v0.2.2
[v0.2.1]: https://github.com/xmidt-org/gungnir/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/xmidt-org/gungnir/compare/v0.1.1...v0.2.0
[v0.1.1]: https://github.com/xmidt-org/gungnir/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/xmidt-org/gungnir/compare/0.0.0...v0.1.0
