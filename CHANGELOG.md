# Changelog

## [1.8.0](https://github.com/valter-silva-au/ai-dev-brain/compare/v1.7.0...v1.8.0) (2026-02-25)


### Features

* add .gitignore template and git init to adb init ([94a31ff](https://github.com/valter-silva-au/ai-dev-brain/commit/94a31ffb72b06ea886faa1e59c4a2c5a756e3ef0))
* add `adb init` command to bootstrap project workspace ([f3ce485](https://github.com/valter-silva-au/ai-dev-brain/commit/f3ce4858bdbe5530c7e5d69ed91ff0ccd0d0102c))
* add Claude Code status line with adb task context and portfolio… ([#26](https://github.com/valter-silva-au/ai-dev-brain/issues/26)) ([b2d7731](https://github.com/valter-silva-au/ai-dev-brain/commit/b2d77319bf2f05ff291ad521120269e78773ec19))
* add cleanup command, fix worktree placement, and persist terminal title ([237f83e](https://github.com/valter-silva-au/ai-dev-brain/commit/237f83e1265e80fb377d52297a5cbee3b6034743))
* add configurable task ID pad width, branch pattern, and dynamic… ([#25](https://github.com/valter-silva-au/ai-dev-brain/issues/25)) ([6d585c2](https://github.com/valter-silva-au/ai-dev-brain/commit/6d585c28b584b166ab32956ed8dcdf68177b051c))
* add feedback loop, knowledge store, and channel adapter system ([#21](https://github.com/valter-silva-au/ai-dev-brain/issues/21)) ([455b44b](https://github.com/valter-silva-au/ai-dev-brain/commit/455b44bac3ab2cf932c737a67662be2d98d33c9f))
* add observability infrastructure with metrics, alerts, and session tracking ([#14](https://github.com/valter-silva-au/ai-dev-brain/issues/14)) ([2598c62](https://github.com/valter-silva-au/ai-dev-brain/commit/2598c62d3d514b167ddbb1671a6003aafad52a93))
* add observability infrastructure, MCP config, agents, skills, and hooks ([2e2d722](https://github.com/valter-silva-au/ai-dev-brain/commit/2e2d7226f239250c504a44093450a3f7a136d5a7))
* add Phase 3 advanced features - TUI dashboard, MCP server, Slack notifications ([#14](https://github.com/valter-silva-au/ai-dev-brain/issues/14)) ([917f622](https://github.com/valter-silva-au/ai-dev-brain/commit/917f622afcb726996108be4ba4d75f3560875d6d))
* add Release Please for automated versioning and changelog ([b9ab55e](https://github.com/valter-silva-au/ai-dev-brain/commit/b9ab55e040b0e57881e982e75da6cb6a268ac6f8))
* add sync-repos command with backlog-aware branch protection ([f68e1aa](https://github.com/valter-silva-au/ai-dev-brain/commit/f68e1aa8ddf05a5d57525ed281d22e7c91307262))
* add workflow skills (push, pr, review, sync, changelog) to adb init ([8990b54](https://github.com/valter-silva-au/ai-dev-brain/commit/8990b54866b6253523a0f956f322761b752065ba))
* add worktree auto-detect, post-create workflow, and shell completions ([10d5206](https://github.com/valter-silva-au/ai-dev-brain/commit/10d520693fab8300e88fa5cd72cc779f4ed43573))
* **browser:** add 4-layer agentic browser automation stack ([54e89c4](https://github.com/valter-silva-au/ai-dev-brain/commit/54e89c42422c8f543c44626df211d87c176831f2))
* **claude:** add Claude Code template system with init-claude and sync-claude-user commands ([62bd7ba](https://github.com/valter-silva-au/ai-dev-brain/commit/62bd7babd9a1dd3209c1dd116a416909f4818a47))
* **cli:** add universal Claude Code status line with tiered enrichment ([#31](https://github.com/valter-silva-au/ai-dev-brain/issues/31)) ([badb34b](https://github.com/valter-silva-au/ai-dev-brain/commit/badb34bde2de1e2253d1b2cc50dd7d290ce69278))
* **cli:** auto-register adb MCP server in global Claude config ([c16fc61](https://github.com/valter-silva-au/ai-dev-brain/commit/c16fc61a934aa01301f9faf94c9463cfa1c393d6))
* **cli:** unify CLI into noun-verb command groups (task, sync) ([750b79d](https://github.com/valter-silva-au/ai-dev-brain/commit/750b79d0cde3d519ad75b83228ceea039a77aed4))
* **core:** add repo-path-based task organization ([5d42e7a](https://github.com/valter-silva-au/ai-dev-brain/commit/5d42e7a343c6d4ce30f837b8896f5fdb3844209e))
* **core:** add workspace-wide session capture and context evolution tracking ([#29](https://github.com/valter-silva-au/ai-dev-brain/issues/29)) ([f3ba3c7](https://github.com/valter-silva-au/ai-dev-brain/commit/f3ba3c723a80185d8768d878ae3f5d2b7935ae0d))
* **hooks:** add adb-native hook system ([#38](https://github.com/valter-silva-au/ai-dev-brain/issues/38)) ([fc5e144](https://github.com/valter-silva-au/ai-dev-brain/commit/fc5e14486a053f4c6d69dfe0885b232bb2a0851c))
* **hooks:** migrate to adb-native hooks and fix cross-platform issues ([1411e6a](https://github.com/valter-silva-au/ai-dev-brain/commit/1411e6aaab17ef9c77baaaeba2e64aa03c7fdfee))
* **integration:** Claude Code v2.1.50 integration ([#37](https://github.com/valter-silva-au/ai-dev-brain/issues/37)) ([3bcbd5a](https://github.com/valter-silva-au/ai-dev-brain/commit/3bcbd5a6b56de666256cced84ff546084e0d8ff7))
* move archived tickets to tickets/_archived/ for cleaner navigation ([49190e8](https://github.com/valter-silva-au/ai-dev-brain/commit/49190e85e498504c822f93a4c47a9dda5e72002b))
* pass --resume to Claude Code when resuming a task ([65b4504](https://github.com/valter-silva-au/ai-dev-brain/commit/65b4504e5feb0fc867a538b3c02d9c810cb53eef))
* replace default completion command with user-friendly UX ([c37f8e1](https://github.com/valter-silva-au/ai-dev-brain/commit/c37f8e1abec6318270365277e49370742b4b1163))
* wire priority/owner/tags flags, fix taskfile commands, push tes… ([#18](https://github.com/valter-silva-au/ai-dev-brain/issues/18)) ([db09fc8](https://github.com/valter-silva-au/ai-dev-brain/commit/db09fc8a68803787acf8dce3fb4e6c50a6718f06))
* wire priority/owner/tags flags, fix taskfile commands, push test coverage to 97% ([605dc34](https://github.com/valter-silva-au/ai-dev-brain/commit/605dc34dd5aa426ad0c4a30fb4dc079b5a005991))


### Bug Fixes

* **cli:** sync statusline.sh in sync-claude-user command ([#35](https://github.com/valter-silva-au/ai-dev-brain/issues/35)) ([1b390e7](https://github.com/valter-silva-au/ai-dev-brain/commit/1b390e728c63642b61ebf87c16c8fcec7a2a99d1))
* **integration:** resolve parent repo and force-remove worktrees ([aaeb15f](https://github.com/valter-silva-au/ai-dev-brain/commit/aaeb15ffc54035d845579d5832f9255bb306d984))
* migrate golangci-lint config to v2 format ([6901571](https://github.com/valter-silva-au/ai-dev-brain/commit/6901571429c624f9e8e4b241bfe3cbc303cb26b2))
* move exclude-rules to linters.exclusions.rules for golangci-lint v2 ([7794600](https://github.com/valter-silva-au/ai-dev-brain/commit/77946002171c2b5c036a3852749a0e8fd9ce51f3))
* prevent agent team hang by guarding Stop hook and making Teammat… ([#19](https://github.com/valter-silva-au/ai-dev-brain/issues/19)) ([3729297](https://github.com/valter-silva-au/ai-dev-brain/commit/3729297e7b2b09928f81acffb06039d5d30dcd9c))
* publish standalone Windows .exe binaries on release page ([8e5efd2](https://github.com/valter-silva-au/ai-dev-brain/commit/8e5efd2b36b6fec01acecc24917695e79583f031))
* remove gosimple linter (merged into staticcheck in golangci-lint v2) ([1ea4606](https://github.com/valter-silva-au/ai-dev-brain/commit/1ea460672fe3f74d46c9f9b2cf1a4bccf2e4ac6d))
* resolve CI lint failures and release workflow tag mismatch ([#23](https://github.com/valter-silva-au/ai-dev-brain/issues/23)) ([79f7c70](https://github.com/valter-silva-au/ai-dev-brain/commit/79f7c7034f44313603ee3cc2c02859a4a6ab0349))
* resolve final golangci-lint v2 warnings ([033713b](https://github.com/valter-silva-au/ai-dev-brain/commit/033713b7ac1124315bc963b14b68b3de2e986e82))
* resolve golangci-lint v2 warnings across codebase ([7965223](https://github.com/valter-silva-au/ai-dev-brain/commit/796522390bb16e135ffc9b5c87deb1b6ceb2ed32))
* resolve remaining golangci-lint v2 warnings across entire codebase ([1788780](https://github.com/valter-silva-au/ai-dev-brain/commit/17887809d3fa143a0d601c261c524b63d42aa57f))
* **security:** add file locking, input validation, and path traversal guards ([496d9c4](https://github.com/valter-silva-au/ai-dev-brain/commit/496d9c4d05735e0e61599fc5f97c465c28767ab7))
* **tests:** resolve all test failures from security and architecture changes ([26e7c06](https://github.com/valter-silva-au/ai-dev-brain/commit/26e7c06729b60e32b86391680ac155e009302c0a))


### Refactoring

* **audit:** unexport DocTemplates, fix formatting, expand CLI test coverage ([69b0dbb](https://github.com/valter-silva-au/ai-dev-brain/commit/69b0dbbf4adfd181c578573f2b2481d908de0af5))
* **core:** eliminate direct storage imports, deduplicate ticketpath ([11015c7](https://github.com/valter-silva-au/ai-dev-brain/commit/11015c7ed8be2a951c50f7aac78cfbaaa99e9672))
* **hooks:** consolidate Phase B, fix bugs, expand test coverage ([f2ef067](https://github.com/valter-silva-au/ai-dev-brain/commit/f2ef06710775528e1136e3d12b8b520f436956f1))
* **hooks:** remove obsolete adb hook scripts and update settings ([def4837](https://github.com/valter-silva-au/ai-dev-brain/commit/def48375d477d82db37183cfa577de85dec118ce))

## [1.7.0](https://github.com/valter-silva-au/ai-dev-brain/compare/v1.6.0...v1.7.0) (2026-02-24)


### Features

* add Claude Code status line with adb task context and portfolio… ([#26](https://github.com/valter-silva-au/ai-dev-brain/issues/26)) ([b2d7731](https://github.com/valter-silva-au/ai-dev-brain/commit/b2d77319bf2f05ff291ad521120269e78773ec19))
* add configurable task ID pad width, branch pattern, and dynamic… ([#25](https://github.com/valter-silva-au/ai-dev-brain/issues/25)) ([6d585c2](https://github.com/valter-silva-au/ai-dev-brain/commit/6d585c28b584b166ab32956ed8dcdf68177b051c))
* **browser:** add 4-layer agentic browser automation stack ([54e89c4](https://github.com/valter-silva-au/ai-dev-brain/commit/54e89c42422c8f543c44626df211d87c176831f2))
* **claude:** add Claude Code template system with init-claude and sync-claude-user commands ([62bd7ba](https://github.com/valter-silva-au/ai-dev-brain/commit/62bd7babd9a1dd3209c1dd116a416909f4818a47))
* **cli:** add universal Claude Code status line with tiered enrichment ([#31](https://github.com/valter-silva-au/ai-dev-brain/issues/31)) ([badb34b](https://github.com/valter-silva-au/ai-dev-brain/commit/badb34bde2de1e2253d1b2cc50dd7d290ce69278))
* **cli:** auto-register adb MCP server in global Claude config ([c16fc61](https://github.com/valter-silva-au/ai-dev-brain/commit/c16fc61a934aa01301f9faf94c9463cfa1c393d6))
* **core:** add repo-path-based task organization ([5d42e7a](https://github.com/valter-silva-au/ai-dev-brain/commit/5d42e7a343c6d4ce30f837b8896f5fdb3844209e))
* **core:** add workspace-wide session capture and context evolution tracking ([#29](https://github.com/valter-silva-au/ai-dev-brain/issues/29)) ([f3ba3c7](https://github.com/valter-silva-au/ai-dev-brain/commit/f3ba3c723a80185d8768d878ae3f5d2b7935ae0d))
* **hooks:** add adb-native hook system ([#38](https://github.com/valter-silva-au/ai-dev-brain/issues/38)) ([fc5e144](https://github.com/valter-silva-au/ai-dev-brain/commit/fc5e14486a053f4c6d69dfe0885b232bb2a0851c))
* **integration:** Claude Code v2.1.50 integration ([#37](https://github.com/valter-silva-au/ai-dev-brain/issues/37)) ([3bcbd5a](https://github.com/valter-silva-au/ai-dev-brain/commit/3bcbd5a6b56de666256cced84ff546084e0d8ff7))


### Bug Fixes

* **cli:** sync statusline.sh in sync-claude-user command ([#35](https://github.com/valter-silva-au/ai-dev-brain/issues/35)) ([1b390e7](https://github.com/valter-silva-au/ai-dev-brain/commit/1b390e728c63642b61ebf87c16c8fcec7a2a99d1))
* **integration:** resolve parent repo and force-remove worktrees ([aaeb15f](https://github.com/valter-silva-au/ai-dev-brain/commit/aaeb15ffc54035d845579d5832f9255bb306d984))
* resolve CI lint failures and release workflow tag mismatch ([#23](https://github.com/valter-silva-au/ai-dev-brain/issues/23)) ([79f7c70](https://github.com/valter-silva-au/ai-dev-brain/commit/79f7c7034f44313603ee3cc2c02859a4a6ab0349))

## [1.6.0](https://github.com/valter-silva-au/ai-dev-brain/compare/v1.5.0...v1.6.0) (2026-02-13)


### Features

* add feedback loop, knowledge store, and channel adapter system ([#21](https://github.com/valter-silva-au/ai-dev-brain/issues/21)) ([455b44b](https://github.com/valter-silva-au/ai-dev-brain/commit/455b44bac3ab2cf932c737a67662be2d98d33c9f))
* wire priority/owner/tags flags, fix taskfile commands, push tes… ([#18](https://github.com/valter-silva-au/ai-dev-brain/issues/18)) ([db09fc8](https://github.com/valter-silva-au/ai-dev-brain/commit/db09fc8a68803787acf8dce3fb4e6c50a6718f06))


### Bug Fixes

* prevent agent team hang by guarding Stop hook and making Teammat… ([#19](https://github.com/valter-silva-au/ai-dev-brain/issues/19)) ([3729297](https://github.com/valter-silva-au/ai-dev-brain/commit/3729297e7b2b09928f81acffb06039d5d30dcd9c))

## [1.5.0](https://github.com/valter-silva-au/ai-dev-brain/compare/v1.4.0...v1.5.0) (2026-02-13)


### Features

* add feedback loop, knowledge store, and channel adapter system ([#21](https://github.com/valter-silva-au/ai-dev-brain/issues/21)) ([455b44b](https://github.com/valter-silva-au/ai-dev-brain/commit/455b44bac3ab2cf932c737a67662be2d98d33c9f))
* wire priority/owner/tags flags, fix taskfile commands, push tes… ([#18](https://github.com/valter-silva-au/ai-dev-brain/issues/18)) ([db09fc8](https://github.com/valter-silva-au/ai-dev-brain/commit/db09fc8a68803787acf8dce3fb4e6c50a6718f06))


### Bug Fixes

* prevent agent team hang by guarding Stop hook and making Teammat… ([#19](https://github.com/valter-silva-au/ai-dev-brain/issues/19)) ([3729297](https://github.com/valter-silva-au/ai-dev-brain/commit/3729297e7b2b09928f81acffb06039d5d30dcd9c))

## [1.4.0](https://github.com/valter-silva-au/ai-dev-brain/compare/v1.3.0...v1.4.0) (2026-02-13)


### Features

* add Phase 3 advanced features - TUI dashboard, MCP server, Slack notifications ([#14](https://github.com/valter-silva-au/ai-dev-brain/issues/14)) ([917f622](https://github.com/valter-silva-au/ai-dev-brain/commit/917f622afcb726996108be4ba4d75f3560875d6d))
* pass --resume to Claude Code when resuming a task ([65b4504](https://github.com/valter-silva-au/ai-dev-brain/commit/65b4504e5feb0fc867a538b3c02d9c810cb53eef))
* wire priority/owner/tags flags, fix taskfile commands, push tes… ([#18](https://github.com/valter-silva-au/ai-dev-brain/issues/18)) ([db09fc8](https://github.com/valter-silva-au/ai-dev-brain/commit/db09fc8a68803787acf8dce3fb4e6c50a6718f06))


### Bug Fixes

* prevent agent team hang by guarding Stop hook and making Teammat… ([#19](https://github.com/valter-silva-au/ai-dev-brain/issues/19)) ([3729297](https://github.com/valter-silva-au/ai-dev-brain/commit/3729297e7b2b09928f81acffb06039d5d30dcd9c))

## [1.3.0](https://github.com/valter-silva-au/ai-dev-brain/compare/v1.2.0...v1.3.0) (2026-02-13)


### Features

* add Phase 3 advanced features - TUI dashboard, MCP server, Slack notifications ([#14](https://github.com/valter-silva-au/ai-dev-brain/issues/14)) ([917f622](https://github.com/valter-silva-au/ai-dev-brain/commit/917f622afcb726996108be4ba4d75f3560875d6d))
* pass --resume to Claude Code when resuming a task ([65b4504](https://github.com/valter-silva-au/ai-dev-brain/commit/65b4504e5feb0fc867a538b3c02d9c810cb53eef))
* wire priority/owner/tags flags, fix taskfile commands, push test coverage to 97% ([605dc34](https://github.com/valter-silva-au/ai-dev-brain/commit/605dc34dd5aa426ad0c4a30fb4dc079b5a005991))

## [1.2.0](https://github.com/valter-silva-au/ai-dev-brain/compare/v1.1.0...v1.2.0) (2026-02-13)


### Features

* add observability infrastructure with metrics, alerts, and session tracking ([#14](https://github.com/valter-silva-au/ai-dev-brain/issues/14)) ([2598c62](https://github.com/valter-silva-au/ai-dev-brain/commit/2598c62d3d514b167ddbb1671a6003aafad52a93))
* move archived tickets to tickets/_archived/ for cleaner navigation ([49190e8](https://github.com/valter-silva-au/ai-dev-brain/commit/49190e85e498504c822f93a4c47a9dda5e72002b))

## [1.1.0](https://github.com/valter-silva-au/ai-dev-brain/compare/v1.0.1...v1.1.0) (2026-02-13)


### Features

* add .gitignore template and git init to adb init ([94a31ff](https://github.com/valter-silva-au/ai-dev-brain/commit/94a31ffb72b06ea886faa1e59c4a2c5a756e3ef0))
* add `adb init` command to bootstrap project workspace ([f3ce485](https://github.com/valter-silva-au/ai-dev-brain/commit/f3ce4858bdbe5530c7e5d69ed91ff0ccd0d0102c))
* add cleanup command, fix worktree placement, and persist terminal title ([237f83e](https://github.com/valter-silva-au/ai-dev-brain/commit/237f83e1265e80fb377d52297a5cbee3b6034743))
* add observability infrastructure with metrics, alerts, and session tracking ([#14](https://github.com/valter-silva-au/ai-dev-brain/issues/14)) ([2598c62](https://github.com/valter-silva-au/ai-dev-brain/commit/2598c62d3d514b167ddbb1671a6003aafad52a93))
* add sync-repos command with backlog-aware branch protection ([f68e1aa](https://github.com/valter-silva-au/ai-dev-brain/commit/f68e1aa8ddf05a5d57525ed281d22e7c91307262))
* add workflow skills (push, pr, review, sync, changelog) to adb init ([8990b54](https://github.com/valter-silva-au/ai-dev-brain/commit/8990b54866b6253523a0f956f322761b752065ba))
* add worktree auto-detect, post-create workflow, and shell completions ([10d5206](https://github.com/valter-silva-au/ai-dev-brain/commit/10d520693fab8300e88fa5cd72cc779f4ed43573))
* move archived tickets to tickets/_archived/ for cleaner navigation ([49190e8](https://github.com/valter-silva-au/ai-dev-brain/commit/49190e85e498504c822f93a4c47a9dda5e72002b))
* replace default completion command with user-friendly UX ([c37f8e1](https://github.com/valter-silva-au/ai-dev-brain/commit/c37f8e1abec6318270365277e49370742b4b1163))


### Bug Fixes

* migrate golangci-lint config to v2 format ([6901571](https://github.com/valter-silva-au/ai-dev-brain/commit/6901571429c624f9e8e4b241bfe3cbc303cb26b2))
* move exclude-rules to linters.exclusions.rules for golangci-lint v2 ([7794600](https://github.com/valter-silva-au/ai-dev-brain/commit/77946002171c2b5c036a3852749a0e8fd9ce51f3))
* remove gosimple linter (merged into staticcheck in golangci-lint v2) ([1ea4606](https://github.com/valter-silva-au/ai-dev-brain/commit/1ea460672fe3f74d46c9f9b2cf1a4bccf2e4ac6d))
* resolve final golangci-lint v2 warnings ([033713b](https://github.com/valter-silva-au/ai-dev-brain/commit/033713b7ac1124315bc963b14b68b3de2e986e82))
* resolve golangci-lint v2 warnings across codebase ([7965223](https://github.com/valter-silva-au/ai-dev-brain/commit/796522390bb16e135ffc9b5c87deb1b6ceb2ed32))
* resolve remaining golangci-lint v2 warnings across entire codebase ([1788780](https://github.com/valter-silva-au/ai-dev-brain/commit/17887809d3fa143a0d601c261c524b63d42aa57f))

## [1.0.1](https://github.com/valter-silva-au/ai-dev-brain/compare/v1.0.0...v1.0.1) (2026-02-11)


### Bug Fixes

* publish standalone Windows .exe binaries on release page ([8e5efd2](https://github.com/valter-silva-au/ai-dev-brain/commit/8e5efd2b36b6fec01acecc24917695e79583f031))

## 1.0.0 (2026-02-11)


### Features

* add Release Please for automated versioning and changelog ([b9ab55e](https://github.com/valter-silva-au/ai-dev-brain/commit/b9ab55e040b0e57881e982e75da6cb6a268ac6f8))
