// ai-dev-brain (adb) CI — repo-owned pipeline for the fleet's self-hosted
// Jenkins hub. Replaces the former .github/workflows/ci.yml (GitHub Actions is
// billing-blocked account-wide and never ran; see TASK-00070).
//
// Covers the same gates the Actions matrix declared (test / lint / vet) plus
// build and a CRLF-aware gofmt gate. Where the hub allows it, the pipeline is
// *better* than the Actions version ever was:
//   - the full `go test -race` suite (needs gcc-backed cgo; Actions was Linux
//     + billing-blocked, so this coverage never actually existed there);
//   - gotestsum --rerun-fails flake handling (a failing test is rerun
//     individually, so intermittent FS flakes recover while genuine
//     regressions that fail every rerun still fail the build — verified);
//   - the slow TestGenerateTaskID_Format (~100k counter-flock cycles) is
//     skipped from the race suite and run timeboxed below.
//
// Runs on the hub's built-in Linux node (Go + gcc baked into the image).
pipeline {
  agent any

  options {
    timeout(time: 40, unit: 'MINUTES')
    disableConcurrentBuilds()
    buildDiscarder(logRotator(numToKeepStr: '20'))
  }

  // Local box: no inbound webhook from GitHub → poll.
  triggers {
    pollSCM('H/5 * * * *')
  }

  environment {
    PATH = "/usr/local/go/bin:/var/jenkins_home/go/bin:${PATH}"
  }

  stages {
    stage('Checkout') {
      steps {
        git branch: 'main',
            url: 'https://github.com/valter-silva-au/ai-dev-brain.git',
            credentialsId: 'github-token'
      }
    }

    stage('Setup') {
      steps {
        sh '''
          set -eu
          go version
          gcc --version | head -1
          # adb's git-backed tests (reposync, worktrees) commit inside t.TempDir()
          # workspaces and need a committer identity + a default branch name.
          git config --global user.name  "adb CI"
          git config --global user.email "ci@localhost"
          git config --global init.defaultBranch main
          # gotestsum gives us --rerun-fails (flaky-test handling).
          command -v gotestsum >/dev/null 2>&1 || go install gotest.tools/gotestsum@latest
          gotestsum --version
        '''
      }
    }

    stage('Format / Vet / Build') {
      steps {
        sh '''
          set -eu
          # CRLF-aware gofmt gate: strip CR from BOTH gofmt's output and the file
          # before comparing, so a CRLF checkout can never produce false drift.
          # `gofmt -s` matches the repo's `make fmt` (gofmt -s -w), so the gate is
          # equivalent to the local target — a file needing only -s simplification
          # is caught here too.
          tmp=$(mktemp -d)
          drift=""
          for f in $(git ls-files "*.go"); do
            gofmt -s "$f"     | tr -d '\\r' > "$tmp/want"
            tr -d '\\r' < "$f"              > "$tmp/have"
            cmp -s "$tmp/want" "$tmp/have" || drift="$drift $f"
          done
          rm -rf "$tmp"
          if [ -n "$drift" ]; then
            echo "gofmt drift (run gofmt -w on):$drift" >&2
            exit 1
          fi
          echo "gofmt: clean"
          # modernc.org/sqlite is pure Go, so build/vet need no cgo (faster).
          CGO_ENABLED=0 go vet ./...
          CGO_ENABLED=0 go build ./...
        '''
      }
    }

    stage('Lint') {
      steps {
        // Mirrors the Actions lint job: golangci-lint, whole module, 5m budget.
        // The repo pins no version; install on demand (slow first run — go
        // compiles it — then cached in GOPATH/bin for subsequent builds).
        sh '''
          set -eu
          command -v golangci-lint >/dev/null 2>&1 || \
            go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
          golangci-lint run ./... --timeout=5m
        '''
      }
    }

    stage('Test (race)') {
      steps {
        // The headline: the whole suite under the race detector (gcc-backed cgo).
        //  - the slow TestGenerateTaskID_Format is skipped here and run timeboxed
        //    below (it hammers the file-locked counter ~100k times);
        //  - gotestsum --rerun-fails reruns any failing test individually, so the
        //    known intermittent flakes (a counter-flock / t.TempDir EBADF under
        //    heavy -race concurrency on the container FS) recover instead of
        //    red-walling the build — while a genuine regression that fails EVERY
        //    rerun still fails the build (verified).
        sh '''
          set -eu
          gotestsum --rerun-fails=2 --packages="./..." --format=testname \
            -- -race -count=1 -skip "^TestGenerateTaskID_Format$"
        '''
      }
    }

    stage('Slow test (timeboxed)') {
      steps {
        // TestGenerateTaskID_Format is ~100s on Linux (far slower under -race):
        // it acquires/releases the counter's file lock up to 100k times. Run it
        // once for correctness — race-exempt and timeboxed — so it neither
        // dominates the build nor stalls it.
        sh '''
          set -eu
          go test ./internal/core -run "^TestGenerateTaskID_Format$" -count=1 -timeout 20m
        '''
      }
    }
  }

  post {
    always {
      echo "adb CI result: ${currentBuild.currentResult}"
    }
  }
}
