# Changelog

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
